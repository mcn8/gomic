package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/SlyMarbo/rss"
	"github.com/asaskevich/govalidator"
	"github.com/boltdb/bolt"
	"golang.org/x/net/websocket"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var feeds map[string][]FeedBundle
var sentId uint64
var db *bolt.DB
var world = []byte("world")
var FEEDSKEY = []byte("FEEDSKEY")
var BOTAPIKEY = []byte("BOTAPIKEY")

func main() {
	botAPIValue := connectToDb()
	defer db.Close()
	ws := connectToSlack(botAPIValue)

	sentId = uint64(0)

	go checkFeeds(ws)
	go backup()

	readMessages(ws)
}

func connectToDb() string {
	var err error
	db, err = bolt.Open("gomic.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	return initializeDb(db)
}

func initializeDb(db *bolt.DB) string {
	var botAPIValue string
	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(world)

		if err != nil {
			return err
		}

		if botAPIByteValue := bucket.Get(BOTAPIKEY); botAPIByteValue == nil {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Enter bot API key: ")
			botAPIValue, err = reader.ReadString('\n')
			if err != nil {
				return err
			}

			err = bucket.Put(BOTAPIKEY, []byte(botAPIValue))
			if err != nil {
				return err
			}
		} else {
			botAPIValue = string(botAPIByteValue)
		}

		feeds = make(map[string][]FeedBundle)
		if feedsBytes := bucket.Get(FEEDSKEY); feedsBytes != nil {
			json.Unmarshal(feedsBytes, &feeds)
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("botapivalue: " + botAPIValue)

	return botAPIValue
}

func connectToSlack(botAPIValue string) *websocket.Conn {
	fmt.Println("https://slack.com/api/rtm.start?token=" + botAPIValue)
	resp, err := http.Get("https://slack.com/api/rtm.start?token=" + strings.TrimSpace(botAPIValue))
	if err != nil {
		fmt.Println("resp is kil")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("body is kil")
	}

	var respObj ResponseRtmStart
	err = json.Unmarshal(body, &respObj)

	ws, err := websocket.Dial(respObj.Url, "", "https://api.slack.com/")
	if err != nil {
		fmt.Println("ws is kil")
	}
	return ws
}

func checkFeeds(ws *websocket.Conn) {
	fmt.Println("Calling a check on the feedses")
	for channel, feedSlice := range feeds {
		go checkFeed(channel, feedSlice, ws)
	}
}

func checkFeed(channel string, feedSlice []FeedBundle, ws *websocket.Conn) {
	for {
		fmt.Println("Checking channel " + channel)
		feedSlice = feeds[channel]

		if len(feedSlice) > 0 {
			for feedIteration := range feedSlice {
				feed := feedSlice[feedIteration]
				fmt.Println("Checking for updates for " + feed.Url)
				feed.ClearItems()

				err := feed.Update()
				if err != nil {
					fmt.Println(err)
				}

				items := feed.GetItems()
				for itemIteration := range items {
					item := items[itemIteration]
					fmt.Println(item.Title)
					postLink(channel, item, ws)
				}

				feed.RssHandler.Refresh = time.Now()
			}

		} else {
			return
		}

		time.Sleep(time.Minute * 10)
	}
}

func readMessages(ws *websocket.Conn) {
	for {
		var m Message
		err := websocket.JSON.Receive(ws, &m)
		if err == nil {
			handleMessage(m, ws)
		} else {
			fmt.Println(err)
		}
	}
}

func handleMessage(m Message, ws *websocket.Conn) {
	fmt.Println(m)
	if m.Type == "message" {
		text := strings.ToLower(m.Text)
		if strings.HasPrefix(text, "gomic add rss") {
			addRss(m, ws)
		} else if strings.HasPrefix(text, "gomic ls -a") {
			listAllRss(m.Channel, ws)
		} else if strings.HasPrefix(text, "gomic ls") {
			listRssInChannel(m.Channel, ws)
		} else if strings.HasPrefix(text, "gomic rm") {
			removeRss(m, ws)
		} else if strings.HasPrefix(text, "gomic help") {
			printHelp(m.Channel, ws)
		} else if strings.HasPrefix(text, "gomic") {
			sayHi(m.Channel, ws)
		}
	}
}

func addRss(m Message, ws *websocket.Conn) {
	feedUrl := getArgument(m, 3, ws)
	if feedUrl != "" {
		feedUrl = feedUrl[1 : len(feedUrl)-1]
		if govalidator.IsURL(feedUrl) {
			channelFeeds := feeds[m.Channel]
			for _, f := range channelFeeds {
				if f.Url == feedUrl {
					sendMessage(m.Channel, "Already tracking that url here, no worries homie :snowboarder:", ws)
					return
				}
			}

			var newFeed FeedBundle
			var err error
			newFeed.Url = feedUrl
			newFeed.RssHandler, err = rss.Fetch(feedUrl)
			if err != nil {
				sendMessage(m.Channel, "Incorrect url :OoOoOoOo", ws)
			} else {
				createNewFeedChecker := false
				if _, exists := feeds[m.Channel]; !exists {
					createNewFeedChecker = true
				}
				feeds[m.Channel] = append(feeds[m.Channel], newFeed)
				sendMessage(m.Channel, "Yes, sir. "+feedUrl+" added.", ws)

				if createNewFeedChecker {
					go checkFeed(m.Channel, feeds[m.Channel], ws)
				}
			}
		} else {
			sendMessage(m.Channel, "Incorrect url :OoOoOoOo", ws)
		}
	}
}

func getArgument(m Message, position int, ws *websocket.Conn) string {
	tokens := strings.Split(m.Text, " ")
	if len(tokens) <= position {
		sendMessage(m.Channel, "You need more arguments :(", ws)
		return ""
	}

	return tokens[position]
}

func listRssInChannel(channel string, ws *websocket.Conn) {
	text := getRssInChannel(channel)
	if text == "" {
		text = "No RSS feeds in this channel :neutral_face:"
	}
	sendMessage(channel, text, ws)
}

func getRssInChannel(channel string) string {
	text := "Listing all feeds in channel <#" + channel + ">"

	channelFeeds := feeds[channel]
	for _, feed := range channelFeeds {
		text = text + "\n" + feed.Url
	}

	if text == "Listing all feeds in channel <#"+channel+">" {
		return ""
	} else {
		return text
	}
}

func listAllRss(channel string, ws *websocket.Conn) {
	text := "Listing all RSS feeds\n\n"

	for c := range feeds {
		channelText := getRssInChannel(c)
		if channelText != "" {
			text = text + getRssInChannel(c) + "\n"
		}
	}

	if text == "Listing all RSS feeds\n\n" {
		fmt.Println(text)
		text = "Looks like your team doesn't have any RSS feeds at the moment. You should add some! :wink:"
	}

	sendMessage(channel, text, ws)
}

func removeRss(m Message, ws *websocket.Conn) {
	urlToRemove := getArgument(m, 2, ws)
	urlToRemove = urlToRemove[1 : len(urlToRemove)-1]
	for c, channelFeeds := range feeds {
		for i, f := range channelFeeds {
			if f.Url == urlToRemove {
				feeds[c] = append(channelFeeds[:i], channelFeeds[i+1:]...)
				sendMessage(m.Channel, urlToRemove+" found and annihilated :fire: :sunglasses: :fire: ", ws)
				return
			}
		}
	}

	sendMessage(m.Channel, "I couldn't find that one... try again :ambulance:", ws)
}

func backup() {
	var err error
	for {
		if err = saveFeeds(); err != nil {
			log.Fatal(err)
		}

		time.Sleep(time.Second * 10)
	}
}

func saveFeeds() error {
	err := db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(world)

		feedsJson, err := json.Marshal(feeds)
		if err != nil {
			return err
		}

		return bucket.Put(FEEDSKEY, feedsJson)
	})

	return err
}

func printHelp(channel string, ws *websocket.Conn) {
	sendMessage(channel,
		`Hi there! I'm gomic! I'm a general purpose rss slack bot (originally created for posting webcomic feeds). I'm also made in Go! Sometimes I wonder where my name comes from. 
		
OH! You probably want some help. Here's what I can do:

- "gomic add rss {url}" : adds an rss feed to my database for me to automatically check and post updates in the channel it was added from
- "gomic ls [-a]" : Lists all the RSS feeds in the channel you're in. If -a flag is given, lists all RSS feeds in slack team"
- "gomic rm {url}" : removes an rss feed from the database
- "gomic help" : what you did just now! Prints the help!
- "gomic" : In case you just wanted to say hi :)`, ws)
}

func sayHi(channel string, ws *websocket.Conn) {
	sendMessage(channel, "Hi, that's my name :simple_smile:", ws)
}

func postLink(channel string, item *rss.Item, ws *websocket.Conn) {
	sendMessage(channel, "Hey, have you checked this out "+item.Link+" ?", ws)
}
func sendMessage(channel string, text string, ws *websocket.Conn) {
	var botMsg Message
	botMsg.Id = sentId
	sentId++
	botMsg.Type = "message"
	botMsg.Channel = channel
	botMsg.Text = text
	websocket.JSON.Send(ws, botMsg)
}

type FeedBundle struct {
	Url        string    `json:"url"`
	RssHandler *rss.Feed `json:"rssHandler"`
}

func (f FeedBundle) Update() error {
	return f.RssHandler.Update()
}

func (f FeedBundle) GetItemMap() map[string]struct{} {
	return f.RssHandler.ItemMap
}

func (f FeedBundle) GetItems() []*rss.Item {
	return f.RssHandler.Items
}

func (f FeedBundle) ClearItems() {
	f.RssHandler.Items = make([]*rss.Item, 0)
}

type ResponseRtmStart struct {
	Ok    bool         `json:"ok"`
	Error string       `json:"error"`
	Url   string       `json:"url"`
	Self  ResponseSelf `json:"self"`
}

type ResponseSelf struct {
	Id string `json:"id"`
}

type Message struct {
	Id      uint64 `json:"id"`
	Type    string `json:"type"`
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

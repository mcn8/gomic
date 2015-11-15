## A slackbot for RSS feeds written in Go!

> Gomic was originally created to post webcomic RSS feeds in a slack channel. But he works for any general purpose RSS feeds

### How he works

Just run

```
go get github.com/mccordnate/gomic
go build gomic.go
./gomic.go
```

then follow the steps [here](https://api.slack.com/bot-users) to create a bot user to get your API key. After that, he should be running in your team's slack, just invite him to channels and run his commands. Should you ever forget what he can do, just run `gomic help`

### Commands

```
- "gomic add rss {url}" : adds an rss feed to my database for me to automatically check and post updates in the channel it was added from
- "gomic ls [-a]" : Lists all the RSS feeds in the channel you're in. If -a flag is given, lists all RSS feeds in slack team"
- "gomic rm {url}" : removes an rss feed from the database
- "gomic help" : what you did just now! Prints the help!
- "gomic" : In case you just wanted to say hi :)
```

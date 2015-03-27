package main

import (
	"github.com/nurza/logo"

	"github.com/ZenlabsFR/GitlabHookServer/data"

	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

/*
	Global variables
*/
var (
	// Logging
	l       logo.Logger
	Loggers []*logo.Logger

	// Configuration
	BotUsername     string     // Bot's username
	BotChannel      string     // Bot's system channel
	BotIcon         string     // Bot's icon (Slack emoji)
	PushIcon        string     // Push icon (Slack emoji)
	MergeIcon       string     // Merge icon (Slack emoji)
	BuildIcon       string     // Build icon (Slack emoji)
	BotStartMessage string     // Bot's start message
	SlackAPIUrl     string     // Slack API URL
	SlackAPIToken   string     // Slack API Token
	ChannelPrefix   string     // Slack channel prefix
	Verbose         bool       // Enable verbose mode
	HttpTimeout     int        // Http timeout in second
	Redirect        []struct { // List of channel redirect
		Channel      string
		Repositories []string
	}

	// Misc
	currentBuildID float64 = 0      // Current build ID
	n              string  = "%5Cn" // Encoded line return
)

/*
	Flags
*/
var (
	ConfigFile = flag.String("f", "config.json", "Configuration file")
)

const (
	Bot   int = iota
	Push  int = iota
	Merge int = iota
	Build int = iota
)

/*
	Struc for HTTP servers
*/
type PushServ struct{}
type MergeServ struct{}
type BuildServ struct{}

/*
	Load configuration file
*/
func LoadConf() {

	conf := struct {
		BotUsername     string
		BotChannel      string
		BotIcon         string
		PushIcon        string
		MergeIcon       string
		BuildIcon       string
		BotStartMessage string
		SlackAPIUrl     string
		SlackAPIToken   string
		ChannelPrefix   string
		Verbose         bool
		HttpTimeout     float64
		Redirect        []struct {
			Channel      string
			Repositories []string
		}
	}{}

	content, err := ioutil.ReadFile(*ConfigFile)
	if err != nil {
		l.Critical("Error: Read config file error: " + err.Error())
	}

	err = json.Unmarshal(content, &conf)
	if err != nil {
		l.Critical("Error: Parse config file error: " + err.Error())
	}

	BotUsername = conf.BotUsername
	BotChannel = conf.BotChannel
	BotIcon = conf.BotIcon
	PushIcon = conf.PushIcon
	MergeIcon = conf.MergeIcon
	BuildIcon = conf.BuildIcon
	BotStartMessage = conf.BotStartMessage
	SlackAPIUrl = conf.SlackAPIUrl
	SlackAPIToken = conf.SlackAPIToken
	ChannelPrefix = conf.ChannelPrefix
	Verbose = conf.Verbose
	HttpTimeout = int(conf.HttpTimeout)
	Redirect = conf.Redirect
}

/*
	HTTP POST request

	target:		url target
	payload:	payload to send

	Returned values:

	int:	HTTP response status code
	string:	HTTP response body
*/
func Post(target string, payload string) (int, string) {
	// Variables
	var err error          // Error catching
	var res *http.Response // HTTP response
	var req *http.Request  // HTTP request
	var body []byte        // Body response

	// Build request
	req, err = http.NewRequest("POST", target, bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Do request
	client := &http.Client{}
	client.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   time.Duration(HttpTimeout) * time.Second,
			KeepAlive: time.Duration(HttpTimeout) * time.Second,
		}).Dial,
		TLSHandshakeTimeout: time.Duration(HttpTimeout) * time.Second,
	}

	res, err = client.Do(req)
	if err != nil {
		l.Error("Error : Curl POST : " + err.Error())
		if res != nil {
			return res.StatusCode, ""
		} else {
			return 0, ""
		}
	}
	defer res.Body.Close()

	// Read body
	body, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		l.Error("Error : Curl POST body read : " + err.Error())
	}

	return res.StatusCode, string(body)
}

/*
	Create a Slack channel

	@param chanName : The Slack channel name (without the #)
*/
func CreateSlackChannel(chanName string) {
	// Variables
	var err error                                       // Error catching
	var supl string = "&name=" + chanName + "&pretty=1" // Additional request
	var resp *http.Response                             // Response

	// API Get
	resp, err = http.Get("https://slack.com/api/channels.join?token=" + SlackAPIToken + supl)

	if err != nil {
		// Error
		l.Error("Error : CreateSlackChannel :", err, "\nResponse :", resp)
	} else {
		// Ok
		l.Verbose("CreateSlackChannel OK\nResponse :", resp)
	}
}

/*
	Encode the git commit message with replacing some special characters not allowed by the Slack API

	@param origin Git message to encode
*/
func MessageEncode(origin string) string {
	var result string = ""

	for _, e := range strings.Split(origin, "") {
		switch e {
		case "\n":
			result += "%5Cn"
		case "+":
			result += "%2B"
		case "\"":
			result += "''"
		case "&":
			result += " and "
		default:
			result += e
		}
	}

	return result
}

/*
	Send a message on Slack

	@param channel : Targeted channel (without the #)
*/
func SendSlackMessage(channel, message string, typeMessage int) {
	// Variables
	var payload string // POST data sent to slack
	var icon string    // Slack emoji

	// toLower(channel)
	l.Silly("toLower =", channel)
	channel = strings.ToLower(channel)
	l.Silly("toLower =", channel)

	// Redirect channel
	l.Silly("RedirectBreak =", channel)
RedirectBreak:
	for _, redirect := range Redirect {
		for _, repo := range redirect.Repositories {
			if channel == repo {
				l.Silly("RedirectBreakSet", channel, "=", redirect.Channel)
				channel = redirect.Channel
				break RedirectBreak
			}
		}
	}
	l.Silly("RedirectBreak =", channel)

	// Insert prefix on non system channels
	l.Silly("ChannelPrefix =", channel)
	if channel != BotChannel {
		channel = ChannelPrefix + channel
	}
	l.Silly("ChannelPrefix =", channel)

	// Crop channel name if len(channel)>21
	l.Silly("Crop =", channel)
	if len(channel) > 21 {
		channel = channel[:21]
	}
	l.Silly("Crop =", channel)

	// Create channel if not exists
	CreateSlackChannel(channel)

	// Set icon
	switch typeMessage {
	case Bot:
		icon = BotIcon
	case Push:
		icon = PushIcon
	case Merge:
		icon = MergeIcon
	case Build:
		icon = BuildIcon
	}

	// POST Payload formating
	payload = "payload="
	payload += `{"channel": "#` + strings.ToLower(channel) + `", "username": "` + BotUsername + `", "text": "` + message + `", "icon_emoji": "` + icon + `"}`

	// Debug information
	if Verbose {
		l.Debug("payload =", payload)
	}

	code, body := Post(SlackAPIUrl, payload)
	if code != 200 {
		l.Error("Error post, Slack API returned:", body)
	}

	// Debug information
	if Verbose {
		l.Debug("Slack API returned:", body)
	}
}

/*
	Handler function to handle http requests for push

	@param w http.ResponseWriter
	@param r *http.Request
*/
func (s *PushServ) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var j data.Push         // Json structure to parse the push webhook
	var buffer bytes.Buffer // Buffer to get request body
	var body string         // Request body (it's a json)
	var err error           // Error catching
	var message string = "" // Bot's message
	var date time.Time      // Time of the last commit

	// Log
	l.Info("Push Request")

	// Read http request body and put it in a string
	buffer.ReadFrom(r.Body)
	body = buffer.String()

	// Debug information
	if Verbose {
		l.Debug("JsonString receive =", body)
	}

	// Parse json and put it in a the data.Build structure
	err = json.Unmarshal([]byte(body), &j)
	if err != nil {
		// Error
		l.Error("Error : Json parser failed :", err)
	} else {
		// Ok
		// Debug information
		if Verbose {
			l.Debug("JsonObject =", j)
		}

		// Send the message

		// Date parsing (parsing result example : 18 November 2014 - 14:34)
		date, err = time.Parse("2006-01-02T15:04:05Z07:00", j.Commits[0].Timestamp)
		var dateString = date.Format("02 Jan 06 15:04")

		// Message
		lastCommit := j.Commits[len(j.Commits)-1]
		message += "[PUSH] " + n + "Push on *" + j.Repository.Name + "* by *" + j.User_name + "* at *" + dateString + "* on branch *" + j.Ref + "*:" + n // First line
		message += "Last commit : <" + lastCommit.Url + "|" + lastCommit.Id + "> :" + n                                                                  // Second line
		message += "```" + MessageEncode(lastCommit.Message) + "```"                                                                                     // Third line (last commit message)
		SendSlackMessage(j.Repository.Name, message, Push)
	}
}

/*
	Handler function to handle http requests for merge

	@param w http.ResponseWriter
	@param r *http.Request
*/
func (s *MergeServ) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var j data.Merge        // Json structure to parse the push webhook
	var buffer bytes.Buffer // Buffer to get request body
	var body string         // Request body (it's a json)
	var err error           // Error catching
	var message string = "" // Bot's message
	var date time.Time      // Time of the last commit

	// Log
	l.Info("Merge Request")

	// Read http request body and put it in a string
	buffer.ReadFrom(r.Body)
	body = buffer.String()

	// Debug information
	if Verbose {
		l.Debug("JsonString receive =", body)
	}

	// Parse json and put it in a the data.Build structure
	err = json.Unmarshal([]byte(body), &j)
	if err != nil {
		// Error
		l.Error("Error : Json parser failed :", err)
	} else {
		// Ok
		// Debug information
		if Verbose {
			l.Debug("JsonObject =", j)
		}

		// Send the message

		// Date parsing (parsing result example : 18 November 2014 - 14:34)
		date, err = time.Parse("2006-01-02 15:04:05 UTC", j.Object_attributes.Created_at)
		var dateString = date.Format("02 Jan 06 15:04")

		// Message
		message += "[MERGE REQUEST " + strings.ToUpper(j.Object_attributes.State) + "] " + n + "Target : *" + j.Object_attributes.Target.Name + "/" + j.Object_attributes.Target_branch + "* Source : *" + j.Object_attributes.Source.Name + "/" + j.Object_attributes.Source_branch + "* : at *" + dateString + "* :" + n // First line
		message += "```" + MessageEncode(j.Object_attributes.Description) + "```"                                                                                                                                                                                                                                          // Third line (last commit message)
		SendSlackMessage(j.Object_attributes.Target.Name, message, Merge)
	}
}

/*
	Handler function to handle http requests for build

	@param w http.ResponseWriter
	@param r *http.Request
*/
func (s *BuildServ) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var j data.Build        // Json structure to parse the build webhook
	var buffer bytes.Buffer // Buffer to get request body
	var body string         // Request body (it's a json)
	var err error           // Error catching
	var message string = "" // Bot's message
	var date time.Time      // Time of the last commit

	// Log
	l.Info("Build Request")

	// Read http request body and put it in a string
	buffer.ReadFrom(r.Body)
	body = buffer.String()

	// Debug information
	if Verbose {
		l.Debug("JsonString receive =", body)
	}

	// Parse json and put it in a the data.Build structure
	err = json.Unmarshal([]byte(body), &j)
	if err != nil {
		// Error
		l.Error("Error : Json parser failed :", err)
	} else {
		// Ok
		// Debug information
		if Verbose {
			l.Debug("JsonObject =", j)
		}

		// Test if the message is already sent
		if currentBuildID < j.Build_id {
			// Not sent
			currentBuildID = j.Build_id // Update current build ID

			// Send the message

			// Date parsing (parsing result example : 18 November 2014 - 14:34)
			date, err = time.Parse("2006-01-02T15:04:05Z07:00", j.Push_data.Commits[0].Timestamp)
			var dateString = strconv.Itoa(date.Day()) + " " + date.Month().String() + " " + strconv.Itoa(date.Year()) +
				" - " + strconv.Itoa(date.Hour()) + ":" + strconv.Itoa(date.Minute())

			// Message
			lastCommit := j.Push_data.Commits[len(j.Push_data.Commits)-1]
			message += "[BUILD] " + n + strings.ToUpper(j.Build_status) + " : Push on *" + j.Push_data.Repository.Name + "* by *" + j.Push_data.User_name + "* at *" + dateString + "* on branch *" + j.Ref + "*:" + n // First line
			message += "Last commit : <" + lastCommit.Url + "|" + lastCommit.Id + "> :" + n                                                                                                                            // Second line
			message += "```" + MessageEncode(lastCommit.Message) + "```"                                                                                                                                               // Third line (last commit message)
			SendSlackMessage(j.Push_data.Repository.Name, message, Build)
		} else {
			// Already sent
			// Do nothing
		}
	}

}

/*
	Main function
*/
func main() {
	flag.Parse()                                             // Parse flags
	l.AddTransport(logo.Console).AddColor(logo.ConsoleColor) // Configure Logger
	l.EnableAllLevels()                                      // Configure Logger
	LoadConf()                                               // Load configuration
	SendSlackMessage(BotChannel, BotStartMessage, Bot)       // Slack notification
	l.Info(BotStartMessage)                                  // Logging
	go http.ListenAndServe(":8100", &PushServ{})             // Run HTTP server for push hook
	go http.ListenAndServe(":8200", &MergeServ{})            // Run HTTP server for merge request hook
	http.ListenAndServe(":8300", &BuildServ{})               // Run HTTP server for build hook
}

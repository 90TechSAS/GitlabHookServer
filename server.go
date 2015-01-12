package main

import (
	"./data"
	"bytes"
	"encoding/json"
	"fmt"
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
	verboseMode            = true                                                                            // Enable verbose mode
	slackURL               = "https://hooks.slack.com/services/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" // Slack API URL
	username               = "GitLabBot"                                                                     // Bot's name
	systemChannel          = "gitlabbot"                                                                     // Bot's system channel
	icon                   = ":heavy_exclamation_mark:"                                                      // Bot's icon (Slack emoji)
	currentBuildID float64 = 0                                                                               // Current build ID
	n              string  = "%5Cn"                                                                          // Encoded line return
	channelPrefix  string  = "dev-"                                                                          // Prefix on slack non system channel
)

/*
	Struc for HTTP servers
*/
type PushServ struct{}
type MergeServ struct{}
type BuildServ struct{}

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
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 30 * time.Second,
	}

	res, err = client.Do(req)
	if err != nil {
		fmt.Println("Error : Curl POST : " + err.Error())
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
		fmt.Println("Error : Curl POST body read : " + err.Error())
	}

	return res.StatusCode, string(body)
}

/*
	Create a Slack channel

	@param chanName : The Slack channel name (without the #)
*/
func CreateSlackChannel(chanName string) {
	// Variables
	var err error                                                     // Error catching
	var url string = "https://slack.com/api/channels.join?token="     // Token API url
	var token string = "xoxp-2874720296-3008670361-3035239562-5f7efd" // Slack token
	var supl string = "&name=" + chanName + "&pretty=1"               // Additional request
	var resp *http.Response                                           // Response

	// API Get
	resp, err = http.Get(url + token + supl)

	if err != nil {
		// Error
		fmt.Println("Error : CreateSlackChannel :", err, "\nResponse :", resp)
	} else {
		// Ok
		fmt.Println("CreateSlackChannel OK\nResponse :", resp)
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
func SendSlackMessage(channel, message string) {
	// Variables
	var payload string // POST data sent to slack

	// Insert prefix on non system channels
	if channel != systemChannel {
		channel = channelPrefix + channel
	}

	// Crop channel name if len(channel)>21
	if len(channel) > 21 {
		channel = channel[:21]
	}

	// Create channel if not exists
	CreateSlackChannel(channel)

	// POST Payload formating
	payload = "payload="
	payload += `{"channel": "#` + strings.ToLower(channel) + `", "username": "` + username + `", "text": "` + message + `", "icon_emoji": "` + icon + `"}`
	code, body := Post(slackURL, payload)
	if code != 200 {
		fmt.Println("ERROR:\n", body)
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

	// Read http request body and put it in a string
	buffer.ReadFrom(r.Body)
	body = buffer.String()

	// Debug information
	if verboseMode {
		fmt.Println("JsonString receive =", body)
	}

	// Parse json and put it in a the data.Build structure
	err = json.Unmarshal([]byte(body), &j)
	if err != nil {
		// Error
		fmt.Println("Error : Json parser failed :", err)
	} else {
		// Ok
		// Debug information
		if verboseMode {
			fmt.Println("JsonObject =", j)
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
		SendSlackMessage(strings.ToLower(j.Repository.Name), message)
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

	// Read http request body and put it in a string
	buffer.ReadFrom(r.Body)
	body = buffer.String()

	// Debug information
	if verboseMode {
		fmt.Println("JsonString receive =", body)
	}

	// Parse json and put it in a the data.Build structure
	err = json.Unmarshal([]byte(body), &j)
	if err != nil {
		// Error
		fmt.Println("Error : Json parser failed :", err)
	} else {
		// Ok
		// Debug information
		if verboseMode {
			fmt.Println("JsonObject =", j)
		}

		// Send the message

		// Date parsing (parsing result example : 18 November 2014 - 14:34)
		date, err = time.Parse("2006-01-02 15:04:05 UTC", j.Object_attributes.Created_at)
		var dateString = date.Format("02 Jan 06 15:04")

		// Message
		message += "[MERGE REQUEST " + strings.ToUpper(j.Object_attributes.State) + "] " + n + "Target : *" + j.Object_attributes.Target.Name + "/" + j.Object_attributes.Target_branch + "* Source : *" + j.Object_attributes.Source.Name + "/" + j.Object_attributes.Source_branch + "* : at *" + dateString + "* :" + n // First line
		message += "```" + MessageEncode(j.Object_attributes.Description) + "```"                                                                                                                                                                                                                                          // Third line (last commit message)
		SendSlackMessage(strings.ToLower(j.Object_attributes.Target.Name), message)
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

	// Read http request body and put it in a string
	buffer.ReadFrom(r.Body)
	body = buffer.String()

	// Debug information
	if verboseMode {
		fmt.Println("JsonString receive =", body)
	}

	// Parse json and put it in a the data.Build structure
	err = json.Unmarshal([]byte(body), &j)
	if err != nil {
		// Error
		fmt.Println("Error : Json parser failed :", err)
	} else {
		// Ok
		// Debug information
		if verboseMode {
			fmt.Println("JsonObject =", j)
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
			SendSlackMessage(strings.ToLower(j.Push_data.Repository.Name), message)
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
	SendSlackMessage(systemChannel, "GitLab SlackBot started and ready to party hard!") // Slack notification
	go http.ListenAndServe(":8100", &PushServ{})                                        // Run HTTP server for push hook
	go http.ListenAndServe(":8200", &MergeServ{})                                       // Run HTTP server for merge request hook
	http.ListenAndServe(":8300", &BuildServ{})                                          // Run HTTP server for build hook
}

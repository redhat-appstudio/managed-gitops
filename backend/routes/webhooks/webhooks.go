package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/emicklei/go-restful/v3"
	"github.com/google/go-github/github"
)

type WebHookInfo struct {
	Id        string // Id for the github webhook request
	Event     string // indicates which event took place (push, starred, pull request etc)
	Signature string // signature of the github webhook request
	Payload   []byte // consists of all the contents within the webhook
}

func ParseWebhookInfo(request *restful.Request, response *restful.Response) {
	webhook := new(WebHookInfo)
	if !strings.EqualFold(request.Request.Method, "POST") {
		log.Fatalln(nil, errors.New("POST method not found, unknown method occurred!"))
	}
	if webhook.Event = request.Request.Header.Get("x-github-event"); len(webhook.Event) == 0 {
		log.Fatalln(nil, errors.New("No event!"))
	}
	if webhook.Id = request.Request.Header.Get("x-github-delivery"); len(webhook.Id) == 0 {
		log.Fatalln(nil, errors.New("No event Id!"))
	}
	// assigning payload data
	payload, err := io.ReadAll(request.Request.Body)
	if err != nil {
		log.Printf("error reading request body: err=%s\n", err)
		return
	}
	webhook.Payload = payload
	defer request.Request.Body.Close()

	event, err := github.ParseWebHook(github.WebHookType(request.Request), webhook.Payload)
	if err != nil {
		log.Printf("could not parse webhook: err=%s\n", err)
		return
	}

	// printing the payload data in structured format
	webhookData, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		log.Printf("error, %v", err)
		return
	}

	errWrite := response.WriteEntity(event)
	if errWrite != nil {
		log.Fatal(errWrite)
	}

	// classifying the type of event
	switch e := event.(type) {
	case *github.PushEvent:
		// this is a commit push
		fmt.Println("A Commit event occurred")
		fmt.Println(string(webhookData))
	case *github.PullRequestEvent:
		// this is a pull request
		fmt.Println("A pull request has been created")
		fmt.Println(string(webhookData))
	case *github.WatchEvent:
		// if someone starred our repository
		if e.Action != nil && *e.Action == "starred" {
			fmt.Printf("%s starred repository %s\n",
				*e.Sender.Login, *e.Repo.FullName)
		}
		fmt.Println(string(webhookData))
	default:
		log.Printf("unknown event type %s\n", github.WebHookType(request.Request))
		return
	}
}

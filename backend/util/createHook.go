package util

import (
	"context"
	"fmt"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type TokenSource struct {
	AccessToken string
}

func (t *TokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}
	return token, nil
}

func CreateWebHook(personalAccessToken string, authorUsername string, authorName string, repoName string, hookURL string) {
	// OAuth Authentication
	tokenSource := &TokenSource{
		AccessToken: personalAccessToken,
	}
	oauthClient := oauth2.NewClient(context.TODO(), tokenSource)
	client := github.NewClient(oauthClient)
	user, _, err := client.Users.Get(context.Background(), authorUsername)
	if err != nil {
		fmt.Printf("client.Users.Get() faled with '%s'\n", err)
		return
	}
	fmt.Printf("Logged in via:\nUser Name: %s\nUser Email: %s\n", *user.Name, *user.Email)

	// To create a Github WebHook
	optsWebhook := &github.Hook{
		Name: github.String("web"),
		URL:  github.String(hookURL),
		Config: map[string]interface{}{
			"url":          hookURL,
			"content_type": "json",
		},
	}
	_, _, errHook := client.Repositories.CreateHook(context.Background(), authorUsername, repoName, optsWebhook)
	if errHook != nil {
		fmt.Println(errHook)
		return
	}
	fmt.Println("WebHook Created")
}

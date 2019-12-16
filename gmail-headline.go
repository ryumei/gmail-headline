package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"

	"github.com/BurntSushi/toml"
)

type Response struct {
	Messages           []Message `json:"messages"`
	NextPageToken      string    `json:"nextPageToken"`
	ResultSizeEstimate uint      `json:"resultSizeEstimate"`
}

type Message struct {
	Id           string   `json:"id"`
	ThreadId     string   `json:"threadId"`
	LabelIds     []string `json:"labelIds"`
	Snippet      string   `json:"snippet"`
	HistoryId    uint64   `json:"hostoryId"`
	SizeEstimate int      `json:"sizeEstimate"`
	Raw          string   `json:"raw"`
	Payload      Payload  `json:"payload"`
}

type Payload struct {
	Body Attachments `json:"body"`
}

type Attachments struct {
	AttachmentId string `json:attachmentId`
	Data         string `json:"data"`
	Size         int    `json:"size"`
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config, tokFile string) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	//tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

type Config struct {
	Gmail    GmailConfig
	Headline HeadlineConfig
}
type GmailConfig struct {
	TokenFile       string
	CredentialsFile string
}
type HeadlineConfig struct {
	Limit      uint
	OutputFile string
}

func main() {
	var conf Config
	_, err := toml.DecodeFile("gmail-headline.toml", &conf)
	if err != nil {
		log.Fatal(err)
	}

	b, err := ioutil.ReadFile(conf.Gmail.CredentialsFile)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, gmail.GmailModifyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config, conf.Gmail.TokenFile)

	srv, err := gmail.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Gmail client: %v", err)
	}

	user := "me"
	r, err := srv.Users.Labels.List(user).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve labels: %v", err)
	}
	if len(r.Labels) == 0 {
		fmt.Println("No labels found.")
		return
	}

	mes, err := srv.Users.Messages.List(user).Q("is:unread").Do()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	count := 0
	ids := []string{}
	for _, msg := range mes.Messages {
		//msg, _ := srv.Users.Messages.Get(user, msg.Id).Format("minimal").Do()
		msg, _ := srv.Users.Messages.Get(user, msg.Id).Format("metadata").Do()
		header, _ := msg.MarshalJSON()
		fmt.Println(string(header))

		ids = append(ids, msg.Id)

		count += 1
		if count > 10 {
			break
		}
	}

}

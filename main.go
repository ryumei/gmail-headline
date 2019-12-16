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

type ExcerptMessage struct {
	Metadata *gmail.Message      `json:metadata`
	Header   map[string][]string `json:header`
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config, tokFile string) *http.Client {
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

// Find takes a slice and looks for keys in it. If found it will
// return it's key, otherwise it will return -1 and a bool of false.
func find(keys []string, slice []string) (int, bool) {
	for i, item := range slice {
		for _, val := range keys {
			if item == val {
				return i, true
			}
		}
	}
	return -1, false
}

type Config struct {
	Gmail    GmailConfig
	Headline HeadlineConfig
}
type GmailConfig struct {
	TokenFile       string
	CredentialsFile string
	User            string
	SkipLabels      []string
}
type HeadlineConfig struct {
	Limit      int
	OutputFile string
}

func main() {
	var conf Config
	_, err := toml.DecodeFile("gmail-headline.toml", &conf)
	if err != nil {
		log.Fatalf("Unable to read config file: %v", err)
	}

	b, err := ioutil.ReadFile(conf.Gmail.CredentialsFile)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	gConfig, err := google.ConfigFromJSON(b, gmail.GmailModifyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(gConfig, conf.Gmail.TokenFile)

	// Retrieve messages from Gmail
	srv, err := gmail.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Gmail client: %v", err)
	}
	execute(srv, &conf.Gmail, &conf.Headline)
}

func execute(srv *gmail.Service, gmailConf *GmailConfig, headline *HeadlineConfig) {
	mes, err := srv.Users.Messages.List(gmailConf.User).Q("is:unread").Do()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	file, err := os.OpenFile(headline.OutputFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("Failed to open output file: %v", err)
	}
	defer file.Close()

	ids := []string{}
	for _, msgID := range mes.Messages {
		msg, _ := srv.Users.Messages.Get(gmailConf.User, msgID.Id).Format("metadata").Do()

		_, ok := find(gmailConf.SkipLabels, msg.LabelIds)
		if ok {
			continue
		}

		// Extract Payload.Headers
		header := map[string][]string{}
		for _, h := range msg.Payload.Headers {
			header[h.Name] = append(header[h.Name], h.Value)
		}
		msg.Payload.Headers = nil

		// Export a mail data
		excerpted := ExcerptMessage{Metadata: msg, Header: header}
		data, _ := json.Marshal(excerpted)
		fmt.Fprintln(file, string(data))

		// memo to mark as read
		ids = append(ids, msgID.Id)
		if len(ids) > headline.Limit {
			break
		}
	}

	// Mark as Read
	batchReq := gmail.BatchModifyMessagesRequest{
		RemoveLabelIds: []string{"UNREAD"},
		Ids:            ids,
	}
	err = srv.Users.Messages.BatchModify(gmailConf.User, &batchReq).Do()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

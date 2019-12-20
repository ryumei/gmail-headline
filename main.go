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

// ExcerptMessage is structure for each email
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
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0640)
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

// Config is a set of configurations
type Config struct {
	Gmail    GmailConfig
	Headline HeadlineConfig
}

// GmailConfig set Gmail API
type GmailConfig struct {
	TokenFile          string
	CredentialsFile    string
	User               string
	RetrieveConditions []string
	DeleteConditions   []string
}

// HeadlineConfig set client behavior
type HeadlineConfig struct {
	Limit      int
	OutputFile string
}

func httpClient(credentialsPath string, tokenPath string) *http.Client {
	b, err := ioutil.ReadFile(credentialsPath)
	if err != nil {
		log.Fatalf("[ERROR] Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	gConfig, err := google.ConfigFromJSON(b, gmail.MailGoogleComScope)
	if err != nil {
		log.Fatalf("[ERROR] Unable to parse client secret file to config: %v", err)
	}
	return getClient(gConfig, tokenPath)
}

func retrieveMessage(srv *gmail.Service, user string, msgID string) ExcerptMessage {
	msg, _ := srv.Users.Messages.Get(user, msgID).Format("metadata").Do()

	// Extract Payload.Headers
	header := map[string][]string{}
	for _, h := range msg.Payload.Headers {
		header[h.Name] = append(header[h.Name], h.Value)
	}
	msg.Payload.Headers = nil

	return ExcerptMessage{Metadata: msg, Header: header}
}

func readMessages(srv *gmail.Service, user string, queries []string, outPath string, limit int) {
	file, err := os.OpenFile(outPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("[ERROR] Failed to open output file: %v", err)
	}
	defer file.Close()

	readIDs := []string{}
	for _, query := range queries {
		log.Printf("[INFO] Retrieve q: %v ", query)
		mes, err := srv.Users.Messages.List(user).Q(query).Do()
		if err != nil {
			log.Fatalf("[ERROR] %v", err)
		}

		for _, msgID := range mes.Messages {
			excerpted := retrieveMessage(srv, user, msgID.Id)
			data, _ := json.Marshal(excerpted)
			fmt.Fprintln(file, string(data))

			// memo to mark as read
			readIDs = append(readIDs, msgID.Id)
			if len(readIDs) >= limit {
				log.Println("[INFO] Exceeded retrieve limit per execution.")
				break
			}
		}
		log.Printf("[INFO] Retrieved %d mails.", len(readIDs))
	}

	// Mark as Read
	modReq := gmail.BatchModifyMessagesRequest{
		RemoveLabelIds: []string{"UNREAD"},
		Ids:            readIDs,
	}
	err = srv.Users.Messages.BatchModify(user, &modReq).Do()
	if err != nil {
		log.Fatalf("[Error] %v", err)
	}
	log.Printf("[INFO] Change %d mails to READ.", len(readIDs))
}

func deleteMessages(srv *gmail.Service, user string, queries []string) {
	for _, query := range queries {
		log.Printf("[INFO] Delete q: %v ", query)
		mes, err := srv.Users.Messages.List(user).Q(query).Do()
		if err != nil {
			log.Fatalf("[Error] %v", err)
		}
		if len(mes.Messages) < 1 {
			log.Println("[INFO] No mail found. Skip.")
			continue
		}
		delIDs := []string{}
		for _, msgID := range mes.Messages {
			delIDs = append(delIDs, msgID.Id)
		}

		delReq := gmail.BatchDeleteMessagesRequest{
			Ids: delIDs,
		}
		err = srv.Users.Messages.BatchDelete(user, &delReq).Do()
		if err != nil {
			log.Fatalf("[Error] %v", err)
		}
		log.Printf("[INFO] Deleted %d mails.", len(delIDs))
	}
}

func main() {
	var conf Config
	_, err := toml.DecodeFile("gmail-headline.toml", &conf)
	if err != nil {
		log.Fatalf("[ERROR] Unable to read config file: %v", err)
	}

	client := httpClient(conf.Gmail.CredentialsFile, conf.Gmail.TokenFile)
	srv, err := gmail.New(client)
	if err != nil {
		log.Fatalf("[ERROR] Unable to retrieve Gmail client: %v", err)
	}

	readMessages(srv, conf.Gmail.User, conf.Gmail.RetrieveConditions,
		conf.Headline.OutputFile, conf.Headline.Limit)

	deleteMessages(srv, conf.Gmail.User, conf.Gmail.DeleteConditions)
}

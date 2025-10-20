package main

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"golang.org/x/net/idna"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

func getClient() (*gmail.Service, error) {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		return nil, err
	}

	config, err := google.ConfigFromJSON(b, gmail.GmailSendScope)
	if err != nil {
		return nil, err
	}

	tokFile := "token.json"
	var token *oauth2.Token
	if _, err := os.Stat(tokFile); err == nil {
		f, _ := os.Open(tokFile)
		defer f.Close()
		token = &oauth2.Token{}
		json.NewDecoder(f).Decode(token)
	} else {
		authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
		fmt.Printf("Go to the following link in your browser then paste the authorization code:\n%v\n", authURL)
		var code string
		fmt.Print("Enter code: ")
		fmt.Scan(&code)
		token, err = config.Exchange(ctx, code)
		if err != nil {
			return nil, err
		}
		f, _ := os.Create(tokFile)
		defer f.Close()
		json.NewEncoder(f).Encode(token)
	}

	client := config.Client(ctx, token)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	return srv, nil
}

func loadTemplates(dir string) (map[string]string, error) {
	templates := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".html")
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		templates[name] = string(content)
	}

	return templates, nil
}

func createMessage(from, to, subject, htmlBody string) *gmail.Message {
	/*msg := []byte(
		fmt.Sprintf("From: %s\r\n", from) +
			fmt.Sprintf("To: %s\r\n", to) +
			fmt.Sprintf("Subject: %s\r\n", subject) +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n" +
			htmlBody,
	)

	return &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString(msg),
	}*/

	parts := strings.Split(to, "@")
	if len(parts) != 2 {
		log.Printf("Invalid email format: %s", to)
		return nil
	}
	local := parts[0]
	domainASCII, err := idna.ToASCII(parts[1])
	if err != nil {
		log.Printf("Failed to encode domain: %s", parts[1])
		return nil
	}
	toHeader := fmt.Sprintf("<%s@%s>", local, domainASCII)

	msg := []byte(
		fmt.Sprintf("From: %s\r\n", from) +
			fmt.Sprintf("To: %s\r\n", toHeader) +
			fmt.Sprintf("Subject: %s\r\n", subject) +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n" +
			htmlBody,
	)

	return &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString(msg),
	}
}

func main() {
	fmt.Print("Enter your email address: ")
	var from string
	_, err := fmt.Scanln(&from)
	if err != nil {
		log.Fatal(err)
	}

	from = strings.TrimSpace(from)
	subject := mime.QEncoding.Encode("utf-8", "UUS ALBUM! Teravmoon - Tee")

	service, err := getClient()
	if err != nil {
		log.Fatalf("Failed to create Gmail client: %v", err)
	}

	templates, err := loadTemplates("templates")
	if err != nil {
		log.Fatalf("Failed to load templates: %v", err)
	}

	file, err := os.Open("recipients.csv")
	if err != nil {
		log.Fatalf("Failed to open CSV: %v", err)
	}
	defer file.Close()

	decodedReader := transform.NewReader(file, charmap.Windows1257.NewDecoder())
	reader := csv.NewReader(decodedReader)

	//reader := csv.NewReader(file)
	reader.Comma = ';'
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("Failed to read CSV: %v", err)
	}

	for _, record := range records {
		if len(record) != 3 {
			log.Printf("Skipping invalid row: %v", record)
			continue
		}
		email := strings.TrimSpace(record[0])
		templateID := strings.TrimSpace(record[1])
		recipientName := strings.TrimSpace(record[2])

		body, ok := templates[templateID]
		if !ok {
			log.Printf("Template not found: %s", templateID)
			continue
		}
		personalizedBody := strings.ReplaceAll(body, "{{name}}", recipientName)

		msg := createMessage(from, email, subject, personalizedBody)
		_, err := service.Users.Messages.Send("me", msg).Do()
		if err != nil {
			log.Printf("Failed to send email to %s: %v", email, err)
		} else {
			fmt.Printf("Email sent to %s using template %s\n", email, templateID)
		}
	}
}

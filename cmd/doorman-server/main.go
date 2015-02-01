package main

import (
	"log"
	"net/http"
	"os"

	"github.com/rakyll/statik/fs"

	"github.com/kelvl/doorman"
	_ "github.com/kelvl/doorman/statik"
)

const timeLayout = "18 Dec, 4:30pm"

func fatalIfEmpty(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Environmental Variable %s cannot be empty", key)
	}
	return val
}

func main() {
	accountSid := fatalIfEmpty("TWILIO_ACCOUNT_SID")
	authToken := fatalIfEmpty("TWILIO_AUTH_TOKEN")
	phoneNumber := fatalIfEmpty("PHONE_NUMBER")
	baseUrl := fatalIfEmpty("BASE_URL")

	doorman := doorman.NewDoorman(accountSid, authToken, phoneNumber, timeLayout, baseUrl)
	statikFS, err := fs.New()

	if err != nil {
		log.Fatalf(err.Error())
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(statikFS)))
	mux.HandleFunc("/open", doorman.Open)
	mux.HandleFunc("/door", doorman.Door)
	mux.HandleFunc("/callme", doorman.CallMe)
	mux.HandleFunc("/sms", doorman.Sms)

	log.Println("Listening on 5000...")
	log.Fatal(http.ListenAndServe(":5000", mux))
}

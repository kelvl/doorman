package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rakyll/statik/fs"

	"github.com/kelvl/doorman"
	_ "github.com/kelvl/doorman/statik"
)

const timeLayout = "2 Jan, 3:04pm"

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
	port := fatalIfEmpty("PORT")

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
	mux.HandleFunc("/dummy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "nothing")
	})

	fmt.Println("Starting scheduler")
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				resp, _ := http.Get(fmt.Sprintf("%s/dummy", baseUrl))
				defer resp.Body.Close()
			}
		}
	}()

	log.Printf("Listening on %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

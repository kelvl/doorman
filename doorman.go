package doorman

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kelvl/doorman/twilio"
)

type Doorman struct {
	TwilioClient *twilio.TwilioClient
	PhoneNumber  string
	TimeLayout   string
	OpenStart    time.Time
	OpenEnd      time.Time
	CallSid      string
	BaseUrl      string
}

const (
	dateLayout = "Sun 2 Jan, 3:04pm"
)

func parseTimes(str string) (t time.Time, err error) {
	formats := []string{"1504", "3:04pm", "3:04 pm", "3pm"}
	for _, format := range formats {
		p, e := time.Parse(format, str)
		if e == nil {
			cur := time.Now()
			t = time.Date(cur.Year(), cur.Month(), cur.Day(), p.Hour(), p.Minute(), p.Second(), p.Nanosecond(), cur.Location())
			err = e
			return
		}
	}
	err = errors.New("Can't parse string as time: " + str)
	return
}

func mustParseTimes(str string) (t time.Time) {
	t, err := parseTimes(str)
	if err != nil {
		panic(err)
	}
	return t
}

func (d *Doorman) messageUrl() string {
	return fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", d.TwilioClient.AccountSid)
}

func (d *Doorman) callUrl() string {
	return fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls/%s.json", d.TwilioClient.AccountSid, d.CallSid)
}

func NewDoorman(accountSid, authToken, phoneNumber, timeLayout, baseUrl string) *Doorman {
	client := twilio.NewTwilioClient(accountSid, authToken)
	doorman := &Doorman{TwilioClient: client, PhoneNumber: phoneNumber, TimeLayout: timeLayout, BaseUrl: baseUrl}
	return doorman
}

func (d *Doorman) Open(w http.ResponseWriter, r *http.Request) {
	log.Println("Opening Door")

	res, err := d.TwilioClient.PostForm(d.messageUrl(), url.Values{
		"From": {r.FormValue("Called")},
		"To":   {d.PhoneNumber},
		"Body": {fmt.Sprintf("Gate was opened at %s", time.Now().Format(dateLayout))},
	})

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer res.Body.Close()
	contents, _ := ioutil.ReadAll(res.Body)
	log.Printf("twilio returned %v - %s", res, string(contents))

	// generate the ringback twiml and return it
	twiml := fmt.Sprintf(`
        <Response>
            <Pause length="1"></Pause>
            <Play loop="2">%s</Play>
        </Response>        
    `, "/static/9.wav")
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, twiml)

	d.CallSid = ""

	return
}

func (d *Doorman) cleanUpTimes() {
	if d.OpenStart.Before(time.Now()) && d.OpenEnd.Before(time.Now()) {
		d.OpenStart = time.Time{}
		d.OpenEnd = time.Time{}
	}
}

func (d *Doorman) Door(w http.ResponseWriter, r *http.Request) {
	// update the current call sid
	callSid := r.FormValue("CallSid")
	d.CallSid = callSid

	// check if the door is already opened
	// if yes then just open the door and return
	if time.Now().After(d.OpenStart) && time.Now().Before(d.OpenEnd) {
		log.Println("Door is marked open, automatically opening door.")
		d.Open(w, r)
		return
	}

	// do any clean up of times
	d.cleanUpTimes()

	// send notification to the user and return ringback tone to keep the user occupied
	res, err := d.TwilioClient.PostForm(d.messageUrl(), url.Values{
		"From": {r.FormValue("Called")},
		"To":   {d.PhoneNumber},
		"Body": {fmt.Sprintf("%s - Someone is at the gate. 1 to open, 2 to talk to the person.", time.Now().Format(dateLayout))},
	})

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if res.StatusCode != 201 {
		log.Println("Received invalid response", res)
	}

	// generate the ringback twiml and return it
	twiml := fmt.Sprintf(`
        <Response>
            <Play loop="5">%s</Play>
        </Response>
    `, "/static/us_ringback_tone.mp3")
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, twiml)
	return
}

func (d *Doorman) CallMe(w http.ResponseWriter, r *http.Request) {
	twiml := fmt.Sprintf(`
        <Response>
            <Dial>%s</Dial>
        </Response>
    `, d.PhoneNumber)
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, twiml)
}

func (d *Doorman) Sms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	response := strings.TrimSpace(r.FormValue("Body"))

	d.cleanUpTimes()

	log.Printf("%q - %q", response, d.CallSid)

	switch {
	case response == "1" && d.CallSid != "":

		res, err := d.TwilioClient.PostForm(d.callUrl(), url.Values{
			"Url":    {fmt.Sprintf("%s/open", d.BaseUrl)},
			"Method": {"GET"},
		})

		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer res.Body.Close()
		contents, _ := ioutil.ReadAll(res.Body)
		log.Printf("twilio returned %v - %s", res, string(contents))

		return

	case response == "2" && d.CallSid != "":
		res, err := d.TwilioClient.PostForm(d.callUrl(), url.Values{
			"Url":    {fmt.Sprintf("%s/callme", d.BaseUrl)},
			"Method": {"GET"},
		})

		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer res.Body.Close()
		contents, _ := ioutil.ReadAll(res.Body)
		log.Printf("twilio returned %v - %s", res, string(contents))

		return

	case response == "status":
		fmt.Fprint(w, d.ruleStatus())
		return

	case strings.HasPrefix(response, "open"):
		openRegex := regexp.MustCompile("^open from (.+?) to (.+)$")
		switch {
		case strings.HasPrefix(response, "open for"):
			duration, err := time.ParseDuration(response[len("open for "):])
			if err != nil {
				log.Printf("Unable to parse duration from %v", err)
				return
			}

			d.OpenStart = time.Now()
			d.OpenEnd = time.Now().Add(duration)
			fmt.Fprint(w, d.ruleStatus())
			return

		case strings.HasPrefix(response, "open from"):
			matches := openRegex.FindStringSubmatch(response)

			cur := time.Now()
			start := mustParseTimes(matches[1])
			end := mustParseTimes(matches[2])

			log.Printf("Matched times %v - %v", start.Format("2006-01-02 15:04:05 MST"), end.Format("2006-01-02 15:04:05 MST"))

			if start.Before(cur) {
				newStart := start.AddDate(0, 0, 1)
				log.Printf("Start Time %v is before now %v, Adding 1 day to start time %v", start.Format(dateLayout), cur.Format(dateLayout), newStart.Format(dateLayout))
				start = newStart
			}
			if end.Before(start) {
				end = end.AddDate(0, 0, 1)
			}
			d.OpenStart = start
			d.OpenEnd = end
			fmt.Fprint(w, d.ruleStatus())
			return

		}
		fmt.Fprint(w, "unimplemented")
		return

	case response == "clear":
		d.OpenStart = time.Time{}
		d.OpenEnd = time.Time{}
		fmt.Fprint(w, d.ruleStatus())
		return

	default:
		help := `Invalid command: "%s"
Valid commands: open from, open for, status, clear`
		fmt.Fprintf(w, help, response)
		return
	}

}

func (d *Doorman) ruleStatus() string {
	log.Printf("Rules: Start %v End %v", d.OpenStart.Format(dateLayout), d.OpenEnd.Format(dateLayout))
	if !d.OpenStart.IsZero() && d.OpenStart.Before(time.Now()) {
		return fmt.Sprintf("Gate is open until %s", d.OpenEnd.Format(dateLayout))
	} else if !d.OpenStart.IsZero() && d.OpenStart.After(time.Now()) {
		return fmt.Sprintf("Gate is open from %s to %s", d.OpenStart.Format(dateLayout), d.OpenEnd.Format(dateLayout))
	} else {
		return "No rules defined"
	}
}

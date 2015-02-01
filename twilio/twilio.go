package twilio

import "net/http"
import "net/url"
import "strings"

type TwilioClient struct {
	AccountSid string
	AuthToken  string
	Client     *http.Client
}

func NewTwilioClient(accountSid string, authToken string) *TwilioClient {
	httpClient := &http.Client{}
	client := &TwilioClient{accountSid, authToken, httpClient}
	return client
}

func (c *TwilioClient) PostForm(url string, data url.Values) (resp *http.Response, err error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.AccountSid, c.AuthToken)
	return c.Client.Do(req)
}

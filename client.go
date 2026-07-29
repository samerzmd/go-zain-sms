// Package zainsms is a client for the Zain Jordan Integrated SMS service
// (zsms.jo.zain.com).
//
// The API is a two-step flow: generate an integration token with your
// username/password, then send messages with that token. The client handles
// this transparently — it lazily generates the token on first send, caches
// it, and refreshes it once if the gateway reports invalid authentication.
package zainsms

import (
	"net/http"
	"sync"
)

// DefaultBaseURL is the production Zain Integrated SMS endpoint.
const DefaultBaseURL = "https://zsms.jo.zain.com"

// Config holds credentials for the Zain Integrated SMS service.
type Config struct {
	BaseURL  string // defaults to DefaultBaseURL when empty
	Username string // account username, e.g. "96279XXXXXXX"
	Password string
	SenderID string // pre-approved sender ID
}

// Client talks to the Zain Integrated SMS REST API.
type Client struct {
	Config     Config
	HTTPClient *http.Client

	mu    sync.Mutex
	token string
}

// NewClient creates a new Zain Integrated SMS client. Pass nil to use a
// default http.Client.
func NewClient(cfg Config, httpClient *http.Client) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{Config: cfg, HTTPClient: httpClient}
}

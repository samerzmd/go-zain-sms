package zainsms

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	tokenPath = "/core/user/rest/user/generateintegrationtoken"
	sendPath  = "/core/corpsms/sendNow"
	otpPath   = "/core/corpsms/sendOTP"
)

// --- API payloads ---

type tokenResponse struct {
	Status string `json:"status"`
	Result struct {
		IntegrationToken string `json:"integration_token"`
		AccountSid       string `json:"accountSid"`
	} `json:"result"`
}

type sendRequest struct {
	ServiceType          string   `json:"service_type"`
	RecipientNumbersType string   `json:"recipient_numbers_type"`
	PhoneNumbers         []string `json:"phone_numbers"`
	Content              string   `json:"content"`
	SenderID             string   `json:"sender_id"`
}

// SendResult mirrors the "result" object of a successful send response.
type SendResult struct {
	ValidNumbersCount   int      `json:"valid_numbers_count"`
	InvalidNumbers      []string `json:"invalid_numbers"`
	InvalidNumbersCount int      `json:"invalid_numbers_count"`
	TotalMessages       int      `json:"total_messages"`
}

type sendResponse struct {
	Status string     `json:"status"`
	Result SendResult `json:"result"`
}

// --- API methods ---

// GenerateToken requests a fresh integration token using the configured
// username and password. Most callers never need this directly: the send
// methods obtain and cache a token automatically.
func (c *Client) GenerateToken() (string, error) {
	req, err := http.NewRequest(http.MethodPost, c.Config.BaseURL+tokenPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("username", c.Config.Username)
	req.Header.Set("password", c.Config.Password)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("zainsms: token response is not JSON (HTTP %d): %s", resp.StatusCode, truncate(body))
	}
	if parsed.Status != "success" || parsed.Result.IntegrationToken == "" {
		return "", fmt.Errorf("zainsms: token generation failed: %s", parsed.Status)
	}
	return parsed.Result.IntegrationToken, nil
}

// Send delivers one message to the given numbers via the general sendNow
// endpoint.
func (c *Client) Send(numbers []string, message string) (*SendResult, error) {
	return c.send(sendPath, numbers, message)
}

// SendSingle delivers one message to one number via the general sendNow
// endpoint.
func (c *Client) SendSingle(number, message string) (*SendResult, error) {
	return c.send(sendPath, []string{number}, message)
}

// SendOtp delivers an OTP message to one number via the dedicated sendOTP
// endpoint.
func (c *Client) SendOtp(number, message string) (*SendResult, error) {
	return c.send(otpPath, []string{number}, message)
}

// send normalizes the numbers (spaces and the international "+"/"00" prefix
// removed — the gateway rejects them otherwise) and posts to the given send
// endpoint. The cached integration token is refreshed once if the gateway
// reports invalid authentication.
func (c *Client) send(path string, numbers []string, message string) (*SendResult, error) {
	normalized := make([]string, 0, len(numbers))
	for _, number := range numbers {
		if n := NormalizeNumber(number); n != "" {
			normalized = append(normalized, n)
		}
	}
	if len(normalized) == 0 {
		return nil, errors.New("zainsms: no phone numbers to send to")
	}

	token, err := c.getToken(false)
	if err != nil {
		return nil, err
	}

	result, badAuth, err := c.sendOnce(path, token, normalized, message)
	if badAuth {
		if token, err = c.getToken(true); err != nil {
			return nil, err
		}
		result, _, err = c.sendOnce(path, token, normalized, message)
	}
	return result, err
}

// sendOnce performs one send call. badAuth reports that the gateway rejected
// the integration token, so the caller may refresh and retry.
func (c *Client) sendOnce(path, token string, numbers []string, message string) (result *SendResult, badAuth bool, err error) {
	// service_type and recipient_numbers_type are fixed by the API and must
	// not be changed.
	payload, err := json.Marshal(sendRequest{
		ServiceType:          "bulk_sms",
		RecipientNumbersType: "single_numbers",
		PhoneNumbers:         numbers,
		Content:              message,
		SenderID:             c.Config.SenderID,
	})
	if err != nil {
		return nil, false, err
	}

	req, err := http.NewRequest(http.MethodPost, c.Config.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("integration_token", token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	var parsed sendResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, fmt.Errorf("zainsms: send response is not JSON (HTTP %d): %s", resp.StatusCode, truncate(body))
	}
	if strings.Contains(parsed.Status, "invalid authentication") {
		return nil, true, fmt.Errorf("zainsms: %s", parsed.Status)
	}
	if parsed.Result.ValidNumbersCount < 1 {
		return nil, false, fmt.Errorf("zainsms: send rejected: %s", parsed.Status)
	}
	return &parsed.Result, false, nil
}

// getToken returns the cached integration token, generating (or, with
// forceRefresh, regenerating) it as needed.
func (c *Client) getToken(forceRefresh bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && !forceRefresh {
		return c.token, nil
	}
	token, err := c.GenerateToken()
	if err != nil {
		return "", err
	}
	c.token = token
	return token, nil
}

// NormalizeNumber removes spaces and the international dialing prefix —
// "+962790000001", "00962790000001" and "962 79 0000001" all become
// "962790000001", which is the only format the gateway accepts.
func NormalizeNumber(number string) string {
	n := strings.ReplaceAll(number, " ", "")
	n = strings.TrimPrefix(n, "+")
	n = strings.TrimPrefix(n, "00")
	return n
}

func truncate(body []byte) string {
	const max = 200
	s := strings.TrimSpace(string(body))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

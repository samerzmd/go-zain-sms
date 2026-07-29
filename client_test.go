package zainsms

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func tokenSuccess(token string) map[string]any {
	return map[string]any{
		"status": "success",
		"result": map[string]string{"integration_token": token, "accountSid": "sid-ignored"},
	}
}

func sendSuccess(count int) map[string]any {
	return map[string]any{
		"status": "saved successfully!",
		"result": map[string]any{
			"valid_numbers_count":   count,
			"invalid_numbers":       []string{},
			"invalid_numbers_count": 0,
			"total_messages":        count,
		},
	}
}

func TestSendGeneratesTokenAndSends(t *testing.T) {
	tokenCalls, sendCalls := 0, 0

	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		if r.Method != http.MethodPost {
			t.Errorf("token request method = %s, want POST", r.Method)
		}
		if r.Header.Get("username") != "corpuser" || r.Header.Get("password") != "secret" {
			t.Error("credentials were not sent as username/password headers")
		}
		json.NewEncoder(w).Encode(tokenSuccess("tok-1"))
	})
	mux.HandleFunc(sendPath, func(w http.ResponseWriter, r *http.Request) {
		sendCalls++
		if r.Header.Get("integration_token") != "tok-1" {
			t.Errorf("integration_token header = %q, want tok-1", r.Header.Get("integration_token"))
		}
		if r.Header.Get("content-type") != "application/json" {
			t.Errorf("content-type = %q, want application/json", r.Header.Get("content-type"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["service_type"] != "bulk_sms" || body["recipient_numbers_type"] != "single_numbers" {
			t.Errorf("fixed API parameters were changed: %v", body)
		}
		if body["sender_id"] != "Evo App" {
			t.Errorf("sender_id = %v, want Evo App", body["sender_id"])
		}
		numbers := body["phone_numbers"].([]any)
		if len(numbers) != 1 || numbers[0] != "962790000001" {
			t.Errorf("phone_numbers = %v, want [962790000001]", numbers)
		}
		json.NewEncoder(w).Encode(sendSuccess(1))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: "corpuser", Password: "secret", SenderID: "Evo App"}, server.Client())
	result, err := client.SendSingle("+962 79 0000001", "Your code is 1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ValidNumbersCount != 1 || result.TotalMessages != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if tokenCalls != 1 || sendCalls != 1 {
		t.Fatalf("expected 1 token call and 1 send call, got %d and %d", tokenCalls, sendCalls)
	}
}

func TestSendReusesCachedToken(t *testing.T) {
	tokenCalls := 0

	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		json.NewEncoder(w).Encode(tokenSuccess("tok-1"))
	})
	mux.HandleFunc(sendPath, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sendSuccess(1))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: "u", Password: "p", SenderID: "s"}, server.Client())
	for i := 0; i < 3; i++ {
		if _, err := client.SendSingle("962790000001", "hi"); err != nil {
			t.Fatalf("send %d failed: %v", i, err)
		}
	}
	if tokenCalls != 1 {
		t.Fatalf("expected token generated once and cached, got %d calls", tokenCalls)
	}
}

func TestSendRefreshesTokenOnInvalidAuth(t *testing.T) {
	tokenCalls, sendCalls := 0, 0

	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		if tokenCalls == 1 {
			json.NewEncoder(w).Encode(tokenSuccess("tok-stale"))
		} else {
			json.NewEncoder(w).Encode(tokenSuccess("tok-fresh"))
		}
	})
	mux.HandleFunc(sendPath, func(w http.ResponseWriter, r *http.Request) {
		sendCalls++
		if r.Header.Get("integration_token") != "tok-fresh" {
			json.NewEncoder(w).Encode(map[string]any{"status": "invalid authentication!", "result": map[string]any{}})
			return
		}
		json.NewEncoder(w).Encode(sendSuccess(1))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: "u", Password: "p", SenderID: "s"}, server.Client())
	result, err := client.SendSingle("962790000001", "hi")
	if err != nil {
		t.Fatalf("expected refresh-and-retry to succeed, got: %v", err)
	}
	if result.ValidNumbersCount != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if tokenCalls != 2 || sendCalls != 2 {
		t.Fatalf("expected 2 token calls and 2 send calls, got %d and %d", tokenCalls, sendCalls)
	}
}

func TestSendReportsRejection(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tokenSuccess("tok-1"))
	})
	mux.HandleFunc(sendPath, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "no valid numbers found", "result": map[string]any{}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: "u", Password: "p", SenderID: "s"}, server.Client())
	_, err := client.SendSingle("962790000001", "hi")
	if err == nil || !strings.Contains(err.Error(), "no valid numbers found") {
		t.Fatalf("expected rejection error, got: %v", err)
	}
}

func TestTokenGenerationFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "invalid authentication!", "result": map[string]any{}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: "u", Password: "wrong", SenderID: "s"}, server.Client())
	if _, err := client.SendSingle("962790000001", "hi"); err == nil {
		t.Fatal("expected token generation error")
	}
}

func TestNormalizeNumber(t *testing.T) {
	cases := map[string]string{
		"+962790000001":   "962790000001",
		"00962790000001":  "962790000001",
		"962 79 000 0001": "962790000001",
		"962790000001":    "962790000001",
		"0791234567":      "0791234567", // local format left untouched
	}
	for input, want := range cases {
		if got := NormalizeNumber(input); got != want {
			t.Errorf("NormalizeNumber(%q) = %q, want %q", input, got, want)
		}
	}
}

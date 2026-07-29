# go-zain-sms

Go client for the **Zain Jordan Integrated SMS** service (`zsms.jo.zain.com`).

The API is a two-step flow — generate an integration token with your
username/password, then send with that token. This client handles it
transparently: the token is generated lazily on first send, cached, and
refreshed once automatically if the gateway reports invalid authentication.

## Install

```bash
go get github.com/samerzmd/go-zain-sms
```

## Usage

```go
package main

import (
	"fmt"

	zainsms "github.com/samerzmd/go-zain-sms"
)

func main() {
	client := zainsms.NewClient(zainsms.Config{
		Username: "96279XXXXXXX",
		Password: "your-password",
		SenderID: "Your Sender ID",
	}, nil) // pass a custom *http.Client or nil for the default

	result, err := client.SendSingle("+962790000001", "Hello World")
	if err != nil {
		panic(err)
	}
	fmt.Printf("sent %d message(s)\n", result.TotalMessages)
}
```

`Send(numbers []string, message string)` delivers to multiple recipients in
one call. Phone numbers are normalized automatically (spaces and the
international `+`/`00` prefix are stripped — the gateway accepts only the
bare `962...` format).

## Notes

- `service_type` (`bulk_sms`) and `recipient_numbers_type` (`single_numbers`)
  are fixed by the API and set by the client; they must not be changed.
- A send is considered successful when the gateway reports at least one valid
  number; otherwise the response status is returned as an error.

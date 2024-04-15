package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"
)

/*
The client calls the server on every batchInterval with batchCount requests and prints the responses.
*/

const (
	batchInterval = time.Second // how often to call server
	batchCount    = 10          // how many requests in a batch
)

func main() {
	ticker := time.NewTicker(batchInterval)

	batchID := 0

	for {
		for i := 0; i < batchCount; i++ {
			go func(b, n int) {
				resp, err := doRequest()
				if err != nil {
					log.Printf("ERROR: %s", err)
				} else {
					log.Printf("RESPONSE: %s", resp)
				}
			}(batchID, i)
		}

		batchID++
		<-ticker.C
	}

}

func doRequest() (string, error) {
	resp, err := http.Get("http://localhost:8080")
	if err != nil {
		return "", fmt.Errorf("calling server: %w", err)
	}

	defer resp.Body.Close()

	buf := bytes.Buffer{}
	if _, err = buf.ReadFrom(resp.Body); err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	return fmt.Sprintf("[%s] %s", resp.Status, buf.String()), nil
}

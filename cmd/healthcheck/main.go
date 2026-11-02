// healthcheck performs a bounded, database-backed API readiness query.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func check(client *http.Client, url string) error {
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	var result struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&result); err != nil {
		return err
	}
	if len(result.Data) == 0 {
		return fmt.Errorf("empty dataset")
	}
	return nil
}

func main() {
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if check(client, "http://127.0.0.1:8080/v1/regions") != nil {
		os.Exit(1)
	}
}

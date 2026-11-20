package utils

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// SendMECRequest sends an HTTP request to a MEC platform endpoint
//
// This function creates and sends an HTTP request to a MEC platform API endpoint.
// It handles JSON payload marshaling for POST/PUT requests and sets appropriate
// headers for MEC API communication. TLS verification is disabled for development/testing.
// Timeouts, connection errors and 408/429/5xx responses are retried with
// exponential backoff and jitter (see doWithRetry).
//
// @param method HTTP method (GET, POST, PUT, DELETE, etc.)
// @param url The complete URL endpoint for the MEC API request
// @param payload The payload to send (will be marshaled to JSON for non-GET requests)
// @return *http.Response The HTTP response from the MEC platform
// @return error Returns nil on success, or an error if the request fails
func SendMECRequest(method, url string, payload interface{}) (*http.Response, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// Marshal the payload once; the request is rebuilt on each retry attempt
	var reqBody []byte
	if !(method == "GET" && payload == nil) {
		var err error
		reqBody, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %v", err)
		}
		log.Println("Request body:", string(reqBody))
	}

	// Transient failures are retried with jittered backoff (see retry.go)
	return doWithRetry(client, func() (*http.Request, error) {
		var body io.Reader
		if reqBody != nil {
			body = bytes.NewReader(reqBody)
		}
		req, err := http.NewRequestWithContext(context.Background(), method, url, body)
		if err != nil {
			return nil, err
		}
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Go-http-client/1.1")
		return req, nil
	})
}

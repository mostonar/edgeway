package utils

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CSE HTTP Basic auth credentials; when cseUser is empty no Authorization header is sent
var cseUser, csePassword string

// SetCSECredentials sets the HTTP Basic auth credentials used for all oneM2M requests
func SetCSECredentials(user, password string) {
	cseUser = user
	csePassword = password
}

// MakeOneM2MRequest creates and sends a oneM2M HTTP request
//
// This function creates and sends an HTTP request to a oneM2M CSE platform endpoint.
// It sets the required oneM2M headers including X-M2M-Origin, X-M2M-RI (request ID),
// X-M2M-RVI (release version indicator), and Content-Type with resource type (ty).
// Timeouts, connection errors and 408/429/5xx responses are retried with
// exponential backoff and jitter (see doWithRetry).
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param method HTTP method (GET, POST, PUT, DELETE, etc.)
// @param url The complete URL endpoint for the oneM2M API request
// @param body The request body as a string (JSON formatted)
// @param ty Resource type code for oneM2M resources (e.g., 2 for AE, 3 for Container)
// @param originator The originator identifier (typically the AE-ID)
// @return *http.Response The HTTP response from the oneM2M platform
// @return error Returns nil on success, or an error if the request fails
func MakeOneM2MRequest(method, url, body string, ty int, originator string) (*http.Response, error) {
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
		Timeout: 10 * time.Second}
	reqID := uuid.New().String()

	// if originator == "" {
	// 	originator = AE_ID
	// }

	headers := map[string]string{
		"X-M2M-Origin": originator,
		"X-M2M-RI":     reqID,
		"X-M2M-RVI":    "5",
		"Accept":       "application/json",
	}

	if cseUser != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(cseUser+":"+csePassword))
	}

	if method == "POST" {
		headers["Content-Type"] = fmt.Sprintf("application/json;ty=%d", ty)
	} else if method == "PUT" {
		headers["Content-Type"] = "application/json"
	}

	// Transient failures are retried with jittered backoff (see retry.go); the
	// request is rebuilt on each attempt so the body can be resent
	resp, err := doWithRetry(client, func() (*http.Request, error) {
		req, err := http.NewRequest(method, url, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		return req, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	return resp, nil
}

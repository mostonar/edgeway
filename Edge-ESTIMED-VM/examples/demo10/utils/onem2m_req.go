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

// MakeOneM2MRequest creates and sends a oneM2M HTTP request
//
// This function creates and sends an HTTP request to a oneM2M CSE platform endpoint.
// It sets the required oneM2M headers including X-M2M-Origin, X-M2M-RI (request ID),
// X-M2M-RVI (release version indicator), and Content-Type with resource type (ty).
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
		"X-M2M-Origin":  originator,
		"X-M2M-RI":      reqID,
		"X-M2M-RVI":     "5",
		"Accept":        "application/json",
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("CAdmin:ikram123")),
	}

	if method == "POST" {
		headers["Content-Type"] = fmt.Sprintf("application/json;ty=%d", ty)
	}

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	return resp, nil
}

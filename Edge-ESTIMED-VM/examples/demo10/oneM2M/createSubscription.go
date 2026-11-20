package onem2m

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// CreateSubscription creates a Subscription resource on the specified container
//
// This function creates a Subscription resource that enables the oneM2M platform
// to send notifications to a specified endpoint when events occur on the container
// (e.g., when new ContentInstances are created).
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cseName The name of the Common Services Entity (CSE)
// @param appName The resource name for the application
// @param containerName The name of the container to subscribe to
// @return error Returns nil on success, or an error if subscription creation fails
func CreateSubscription(platformURL, cseName, appName, containerName string) error {
	subscriptionURL := fmt.Sprintf("%s/%s/%s/%s", platformURL, cseName, appName, containerName)

	target := subscriptionURL
	// Subscription resource body
	body := map[string]interface{}{
		"m2m:sub": map[string]interface{}{
			"rn":  "DataSyncSub",
			"nu":  []string{"https://192.168.20.163:9999/notifications"},
			"nct": 1,
			"enc": map[string]interface{}{
				"net": []int{3},
			},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription body: %v", err)
	}

	adminUser := "CAdmin"
	adminSecret := "ikram123"
	authString := fmt.Sprintf("%s:%s", adminUser, adminSecret)
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(authString))

	req, err := http.NewRequest("POST", target, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create subscription request: %v", err)
	}
	req.Header.Set("X-M2M-Origin", adminUser)
	req.Header.Set("X-M2M-RI", "req-admin")
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-M2M-RVI", "5")
	req.Header.Set("Content-Type", "application/json;ty=23")

	// Skip TLS verification for development/testing
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send subscription request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 201 && resp.StatusCode != 409 { // 409: Already exists
		return fmt.Errorf("subscription creation failed: %s", string(respBody))
	}
	return nil
}

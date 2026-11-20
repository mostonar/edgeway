package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

// SendAddressChangeNotification sends a webhook to the device application
func SendAddressChangeNotification(callbackRef string, contextId string, appInstanceId string, referenceURI string) {
	if callbackRef == "" {
		log.Warn("No callback reference provided, skipping notification for context: ", contextId)
		return
	}

	notification := AddressChangeNotification{
		NotificationType: "AddressChangeNotification",
		ContextId:        contextId,
		AppInstanceId:    appInstanceId,
		ReferenceURI:     referenceURI, // Note: using ReferenceURI based on generated model
	}

	body, err := json.Marshal(notification)
	if err != nil {
		log.Error("Failed to marshal AddressChangeNotification: ", err)
		return
	}

	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(callbackRef, "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Error("Failed to send AddressChangeNotification to ", callbackRef, ": ", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
			log.Info("Successfully sent AddressChangeNotification to ", callbackRef)
		} else {
			log.Warn("Received unexpected status code ", resp.StatusCode, " from ", callbackRef)
		}
	}()
}

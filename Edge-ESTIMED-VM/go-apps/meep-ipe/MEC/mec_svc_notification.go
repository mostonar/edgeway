package mec

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	onem2m "meep-ipe/oneM2M"
	"net/http"
	// "time"
)

type ServiceReference struct {
	Link struct {
		Href string `json:"href"`
	} `json:"link"`
	ServiceName   string `json:"serName"`
	ServiceInstID string `json:"serInstanceId"`
	State         string `json:"state"`
	ChangeType    string `json:"changeType"`
}

type SerAvailabilityNotification struct {
	NotificationType string             `json:"notificationType"`
	ServiceRefs      []ServiceReference `json:"serviceReferences"`
}

// ServiceAvailabilityNotificationHandler handles Notifications for service availability from the MEC platform.
func ServiceAvailabilityNotificationHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received Service Availability Notification")
	log.Printf("Request Method: %s, Request URL: %s", r.Method, r.URL.String())

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("Request Body: %s", string(body))

	var notification SerAvailabilityNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		log.Printf("Error unmarshalling request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, ref := range notification.ServiceRefs {
		log.Printf("Service Name: %s, State: %s, ChangeType: %s", ref.ServiceName, ref.State, ref.ChangeType)
		if ref.ChangeType == "ADDED" {
			onem2m.CreateContainer(ref.ServiceName, ref.Link.Href)
		} else if ref.ChangeType == "REMOVED" {
			onem2m.DeleteContainer(ref.ServiceName)
		} else {
			log.Printf("Unhandled ChangeType: %s", ref.ChangeType)
		}
	}

	w.WriteHeader(http.StatusNoContent)
	fmt.Fprintf(w, "Notification received")
}

// ServiceTerminationNotificationHandler handles Notifications for service termination from the MEC platform.
func ServiceTerminationNotificationHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received Service Termination Notification")
	// Here you would handle the notification, e.g., parse the request body, validate it, etc.
	// For now, we just log the request method and URL.
	log.Printf("Request Method: %s, Request URL: %s", r.Method, r.URL.String())

	// Display body content recieved in the notification
	body := make([]byte, r.ContentLength)
	_, err := r.Body.Read(body)
	if err != nil && err.Error() != "EOF" {
		log.Printf("Error reading request body: %v", err)
	} else {
		log.Printf("Request Body: %s", string(body))
	}

	// Respond with a 200 OK status
	w.WriteHeader(http.StatusNoContent)
	fmt.Fprintf(w, "Notification received")
}
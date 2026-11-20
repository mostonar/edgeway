package onem2m

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	utils "meep-ipe/utils"
)

type incomingMECDev struct {
	MECIOTDEV struct {
		CND   string `json:"cnd"`
		DEVID string `json:"devId"`
		ENABD bool   `json:"enabd"`
		RN    string `json:"rn"`
	} `json:"mec:iotDev"`
}

type outgoingMECDev struct {
	MECIOTDEV struct {
		CND   string `json:"cnd"`
		DEVID string `json:"devId"`
		ENABD bool   `json:"enabd"`
		RN    string `json:"rn"`
	} `json:"mec:iotDev"`
}

func CreateMECDev(w http.ResponseWriter, r *http.Request) {
	// Print header information for debugging
	log.Println("Received Device Notification POST request:", r.URL.String())
	log.Printf("Request Method: %s, Request URL: %s", r.Method, r.URL.String())
	log.Printf("Request Headers: %v", r.Header)
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the incoming request
	var incoming incomingMECDev
	if err := json.Unmarshal(body, &incoming); err != nil {
		log.Printf("Error unmarshalling request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if incoming.MECIOTDEV.DEVID == "" || incoming.MECIOTDEV.CND == "" {
		log.Printf("Error: incoming body missing devId/cnd: %s", string(body))
		http.Error(w, "Missing devId or cnd", http.StatusBadRequest)
		return
	}

	outgoing := outgoingMECDev{
		MECIOTDEV: struct {
			CND   string `json:"cnd"`
			DEVID string `json:"devId"`
			ENABD bool   `json:"enabd"`
			RN    string `json:"rn"`
		}{
			CND:   incoming.MECIOTDEV.CND,
			DEVID: incoming.MECIOTDEV.DEVID,
			ENABD: incoming.MECIOTDEV.ENABD,
			RN:    incoming.MECIOTDEV.RN,
		},
	}

	reqBodyBytes, err := json.Marshal(outgoing)
	if err != nil {
		log.Printf("Error marshalling outgoing MEC device body: %v", err)
		http.Error(w, "Failed to build outgoing MEC device body", http.StatusInternalServerError)
		return
	}
	reqBody := string(reqBodyBytes)

	url := fmt.Sprintf("%s:%s/~/%s/%s/%s", config["CSE-URL"], config["CSE-PORT"], config["CSE-ID"], config["CSE-NAME"], NodeName())
	log.Println("url (Create MEC Device):", url)
	log.Println("reqBody (Create MEC Device):", reqBody)

	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 28, "CAdmin")
	if err != nil {
		log.Printf("Error making OneM2M request: %v", err)
		http.Error(w, "Failed to make OneM2M request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		http.Error(w, "Failed to read response body", http.StatusInternalServerError)
		return
	}

	log.Printf("Response from OneM2M: %s", string(respBody))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

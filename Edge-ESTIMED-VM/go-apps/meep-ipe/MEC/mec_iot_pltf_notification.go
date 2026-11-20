package mec

import (
	"io"
	"log"
	"net/http"
	"strconv"

	oneM2M "meep-ipe/oneM2M"
	// utils "meep-ipe/utils"
)

func MecIotPltfNotificationHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received MEC IoT Platform Notification")
	log.Printf("Request Method: %s, Request URL: %s", r.Method, r.URL.String())

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Unable to read request body", http.StatusBadRequest)
		return
	}
	log.Printf("Request Body: %s", string(body))

	rsc, statusCode, respBody, err := oneM2M.CreateAE(body)
	if rsc != 0 {
		w.Header().Set("X-M2M-RSC", strconv.Itoa(rsc))
	}
	if err != nil {
		log.Printf("Error creating AE: %v", err)
		// Still relay whatever the CSE said, if we have anything
		if statusCode != 0 {
			w.WriteHeader(statusCode)
			w.Write(respBody)
			return
		}
		http.Error(w, "Failed to create AE", http.StatusInternalServerError)
		return
	}

	log.Printf("Response from oneM2M: RSC %d, body: %s", rsc, string(respBody))

	// Relay the CSE's RSC, status code and body back to the sandbox
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

// func MecIotPltfNotificationHandlerGET(w http.ResponseWriter, r *http.Request) {
// 	log.Println("Received discovery/GET request:", r.URL.String())

// 	fu := r.URL.Query().Get("fu")
// 	ty := r.URL.Query().Get("ty")
// 	log.Println("fu:", fu, "ty:", ty)

// 	// Query the actual known container path on the CSE — not derived from the incoming URL
// 	cseURL := config["CSE-URL"].(string) + ":" +
// 		config["CSE-PORT"].(string) + "/~/" +
// 		config["CSE-ID"].(string) + "/" +
// 		config["CSE-NAME"].(string) + "/" +
// 		config["mecplatform"].(string) + "/MecServices?fu=" + fu + "&ty=" + ty

// 	log.Println("Sending discovery GET to CSE:", cseURL)

// 	resp, err := utils.MakeOneM2MRequest("GET", cseURL, "", 0, "CAdmin")
// 	if err != nil {
// 		log.Println("Error sending discovery GET to CSE:", err)
// 		http.Error(w, "Failed to reach CSE", http.StatusBadGateway)
// 		return
// 	}
// 	defer resp.Body.Close()

// 	cseBody, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		log.Println("Error reading CSE response body:", err)
// 		http.Error(w, "Failed to read CSE response", http.StatusInternalServerError)
// 		return
// 	}
// 	log.Println("CSE discovery response: status=", resp.StatusCode, "body=", string(cseBody))

// 	w.Header().Set("Content-Type", "application/json")
// 	w.WriteHeader(resp.StatusCode)
// 	w.Write(cseBody)
// }

func MecIotPltfNotificationHandlerGET(w http.ResponseWriter, r *http.Request) {
	log.Println("Received discovery/GET request:", r.URL.String())

	fu := r.URL.Query().Get("fu")
	ty := r.URL.Query().Get("ty")
	log.Println("fu:", fu, "ty:", ty)

	// Query the actual known container path on the CSE — not derived from the incoming URL
	// cseBody := []byte(`[]`)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"m2m:uril":[]}`))
	// w.Write(cseBody)
}

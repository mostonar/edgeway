package onem2m

import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	"meep-ipe/utils"
)

// CreateAE creates an Application Entity (AE) on the oneM2M platform
//
// This function creates an AE resource on the oneM2M CSE. The AE represents
// the application in the oneM2M system and provides the entry point for
// application resources and data.
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cse_name The name of the Common Services Entity (CSE)
// @param cse_id The structured ID of the CSE
// @param App_Name The resource name for the application
// @param App_Id The application ID (API identifier)
// @param AE_ID The Application Entity ID for authentication
// @param remote_cse_id The remote CSE ID for access control
// @return error Returns nil on success, or an error if AE creation fails
// incomingAE models the relevant fields from the notification body:
// { "m2m:ae": { "rn": "...", "api": "..." } }
type incomingAE struct {
	M2MAE struct {
		RN  string   `json:"rn"`
		API string   `json:"api"`
		RR  bool     `json:"rr"`
		SRV []string `json:"srv"`
	} `json:"m2m:ae"`
}

// outgoingAE is the request body sent to the oneM2M CSE to create the AE.
type outgoingAE struct {
	M2MAE struct {
		RN  string   `json:"rn"`
		API string   `json:"api"`
		RR  bool     `json:"rr"`
		SRV []string `json:"srv"`
	} `json:"m2m:ae"`
}

// CreateAE creates an Application Entity (AE) on the oneM2M platform,
// using rn/api extracted from the incoming notification body.
// It returns the oneM2M Response Status Code (X-M2M-RSC), the CSE's HTTP status
// and the CSE response body. RSC 2001, 4105 and 4117 are treated as success.
func CreateAE(body []byte) (int, int, []byte, error) {
	log.Println("Creating Application Entity (AE)...")

	var incoming incomingAE
	if err := json.Unmarshal(body, &incoming); err != nil {
		return 0, 0, nil, fmt.Errorf("failed to parse incoming AE body: %w", err)
	}
	if incoming.M2MAE.RN == "" || incoming.M2MAE.API == "" {
		return 0, 0, nil, fmt.Errorf("incoming body missing rn/api: %s", string(body))
	}

	var outgoing outgoingAE
	outgoing.M2MAE.RN = incoming.M2MAE.RN
	outgoing.M2MAE.API = incoming.M2MAE.API
	outgoing.M2MAE.RR = incoming.M2MAE.RR
	outgoing.M2MAE.SRV = incoming.M2MAE.SRV

	config["mecAppName"] = outgoing.M2MAE.RN

	log.Printf("Config[MEC App name]: %+v", config["mecAppName"])
	reqBodyBytes, err := json.Marshal(outgoing)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to build outgoing AE body: %w", err)
	}
	reqBody := string(reqBodyBytes)

	// URL unchanged, exactly as you had it
	url := fmt.Sprintf("%s:%s/~/%s/%s", config["CSE-URL"], config["CSE-PORT"], config["CSE-ID"], config["CSE-NAME"])
	log.Println("url (Create AE):", url)
	log.Println("reqBody (Create AE):", reqBody)

	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 2, config["AE-ID"].(string))
	if err != nil {
		return 0, 0, nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	rsc, err := responseRSC(resp)
	if err != nil {
		return 0, resp.StatusCode, respBody, fmt.Errorf("failed to create AE: %v, body: %s", err, string(respBody))
	}
	log.Printf("Response from CSE: RSC %d, body: %s", rsc, string(respBody))

	if isCreateSuccess(rsc) {
		if rsc == RscCreated {
			log.Println("✓ Application Entity (AE) created successfully")
		} else {
			log.Printf("⚠ AE already exists (RSC %d)", rsc)
		}
		return rsc, resp.StatusCode, respBody, nil
	}

	return rsc, resp.StatusCode, respBody, fmt.Errorf("failed to create AE: RSC %d, body: %s", rsc, string(respBody))
}

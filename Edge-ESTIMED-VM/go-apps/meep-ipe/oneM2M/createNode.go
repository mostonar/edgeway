package onem2m

import (
	"fmt"
	"io"
	"log"

	"meep-ipe/utils"
)

func CreateNode(nodeName string, nodeID string) error {
	log.Println("Creating Node...")

	reqBody := fmt.Sprintf(`{
		"m2m:nod": {
			"rn": "%s",
			"ni": "%s"
		}
	}`, nodeName, nodeID)

	url := fmt.Sprintf("%s:%s/~/%s/%s", config["CSE-URL"], config["CSE-PORT"],
		config["CSE-ID"], config["CSE-NAME"])
	log.Println("url (Create Node):", url)
	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 14, "CAdmin")
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	rsc, err := responseRSC(resp)
	if err != nil {
		return fmt.Errorf("failed to create node: %v, body: %s", err, string(body))
	}

	if isCreateSuccess(rsc) {
		if rsc == RscCreated {
			log.Println("✓ Node created successfully")
		} else {
			log.Printf("⚠ Node already exists (RSC %d)", rsc)
		}
		return nil
	}

	return fmt.Errorf("failed to create node: RSC %d, body: %s", rsc, string(body))
}

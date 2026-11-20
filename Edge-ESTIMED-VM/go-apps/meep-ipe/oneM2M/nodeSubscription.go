package onem2m

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"meep-ipe/utils"
)

func CreateNodeSubscription(callbackURL string) error {
	subBody := map[string]interface{}{
		"m2m:sub": map[string]interface{}{
			"rn": NodeSubscriptionName(),
			"nu": []string{callbackURL},
			"enc": map[string]interface{}{
				"net":  []int{3},
				"chty": []int{14},
			},
			"nct": 1,
		},
	}

	reqBodyBytes, err := json.Marshal(subBody)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription body: %w", err)
	}
	reqBody := string(reqBodyBytes)

	url := fmt.Sprintf("%s:%s/~/%s/%s", config["CSE-URL"], config["CSE-PORT"], config["CSE-ID"], config["CSE-NAME"])
	log.Println("url (Create Node Subscription):", url)

	// ty=23 -> subscription resource type
	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 23, "CAdmin")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	rsc, err := responseRSC(resp)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %v, body: %s", err, string(body))
	}
	log.Println("Create Node Subscription response: RSC", rsc, string(body))

	if isCreateSuccess(rsc) {
		if rsc == RscCreated {
			log.Println("✓ Node creation subscription created successfully")
		} else {
			log.Printf("⚠ Node creation subscription already exists (RSC %d)", rsc)
		}
		return nil
	}
	return fmt.Errorf("failed to create subscription: RSC %d, body: %s", rsc, string(body))
}

func DeleteNodeSubscription() error {
	url := fmt.Sprintf("%s:%s/~/%s/%s/%s", config["CSE-URL"], config["CSE-PORT"], config["CSE-ID"], config["CSE-NAME"], NodeSubscriptionName())
	log.Println("url (Delete Node Subscription):", url)

	resp, err := utils.MakeOneM2MRequest("DELETE", url, "", 0, "CAdmin")
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Println("Delete Node Subscription response:", resp.StatusCode, string(body))

	if resp.StatusCode == http.StatusOK {
		log.Println("✓ Node creation subscription deleted successfully")
		return nil
	} else if resp.StatusCode == http.StatusNotFound {
		log.Println("⚠ Node creation subscription does not exist")
		return nil
	}
	return fmt.Errorf("failed to delete subscription: status %d, body: %s", resp.StatusCode, string(body))
}

package onem2m

import (
	"fmt"
	"log"

	"meep-ipe/utils"
)

var config = make(map[string]interface{})

func Init(configmap map[string]interface{}) {
	config = configmap

	user, _ := config["CSE-USER"].(string)
	password, _ := config["CSE-PASSWORD"].(string)
	utils.SetCSECredentials(user, password)

	log.Println("oneM2M configuration initialized:", config)
}

// NodeName returns the rn of this IPE's node on the CSE. It is the AE-ID, which
// entrypoint.sh builds as <MEEP_AE_ID>-<mecplatform>-<sandbox>, so IPE instances
// (e.g. in-sandbox vs. external) sharing the same CSE get distinct nodes.
func NodeName() string {
	return config["AE-ID"].(string)
}

// NodeSubscriptionName returns the rn of the node creation subscription, suffixed
// with the node name for the same reason as NodeName.
func NodeSubscriptionName() string {
	return fmt.Sprintf("nodeCreationSub-%s", NodeName())
}

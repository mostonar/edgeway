package mec

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	utils "meep-ipe/utils"
)

// --- Incoming oneM2M node-creation notification ---

type NodeNotification struct {
	SGN struct {
		NEV struct {
			REP struct {
				NOD struct {
					RN string `json:"rn"`
					NI string `json:"ni"`
				} `json:"m2m:nod"`
			} `json:"rep"`
		} `json:"nev"`
	} `json:"m2m:sgn"`
}

// --- MEC Location API response types ---

type TimeStamp struct {
	NanoSeconds int `json:"nanoSeconds"`
	Seconds     int `json:"seconds"`
}

type LocationInfo struct {
	Latitude  []float64 `json:"latitude"`
	Longitude []float64 `json:"longitude"`
	Shape     int       `json:"shape"`
}

type CivicInfo struct {
	Country string `json:"country"`
}

type User struct {
	Address       string       `json:"address"`
	AccessPointId string       `json:"accessPointId"`
	ZoneId        string       `json:"zoneId"`
	ResourceURL   string       `json:"resourceURL"`
	TimeStamp     TimeStamp    `json:"timeStamp"`
	LocationInfo  LocationInfo `json:"locationInfo"`
	CivicInfo     CivicInfo    `json:"civicInfo"`
	// RelativeLocationInfo intentionally omitted
}

type UserList struct {
	ResourceURL string `json:"resourceURL"`
	User        []User `json:"user"`
}

// UserListResponse mirrors the MEC Location API's response envelope.
type UserListResponse struct {
	UserList UserList `json:"userList"`
}

// --- oneM2M flexContainer bodies, using FCP short names (sname) as JSON keys ---

// Cnd (containerDefinition) is a provisioning-only attribute: the CSE
// accepts it on create but rejects it on update ("non-provision attribute"),
// so it's omitempty and left unset on update bodies.
type M2MUser struct {
	RN    string   `json:"rn,omitempty"`
	Lbl   []string `json:"lbl,omitempty"`
	Cnd   string   `json:"cnd,omitempty"`
	Addrs string   `json:"addrs"`
	AcPId string   `json:"acPId,omitempty"`
	ZonId string   `json:"zonId,omitempty"`
	ReURL string   `json:"reURL,omitempty"`
}

type M2MLocIo struct {
	RN    string    `json:"rn,omitempty"`
	Lbl   []string  `json:"lbl,omitempty"`
	Cnd   string    `json:"cnd,omitempty"`
	Latie []float64 `json:"latie"`
	Longe []float64 `json:"longe"`
	Shape int       `json:"shape"`
}

type M2MCivAs struct {
	RN    string   `json:"rn,omitempty"`
	Lbl   []string `json:"lbl,omitempty"`
	Cnd   string   `json:"cnd,omitempty"`
	Couny string   `json:"couny"`
}

// NodeCreationNotification handles incoming oneM2M notifications for node
// creation. It kicks off a periodic poller (see pollInterval) that looks up
// the corresponding UE's location info from the MEC Location API and syncs
// it to the CSE, then responds immediately so other notifications are never
// blocked behind the poll's network I/O.
func NodeCreationNotification(w http.ResponseWriter, r *http.Request) {
	log.Println("Received Node Creation Notification from oneM2M platform")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var notification NodeNotification
	err = json.Unmarshal(body, &notification)
	if err != nil {
		log.Printf("Error unmarshalling request body: %v", err)
		http.Error(w, "Error unmarshalling request body", http.StatusInternalServerError)
		return
	}

	nodeName := notification.SGN.NEV.REP.NOD.RN
	nodeID := notification.SGN.NEV.REP.NOD.NI

	log.Printf("Node Name: %s", nodeName)
	log.Printf("Node ID: %s", nodeID)

	// Some notifications carry no node info (e.g. a subscription
	// verification request, which has no m2m:nod payload) - there's nothing
	// to poll for in that case, so don't start a poller for an empty name.
	if nodeName == "" {
		log.Println("Node Creation Notification has no node name, skipping poller start")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Notification received")
		return
	}

	startNodeLocationPoller(nodeName)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Notification received, polling started")
}

// --- Periodic location polling ---

// pollInterval controls how often a node's location is re-synced once a
// poller is running for it.
const pollInterval = 15 * time.Second

var (
	nodePollersMu sync.Mutex
	nodePollers   = map[string]chan struct{}{}
)

// startNodeLocationPoller (re)starts the periodic location poller for
// nodeName. It is only ever called from NodeCreationNotification, i.e. a
// poller only starts once the oneM2M CSE actually notifies us of a new
// node. If a poller is already running for this node (a repeat
// notification), it is stopped first so only one poller per node is ever
// active.
func startNodeLocationPoller(nodeName string) {
	nodePollersMu.Lock()
	if stop, ok := nodePollers[nodeName]; ok {
		close(stop)
	}
	stop := make(chan struct{})
	nodePollers[nodeName] = stop
	nodePollersMu.Unlock()

	go pollNodeLocation(nodeName, stop)
}

// pollNodeLocation runs syncNodeLocation immediately, then every
// pollInterval, until stop is closed.
func pollNodeLocation(nodeName string, stop <-chan struct{}) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		syncNodeLocation(nodeName)

		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

// syncNodeLocation fetches the current location for nodeName and upserts it
// into the CSE (updating the existing flexContainer if present, creating it
// otherwise). Errors are logged and swallowed so a bad poll never stops the
// loop.
func syncNodeLocation(nodeName string) {
	user, err := fetchUserLocation(nodeName)
	if err != nil {
		log.Printf("poll[%s]: %v", nodeName, err)
		return
	}

	if err := upsertUserFlexContainer(nodeName, user); err != nil {
		log.Printf("poll[%s]: upsert failed: %v", nodeName, err)
	}
}

// fetchUserLocation queries the MEC Location API for nodeName and returns
// the first matching user entry.
func fetchUserLocation(nodeName string) (User, error) {
	url := fmt.Sprintf("%s/%s/%s/location/v3/queries/users?address=%s",
		config["sandboxUrl"], config["sandboxName"], config["mecplatform"], nodeName)
	resp, err := utils.SendMECRequest("GET", url, nil)
	if err != nil {
		return User{}, fmt.Errorf("error sending GET request to MEC Location API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("MEC Location API returned non-200 status code: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return User{}, fmt.Errorf("error reading response body from MEC Location API: %w", err)
	}

	log.Printf("Response from MEC Location API: %s", string(respBody))

	var userListResp UserListResponse
	if err := json.Unmarshal(respBody, &userListResp); err != nil {
		return User{}, fmt.Errorf("error unmarshalling MEC Location API response: %w", err)
	}

	if len(userListResp.UserList.User) == 0 {
		return User{}, fmt.Errorf("no user entries returned from MEC Location API for node %s", nodeName)
	}

	return userListResp.UserList.User[0], nil
}

// upsertUserFlexContainer syncs the mec:user flexContainer (and its
// mec:locIo/mec:civAs children) for user under parentRn: each resource is
// updated in place if it already exists on the CSE, or created if it
// doesn't.
func upsertUserFlexContainer(parentRn string, user User) error {
	userPath := parentRn + "/" + user.Address

	userCreateBody := map[string]interface{}{
		"mec:user": M2MUser{
			RN:    user.Address,
			Cnd:   "org.etsi.mec.moduleclass.user",
			Lbl:   []string{"MEC"},
			Addrs: user.Address,
			AcPId: user.AccessPointId,
			ZonId: user.ZoneId,
			ReURL: user.ResourceURL,
		},
	}
	userUpdateBody := map[string]interface{}{
		"mec:user": M2MUser{
			Lbl:   []string{"MEC"},
			Addrs: user.Address,
			AcPId: user.AccessPointId,
			ZonId: user.ZoneId,
			ReURL: user.ResourceURL,
		},
	}
	if err := upsertFlexContainer(userPath, parentRn, userCreateBody, userUpdateBody); err != nil {
		return fmt.Errorf("mec:user: %w", err)
	}

	locIoPath := userPath + "/locationInfo"
	locIoCreateBody := map[string]interface{}{
		"mec:locIo": M2MLocIo{
			RN:    "locationInfo",
			Cnd:   "org.etsi.mec.moduleclass.locationInfo",
			Lbl:   []string{"MEC"},
			Latie: user.LocationInfo.Latitude,
			Longe: user.LocationInfo.Longitude,
			Shape: user.LocationInfo.Shape,
		},
	}
	locIoUpdateBody := map[string]interface{}{
		"mec:locIo": M2MLocIo{
			Lbl:   []string{"MEC"},
			Latie: user.LocationInfo.Latitude,
			Longe: user.LocationInfo.Longitude,
			Shape: user.LocationInfo.Shape,
		},
	}
	if err := upsertFlexContainer(locIoPath, userPath, locIoCreateBody, locIoUpdateBody); err != nil {
		return fmt.Errorf("mec:locIo: %w", err)
	}

	civAsPath := userPath + "/civicInfo"
	civAsCreateBody := map[string]interface{}{
		"mec:civAs": M2MCivAs{
			RN:    "civicInfo",
			Cnd:   "org.etsi.mec.moduleclass.civicAddress",
			Lbl:   []string{"MEC"},
			Couny: user.CivicInfo.Country,
		},
	}
	civAsUpdateBody := map[string]interface{}{
		"mec:civAs": M2MCivAs{
			Lbl:   []string{"MEC"},
			Couny: user.CivicInfo.Country,
		},
	}
	if err := upsertFlexContainer(civAsPath, userPath, civAsCreateBody, civAsUpdateBody); err != nil {
		return fmt.Errorf("mec:civAs: %w", err)
	}

	return nil
}

// upsertFlexContainer tries to UPDATE the flexContainer at resourcePath
// (its own resource path, e.g. "node/addr/locationInfo") with updateBody.
// If the resource doesn't exist yet (404), it falls back to CREATEing it
// under parentPath with createBody (which includes the resource's rn).
func upsertFlexContainer(resourcePath, parentPath string, createBody, updateBody interface{}) error {
	updateReqBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	url := fmt.Sprintf("%s:%s/~/%s/%s/%s", config["CSE-URL"], config["CSE-PORT"], config["CSE-ID"], config["CSE-NAME"], resourcePath)
	resp, err := utils.MakeOneM2MRequest("PUT", url, string(updateReqBytes), 0, "CAdmin")
	if err != nil {
		return err
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Println("✓ Flex container updated successfully:", resourcePath)
		return nil
	}

	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to update flex container: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Not found -> doesn't exist yet, create it.
	return postFlexContainer(parentPath, createBody)
}

// postFlexContainer POSTs a flexContainer creation request under the given
// parent resource path (relative to the CSE root).
func postFlexContainer(parentRn string, body interface{}) error {
	reqBodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}
	reqBody := string(reqBodyBytes)

	url := fmt.Sprintf("%s:%s/~/%s/%s/%s", config["CSE-URL"], config["CSE-PORT"], config["CSE-ID"], config["CSE-NAME"], parentRn)
	log.Println("url (Create Flex Container):", url)
	log.Println("reqBody (Create Flex Container):", reqBody)

	// ty=28 -> flexContainer resource type
	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 28, "CAdmin")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Println("Create Flex Container response:", resp.StatusCode, string(respBody))

	if resp.StatusCode == http.StatusCreated {
		log.Println("✓ Flex container created successfully")
		return nil
	}
	return fmt.Errorf("failed to create flex container: status %d, body: %s", resp.StatusCode, string(respBody))
}
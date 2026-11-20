package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	//"syscall"

	gisClient "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-engine-client"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

func main() {
	log.MeepTextLogInit("system_test_http_stub")

	// Set routing rules
	http.HandleFunc("/sandbox-ctrl/v1/events/MOBILITY", sandbox_ctrl_mobility)
	http.HandleFunc("/gis/v1/geodata/cellularPower", gis_geodata_cellular_power)
	http.HandleFunc("/callback/vis/v2/ProvChgUuUniSubscription", dummy_vis_prov_chg_uu_uni_subscription_callback)
	http.HandleFunc("/callback/vis/v2/ProvChgUuMbmsSubscription", dummy_vis_callback)
	http.HandleFunc("/callback/vis/v2/ProvChgPc5Subscription", dummy_vis_callback)
	http.HandleFunc("/callback/vis/v2/V2xMsgSubscription", dummy_vis_callback)
	http.HandleFunc("/callback/vis/v2/PredQosSubscription", dummy_vis_callback)

	log.Info("Create server")
	srv := &http.Server{Addr: ":8080"}
	go func() {
		//returns ErrServerClosed on graceful close
		log.Info("Call ListenAndServe")
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			//NOTE: there is a chance that next line won't have time to run,
			//as main() doesn't wait for this goroutine to stop. don't use
			//code with race conditions like these for production. see post
			//comments below on more discussion on how to handle this.
			log.Info("ListenAndServe(): %s", err)
			return
		}
		log.Info("Terminate goroutine")
	}()

	// //Use the default DefaultServeMux.
	// err := http.ListenAndServe(":8080", nil)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	log.Info("Configure signals")
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	sig := <-c
	log.Info("Got %s signal. Aborting...\n", sig)
	err := srv.Shutdown(context.TODO())
	if err != nil {
		log.Info("Shutdown(): %s", err)
		return
	}
}

/*func convertPredictedQostoJson(predictedQos *gisClient.PredictedQos) string {
	jsonInfo, err := json.Marshal(*predictedQos)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}*/

func sandbox_ctrl_mobility(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, "")

	log.Debug("<<< sandbox_ctrl_mobility: ", w)
}

func gis_geodata_cellular_power(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var requestData gisClient.GeoCoordinateList
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&requestData)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Debug("gis_geodata_cellular_power: ", requestData)
	// TODO Use Tables instead of hardcoded values {43.733505 7.413917} {43.733517 7.413916}
	io.WriteString(w, "{\"coordinatesPower\": [{\"latitude\": 43.733505,\"longitude\": 7.413917,\"rsrq\": 63,\"rsrp\": 21,\"poaName\": \"4g-macro-cell-5\"},{\"latitude\": 43.733517,\"longitude\": 7.413916,\"rsrq\": 60,\"rsrp\": 58,\"poaName\": \"4g-macro-cell-5\"}]}")
	log.Debug("<<< gis_geodata_cellular_power: ", w)
}

func dummy_vis_callback(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> dummy_callback: ", r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
	io.WriteString(w, "")

	log.Debug("<<< dummy_callback: ", w)
}

func dummy_vis_prov_chg_uu_uni_subscription_callback(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> dummy_vis_prov_chg_uu_uni_subscription_callback: ", r)

	w.Header().Set("Content-Type", "application/json")

	// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	body := string(bodyBytes)
	log.Info("dummy_vis_prov_chg_uu_uni_subscription_callback: body: ", body)
	if strings.Contains(body, "TestNotification") || strings.Contains(body, "ProvChgUuUniSubscription") {
		log.Info("dummy_vis_prov_chg_uu_uni_subscription_callback: Reply with 204")
		w.WriteHeader(http.StatusNoContent)
	} else {
		log.Info("dummy_vis_prov_chg_uu_uni_subscription_callback: Reply with 400")
		w.WriteHeader(http.StatusBadRequest)
	}
	io.WriteString(w, "")

	log.Debug("<<< dummy_vis_prov_chg_uu_uni_subscription_callback: ", w)
}

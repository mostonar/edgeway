package server

import (
	"net/http"
	mec "meep-ipe/MEC"
	onem2m "meep-ipe/oneM2M"
)

func MecIotPltfNotificationHandler(w http.ResponseWriter, r *http.Request) {
	mec.MecIotPltfNotificationHandler(w, r)
}

func ServiceAvailabilityNotificationHandler(w http.ResponseWriter, r *http.Request) {
	mec.ServiceAvailabilityNotificationHandler(w, r)
}

func ServiceTerminationNotificationHandler(w http.ResponseWriter, r *http.Request) {
	mec.ServiceTerminationNotificationHandler(w, r)
}

func MecIotPltfNotificationHandlerGET(w http.ResponseWriter, r *http.Request) {
	mec.MecIotPltfNotificationHandlerGET(w, r)
}

func SensorRegisterPostHandler(w http.ResponseWriter, r *http.Request) {
	mec.SensorRegisterPostHandler(w, r)
}

func DeviceNotificationHandlerAe(w http.ResponseWriter, r *http.Request) {
	onem2m.CreateMECDev(w, r)
}

func NodeCreationNotificationHandler(w http.ResponseWriter, r *http.Request) {
	mec.NodeCreationNotification(w, r)
}

package server

import (
	"encoding/json"
	"os"
	"strings"

	gc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-cache"
	mod "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model"
)

func isExternalNwSimulatorType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "simu5g", "oai":
		return true
	default:
		return false
	}
}

func userMetaExternalNwSimulator(userMeta map[string]string) bool {
	if userMeta == nil {
		return false
	}
	if raw, ok := userMeta["network-info"]; ok && raw != "" {
		var info struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(raw), &info) == nil && isExternalNwSimulatorType(info.Type) {
			return true
		}
	}
	if raw, ok := userMeta["mec-sandbox"]; ok && raw != "" {
		var ms struct {
			ConnectivityModel string `json:"connectivityModel"`
		}
		if json.Unmarshal([]byte(raw), &ms) == nil && isExternalNwSimulatorType(ms.ConnectivityModel) {
			return true
		}
	}
	return false
}

// usesExternalNwSimulator is true when the platform catalog is Simu5G/OAI
// or the active scenario userMeta says the Network Emulation Stack owns radio/net-char.
func usesExternalNwSimulator() bool {
	if isExternalNwSimulatorType(os.Getenv("MEEP_NW_SIMULATOR_TYPE")) {
		return true
	}
	if ge == nil || ge.activeModel == nil {
		return false
	}
	dep := ge.activeModel.GetDeployment(&mod.NodeFilter{ExcludeChildren: true})
	if dep == nil {
		return false
	}
	return userMetaExternalNwSimulator(dep.UserMeta)
}

// mergePoaMeasurementForCache keeps GIS-cache radio fields owned by the
// Network Emulation Stack (Simu5G/OAI). Distance still comes from PostGIS.
func mergePoaMeasurementForCache(external bool, distance, rssi, rsrp, rsrq float32, cached *gc.PoaMeasurement) *gc.PoaMeasurement {
	m := &gc.PoaMeasurement{
		Distance: distance,
		Rssi:     rssi,
		Rsrp:     rsrp,
		Rsrq:     rsrq,
	}
	if !external {
		return m
	}
	if cached != nil {
		m.Rssi = cached.Rssi
		m.Rsrp = cached.Rsrp
		m.Rsrq = cached.Rsrq
	} else {
		m.Rssi = 0
		m.Rsrp = 0
		m.Rsrq = 0
	}
	return m
}

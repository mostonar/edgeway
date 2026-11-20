package server

import (
	"os"
	"testing"

	gc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-cache"
)

func TestIsExternalNwSimulatorType(t *testing.T) {
	if !isExternalNwSimulatorType("simu5g") || !isExternalNwSimulatorType("OAI") {
		t.Fatal("simu5g/oai must be external")
	}
	if isExternalNwSimulatorType("internal") || isExternalNwSimulatorType("") {
		t.Fatal("internal must not be external")
	}
}

func TestUserMetaExternalNwSimulator(t *testing.T) {
	if !userMetaExternalNwSimulator(map[string]string{
		"network-info": `{"type": "simu5g", "path":""}`,
	}) {
		t.Fatal("network-info type simu5g")
	}
	if !userMetaExternalNwSimulator(map[string]string{
		"mec-sandbox": `{"connectivityModel": "oai"}`,
	}) {
		t.Fatal("mec-sandbox connectivityModel oai")
	}
	if userMetaExternalNwSimulator(map[string]string{
		"network-info": `{"type": "internal"}`,
	}) {
		t.Fatal("internal network-info")
	}
	if userMetaExternalNwSimulator(nil) {
		t.Fatal("nil userMeta")
	}
}

func TestMergePoaMeasurementForCache(t *testing.T) {
	cached := &gc.PoaMeasurement{Rssi: 11, Rsrp: -80, Rsrq: 12, Distance: 10}
	got := mergePoaMeasurementForCache(true, 50, 1, 2, 3, cached)
	if got.Distance != 50 || got.Rssi != 11 || got.Rsrp != -80 || got.Rsrq != 12 {
		t.Fatalf("external must keep cached radio and take new distance: %+v", got)
	}
	got = mergePoaMeasurementForCache(true, 50, 1, 2, 3, nil)
	if got.Distance != 50 || got.Rssi != 0 || got.Rsrp != 0 || got.Rsrq != 0 {
		t.Fatalf("external with empty cache must not store GIS-computed radio: %+v", got)
	}
	got = mergePoaMeasurementForCache(false, 50, 1, 2, 3, cached)
	if got.Distance != 50 || got.Rssi != 1 || got.Rsrp != 2 || got.Rsrq != 3 {
		t.Fatalf("internal must use GIS-computed radio: %+v", got)
	}
}

func TestUsesExternalNwSimulatorEnv(t *testing.T) {
	prev := os.Getenv("MEEP_NW_SIMULATOR_TYPE")
	defer os.Setenv("MEEP_NW_SIMULATOR_TYPE", prev)
	saved := ge
	ge = nil
	defer func() { ge = saved }()

	os.Setenv("MEEP_NW_SIMULATOR_TYPE", "simu5g")
	if !usesExternalNwSimulator() {
		t.Fatal("simu5g env must be external")
	}
	os.Setenv("MEEP_NW_SIMULATOR_TYPE", "internal")
	if usesExternalNwSimulator() {
		t.Fatal("internal env must not be external")
	}
}

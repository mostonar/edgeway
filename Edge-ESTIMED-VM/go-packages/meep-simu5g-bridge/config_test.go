package bridge

import (
	"path/filepath"
	"runtime"
	"testing"
)

func repoToolsSimu5g(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "tools", "simu5g", name)
}

/** @brief Example integration config listens on UDP 9100 (not 9110). */
func TestExampleIntegrationConfigTelemetryPort(t *testing.T) {
	cfg, err := LoadIntegrationConfig(repoToolsSimu5g("example-integration-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateForMobilityAdapter(); err != nil {
		t.Fatal(err)
	}
	if cfg.Telemetry.ListenPort != 9100 {
		t.Fatalf("listenPort %d want 9100", cfg.Telemetry.ListenPort)
	}
}

/** @brief monaco-harbor Phase 0 config: port 9100, scene origin, ue-car-0 mapping. */
func TestMonacoHarborIntegrationConfig(t *testing.T) {
	cfg, err := LoadIntegrationConfig(repoToolsSimu5g("monaco-harbor-integration-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateForMobilityAdapter(); err != nil {
		t.Fatal(err)
	}
	if cfg.Deployment.ScenarioID != "monaco-harbor" {
		t.Fatalf("scenarioId %q", cfg.Deployment.ScenarioID)
	}
	if cfg.Telemetry.ListenPort != 9100 {
		t.Fatalf("listenPort %d want 9100", cfg.Telemetry.ListenPort)
	}
	if cfg.TelemetryListenAddr() != "0.0.0.0:9100" {
		t.Fatalf("listen %s", cfg.TelemetryListenAddr())
	}
	if cfg.Telemetry.SceneLatDeg != 43.734042 || cfg.Telemetry.SceneLonDeg != 7.416587 {
		t.Fatalf("scene origin %v,%v", cfg.Telemetry.SceneLatDeg, cfg.Telemetry.SceneLonDeg)
	}
	if len(cfg.UEMappings) != 8 {
		t.Fatalf("ueMappings %d want 8", len(cfg.UEMappings))
	}
	if cfg.UEMappings[0].UeID != "ue-car-0" {
		t.Fatalf("first ueId %q", cfg.UEMappings[0].UeID)
	}
	if cfg.UEMappings[0].Simu5gModulePath != "MonacoHarbor.ue_car[0]" {
		t.Fatalf("module path %q", cfg.UEMappings[0].Simu5gModulePath)
	}
	id, ok := cfg.ResolveSandboxUeID("MonacoHarbor.ue_car[0]", "")
	if !ok || id != "ue-car-0" {
		t.Fatalf("module map %q %v", id, ok)
	}
	id, ok = cfg.ResolveSandboxUeID("unknown", "10.0.0.14")
	if !ok || id != "ue-ped-3" {
		t.Fatalf("ipv4 map %q %v", id, ok)
	}
	if _, ok := cfg.ResolveSandboxUeID("MonacoHarbor.ue_car[99]", "10.9.9.9"); ok {
		t.Fatal("unmapped UE must not resolve")
	}
	foundEnb := false
	for _, c := range cfg.Cells {
		if c.PoaID == "poa-enb" && c.CellID == 100 {
			foundEnb = true
		}
	}
	if !foundEnb {
		t.Fatal("cells missing poa-enb cellId 100")
	}
}

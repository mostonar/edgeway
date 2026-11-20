/*
 * Copyright (c) 2026  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * @file config.go
 * @brief JSON integration configuration for two-host Simu5G deployment.
 *
 * @details
 * Host B (sandbox) uses connectAddress/connectPort and TUN name; host S (simulator)
 * uses listenAddress/listenPort. Shared ueMappings, mecEndpoints, and cells
 * tie sandbox pod CIDRs to Simu5G module paths.
 *
 * @ingroup simu5g_bridge_pkg
 */
package bridge

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

/**
 * @brief Root configuration object for bridge, mobility, and RNIS adapters.
 */
type IntegrationConfig struct {
	Deployment   DeploymentOptions `json:"deployment"`
	Bridge       BridgeOptions     `json:"bridge"`
	Telemetry    TelemetryOptions  `json:"telemetry"`
	RNI          RNIOptions        `json:"rni"`
	GIS          GISOptions        `json:"gis,omitempty"`
	UEMappings   []UEMapping       `json:"ueMappings"`
	MECEndpoints []MECEndpoint     `json:"mecEndpoints"`
	Cells        []CellMapping     `json:"cells"`
	Options      RuntimeOptions    `json:"options"`
}

/**
 * @brief Alias for IntegrationConfig (early bridge-only configs).
 */
type BridgeConfig = IntegrationConfig

/**
 * @brief Identifies scenario and physical hosts B (sandbox) and S (simulator).
 */
type DeploymentOptions struct {
	ScenarioID  string `json:"scenarioId"`
	SandboxHost string `json:"sandboxHost,omitempty"`
	SimHost     string `json:"simHost"`
}

/**
 * @brief TCP bridge and TUN settings.
 *
 * @details listen* is used on simulator host S; connect* on sandbox host B.
 */
type BridgeOptions struct {
	ListenAddress  string `json:"listenAddress,omitempty"`
	ListenPort     int    `json:"listenPort,omitempty"`
	ConnectAddress string `json:"connectAddress,omitempty"`
	ConnectPort    int    `json:"connectPort,omitempty"`
	UnixSocket     string `json:"unixSocket,omitempty"`
	TunName        string `json:"tunName,omitempty"`
}

/**
 * @brief UDP telemetry ingestion on sandbox host B (S sends datagrams to B).
 */
type TelemetryOptions struct {
	ListenAddress string  `json:"listenAddress,omitempty"`
	ListenPort    int     `json:"listenPort,omitempty"`
	SceneLatDeg   float64 `json:"sceneLatDeg,omitempty"` // OSM maxlat; must match S *.coordinateSystem.sceneLatitude
	SceneLonDeg   float64 `json:"sceneLonDeg,omitempty"` // OSM minlon; must match S *.coordinateSystem.sceneLongitude
	SceneAltM     float64 `json:"sceneAltM,omitempty"`
	MetersPerDeg  float64 `json:"metersPerDeg,omitempty"`
	JSONOut       bool    `json:"jsonOut,omitempty"`
}

/**
 * @brief RNI UDP polling of RniUdpQueryResponder on simulator host S from B.
 */
type RNIOptions struct {
	SimPort        int    `json:"simPort,omitempty"`
	PollIntervalMs int    `json:"pollIntervalMs,omitempty"`
	MeAppInst      string `json:"meAppInst,omitempty"`
}

/**
 * @brief GIS engine target on sandbox host B (ClusterIP, not ingress).
 *
 * @details Ingress GIS is session-authenticated. The adapter runs on the
 * node and should POST to the meep-gis-engine ClusterIP path /gis/v1.
 */
type GISOptions struct {
	BaseURL     string `json:"baseUrl,omitempty"`
	SandboxName string `json:"sandboxName,omitempty"`
}

/**
 * @brief Validates fields required on sandbox host B for meep-simu5g-bridge.
 * @param c Configuration pointer.
 * @return nil if valid, else descriptive error.
 */
func (c *IntegrationConfig) ValidateForBridge() error {
	if c == nil {
		return errors.New("nil config")
	}
	if c.Deployment.SimHost == "" {
		return errors.New("deployment.simHost is required (simulator host S)")
	}
	if c.BridgeTCPAddr() == "" {
		return errors.New("bridge connect target is empty")
	}
	return nil
}

/**
 * @brief Validates fields for meep-simu5g-mobility-adapter on B.
 * @param c Configuration pointer; default telemetry port 9100 if unset.
 * @return nil if valid.
 */
func (c *IntegrationConfig) ValidateForMobilityAdapter() error {
	if c == nil {
		return errors.New("nil config")
	}
	if c.Telemetry.ListenPort == 0 {
		c.Telemetry.ListenPort = 9100
	}
	return nil
}

/**
 * @brief Validates fields for meep-simu5g-rnis-adapter on B.
 * @param c Configuration pointer; applies RNI defaults (port 9201, poll 20 ms).
 * @return nil if valid.
 */
func (c *IntegrationConfig) ValidateForRNISAdapter() error {
	if c == nil {
		return errors.New("nil config")
	}
	if c.Deployment.SimHost == "" {
		return errors.New("deployment.simHost is required")
	}
	if c.RNI.SimPort == 0 {
		c.RNI.SimPort = 9201
	}
	if c.RNI.PollIntervalMs == 0 {
		c.RNI.PollIntervalMs = 20
	}
	return nil
}

/**
 * @brief Returns host:port for the bridge client on B to dial on S.
 * @return TCP address string (default port 5555).
 */
func (c *IntegrationConfig) BridgeTCPAddr() string {
	host := c.Bridge.ConnectAddress
	if host == "" {
		host = c.Deployment.SimHost
	}
	port := c.Bridge.ConnectPort
	if port == 0 {
		port = c.Bridge.ListenPort
	}
	if port == 0 {
		port = 5555
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

/**
 * @brief Returns UDP bind address for telemetry on sandbox host B.
 * @return listen host:port (default 0.0.0.0:9100).
 */
func (c *IntegrationConfig) TelemetryListenAddr() string {
	host := c.Telemetry.ListenAddress
	if host == "" {
		host = "0.0.0.0"
	}
	port := c.Telemetry.ListenPort
	if port == 0 {
		port = 9100
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

/**
 * @brief Returns simulator host S address for RNI UDP queries from B.
 * @return UDP address (default port 9201).
 */
func (c *IntegrationConfig) RNISimUDPAddr() string {
	port := c.RNI.SimPort
	if port == 0 {
		port = 9201
	}
	return net.JoinHostPort(c.Deployment.SimHost, strconv.Itoa(port))
}

/**
 * @brief Returns TUN device name on B.
 * @return Configured tunName or default simu5g-br0.
 */
func (c *IntegrationConfig) TunInterfaceName() string {
	if c.Bridge.TunName != "" {
		return c.Bridge.TunName
	}
	return "simu5g-br0"
}

/**
 * @brief Maps a telemetry sim module id or IPv4 to the sandbox GIS asset name.
 * @param simModuleID UeTelemetryUdpSender id (e.g. MonacoHarbor.ue_car[0]).
 * @param ipv4 Optional UE IPv4 from the same record.
 * @return GIS asset name (ue-car-0) and true when a mapping exists.
 */
func (c *IntegrationConfig) ResolveSandboxUeID(simModuleID, ipv4 string) (string, bool) {
	if c == nil {
		return "", false
	}
	for i := range c.UEMappings {
		m := &c.UEMappings[i]
		if m.Simu5gModulePath != "" && m.Simu5gModulePath == simModuleID && m.UeID != "" {
			return m.UeID, true
		}
	}
	if ipv4 != "" {
		idx := c.ResolveUeIndexFromIP(net.ParseIP(ipv4))
		if idx >= 0 && c.UEMappings[idx].UeID != "" {
			return c.UEMappings[idx].UeID, true
		}
	}
	return "", false
}

/**
 * @brief Explicit GIS HTTP base URL from config (no discovery).
 * @return Trimmed base URL or empty.
 */
func (c *IntegrationConfig) GISBaseURL() string {
	if c == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(c.GIS.BaseURL), "/")
}

/**
 * @brief Maps a sandbox pod source IP to ueMappings index.
 * @param ip IPv4 address from uplink packet.
 * @return Index in UEMappings, or -1 if no CIDR match.
 */
func (c *IntegrationConfig) ResolveUeIndexFromIP(ip net.IP) int {
	ip = ip.To4()
	if ip == nil {
		return -1
	}
	for i := range c.UEMappings {
		_, n, err := net.ParseCIDR(c.UEMappings[i].SandboxIPCidr)
		if err != nil {
			continue
		}
		if n.Contains(ip) {
			return i
		}
	}
	return -1
}

/**
 * @brief Maps destination IP to MEC endpoint index or UE index.
 * @param ip IPv4 destination address.
 * @return destID and isMEC true when IP matches MECEndpoints CIDR.
 */
func (c *IntegrationConfig) ResolveDestFromIP(ip net.IP) (destID int, isMEC bool) {
	ip = ip.To4()
	if ip == nil {
		return -1, false
	}
	for i := range c.MECEndpoints {
		_, n, err := net.ParseCIDR(c.MECEndpoints[i].SandboxIPCidr)
		if err != nil {
			continue
		}
		if n.Contains(ip) {
			return i, true
		}
	}
	idx := c.ResolveUeIndexFromIP(ip)
	return idx, false
}

/**
 * @brief Returns a minimal template configuration for documentation and tests.
 */
func DefaultIntegrationConfig() *IntegrationConfig {
	return &IntegrationConfig{
		Deployment: DeploymentOptions{
			ScenarioID: "example",
			SimHost:    "172.29.10.20",
		},
		Bridge: BridgeOptions{
			ConnectPort: 5555,
			TunName:     "simu5g-br0",
		},
		Telemetry: TelemetryOptions{
			ListenAddress: "0.0.0.0",
			ListenPort:    9100,
			SceneLatDeg:   43.734042,
			SceneLonDeg:   7.416587,
			MetersPerDeg:  111320,
		},
		RNI: RNIOptions{
			SimPort:        9201,
			PollIntervalMs: 20,
		},
		Options: RuntimeOptions{
			MTU:                 1400,
			MaxPendingPackets:   10000,
			RealTimeToleranceMs: 50,
		},
	}
}

/**
 * @brief One-line deployment summary for operator logs.
 * @return Human-readable scenario, hosts, and service endpoints.
 */
func (c *IntegrationConfig) FormatTwoHostSummary() string {
	return fmt.Sprintf("scenario=%s simHost=%s bridgeDial=%s telemetryListen=%s rniPoll=%s",
		c.Deployment.ScenarioID, c.Deployment.SimHost, c.BridgeTCPAddr(),
		c.TelemetryListenAddr(), c.RNISimUDPAddr())
}

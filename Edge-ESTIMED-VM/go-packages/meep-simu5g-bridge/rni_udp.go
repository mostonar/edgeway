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
 * @file rni_udp.go
 * @brief RNI UDP query/response client (sandbox host B → simulator host S).
 *
 * @details
 * Implements the binary protocol spoken with monaco-harbor RniUdpQueryResponder
 * on UDP :9201. Query types: RAB, PLMN, S1 bearer, Layer2, network metrics.
 * Used by meep-simu5g-rnis-adapter and CLI tooling.
 *
 * @ingroup simu5g_bridge_pkg
 */
package bridge

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

/** @defgroup rni_udp_wire RNI UDP wire constants (little-endian). */
/** @{ */
const (
	magic       uint32 = 0x524e4931 // 'RNI1'
	protoVer    uint16 = 1
	msgRequest  uint16 = 1
	msgResponse uint16 = 2

	QueryRab            uint32 = 1 // Radio access bearer info
	QueryPlmn           uint32 = 2 // PLMN / cell list
	QueryS1Bearer       uint32 = 3 // S1-U bearer TEIDs
	QueryLayer2         uint32 = 4 // Layer-2 measurements (RSRP/RSRQ/PRB)
	QueryNetworkMetrics uint32 = 5 // Throughput, latency, loss, jitter

	reqSize          = 92
	hdrSize          = 28
	maxUE            = 16
	maxCells         = 8
	netMetricsUeSize = 72
)

/** @} */

/** @brief Monotonic request ID for outbound RNI queries. */
var reqID uint64 = 1

/**
 * @brief Serializes an RNI UDP request datagram.
 * @param queryType One of QueryRab … QueryNetworkMetrics.
 * @param requestID Correlation ID echoed in response.
 * @param appInst ME application instance name (max 63 bytes).
 * @return 92-byte request buffer.
 */
func packRequest(queryType uint32, requestID uint64, appInst string) []byte {
	b := make([]byte, reqSize)
	binary.LittleEndian.PutUint32(b[0:4], magic)
	binary.LittleEndian.PutUint16(b[4:6], protoVer)
	binary.LittleEndian.PutUint16(b[6:8], msgRequest)
	binary.LittleEndian.PutUint32(b[8:12], queryType)
	binary.LittleEndian.PutUint64(b[12:20], requestID)
	if len(appInst) > 63 {
		appInst = appInst[:63]
	}
	binary.LittleEndian.PutUint16(b[20:22], uint16(len(appInst)))
	copy(b[28:28+len(appInst)], appInst)
	return b
}

/** @brief Formats little-endian uint32 as dotted IPv4 string. */
func ipv4StringLE(v uint32) string {
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, v)
	return ip.String()
}

/**
 * @brief Sends one or more RNI queries and prints JSON results to stdout.
 * @param addr Simulator UDP host:port (e.g. 172.29.10.20:9201).
 * @param queries List of query type constants.
 * @param meAppInst ME application instance string in request.
 * @param timeout Reserved (read deadline fixed at 3s per query).
 * @return Last response map and error (fatal on wire/decode failure via log.Fatal).
 */
func QueryRNIUDP(addr string, queries []uint32, meAppInst string, timeout time.Duration) (out map[string]interface{}, err error) {

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	for _, qt := range queries {
		raw, err := queryOne(conn, raddr, qt, meAppInst, reqID)
		if err != nil {
			log.Fatalf("%s: %v", queryName(qt), err)
		}
		qt2, rid2, st, payload, err := parseResponse(raw)
		if err != nil {
			log.Fatalf("%s decode: %v", queryName(qt), err)
		}
		out := map[string]interface{}{
			"query":      queryName(qt),
			"queryType":  qt2,
			"requestId":  rid2,
			"status":     st,
			"statusText": map[uint32]string{0: "OK", 1: "UNKNOWN_QUERY", 2: "BAD_REQUEST", 3: "INTERNAL"}[st],
			"payloadLen": len(payload),
		}
		if st == 0 && len(payload) > 0 {
			switch qt2 {
			case QueryRab:
				out["payloadJson"] = decodeRab(payload)
			case QueryPlmn:
				out["payloadJson"] = decodePlmn(payload)
			case QueryS1Bearer:
				out["payloadJson"] = decodeS1(payload)
			case QueryLayer2:
				out["payloadJson"] = decodeL2(payload)
			case QueryNetworkMetrics:
				out["payloadJson"] = decodeNetMetrics(payload)
			}
		}
		if err := enc.Encode(out); err != nil {
			log.Fatal(err)
		}
		reqID++
	}

	return out, nil
}

/**
 * @brief Sends one request and waits for a single UDP response.
 * @param conn Local UDP socket.
 * @param addr Remote simulator address.
 * @param qt Query type.
 * @param app ME app instance name.
 * @param rid Request ID.
 * @return Raw response bytes or I/O error.
 */
func queryOne(conn *net.UDPConn, addr *net.UDPAddr, qt uint32, app string, rid uint64) ([]byte, error) {
	req := packRequest(qt, rid, app)
	if _, err := conn.WriteToUDP(req, addr); err != nil {
		return nil, err
	}
	buf := make([]byte, 65536)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

/**
 * @brief Parses RNI UDP response header and extracts payload slice.
 * @param raw Full datagram bytes.
 * @return queryType, requestID, status code, payload, or parse error.
 */
func parseResponse(raw []byte) (queryType uint32, requestID uint64, status uint32, payload []byte, err error) {
	if len(raw) < hdrSize {
		return 0, 0, 0, nil, fmt.Errorf("short response %d", len(raw))
	}
	if binary.LittleEndian.Uint32(raw[0:4]) != magic {
		return 0, 0, 0, nil, fmt.Errorf("bad magic")
	}
	if binary.LittleEndian.Uint16(raw[4:6]) != protoVer {
		return 0, 0, 0, nil, fmt.Errorf("bad protoVer")
	}
	if binary.LittleEndian.Uint16(raw[6:8]) != msgResponse {
		return 0, 0, 0, nil, fmt.Errorf("not a response")
	}
	qt := binary.LittleEndian.Uint32(raw[8:12])
	rid := binary.LittleEndian.Uint64(raw[12:20])
	st := binary.LittleEndian.Uint32(raw[20:24])
	pl := binary.LittleEndian.Uint32(raw[24:28])
	if len(raw) < hdrSize+int(pl) {
		return 0, 0, 0, nil, fmt.Errorf("truncated payload need %d have %d", hdrSize+pl, len(raw))
	}
	return qt, rid, st, raw[hdrSize : hdrSize+int(pl)], nil
}

/** @brief Decodes QueryRab response payload to JSON-friendly structure. */
func decodeRab(p []byte) interface{} {
	type ue struct {
		Name            string `json:"ueName"`
		IPv4            string `json:"ipv4"`
		EcgiIdentity    uint32 `json:"ecgiCellIdentity"`
		Mcc             uint16 `json:"mcc"`
		Mnc             uint16 `json:"mnc"`
		Qci             uint8  `json:"qci"`
		ServingNodeId   uint8  `json:"servingNodeId"`
		NrServingNodeId uint8  `json:"nrServingNodeId"`
	}
	if len(p) < 8 {
		return nil
	}
	n := binary.LittleEndian.Uint16(p[0:2])
	out := map[string]interface{}{"numUe": n, "ues": []ue{}}
	ues := []ue{}
	off := 8
	for i := 0; i < int(n) && i < maxUE; i++ {
		if off+48 > len(p) {
			break
		}
		rec := p[off : off+48]
		off += 48
		ues = append(ues, ue{
			Name:            strings.TrimRight(string(rec[0:32]), "\x00"),
			IPv4:            ipv4StringLE(binary.LittleEndian.Uint32(rec[32:36])),
			EcgiIdentity:    binary.LittleEndian.Uint32(rec[36:40]),
			Mcc:             binary.LittleEndian.Uint16(rec[40:42]),
			Mnc:             binary.LittleEndian.Uint16(rec[42:44]),
			Qci:             rec[44],
			ServingNodeId:   rec[45],
			NrServingNodeId: rec[46],
		})
	}
	out["ues"] = ues
	return out
}

/** @brief Decodes QueryPlmn response payload. */
func decodePlmn(p []byte) interface{} {
	type cell struct {
		Mcc               uint16 `json:"mcc"`
		Mnc               uint16 `json:"mnc"`
		CellIdentity      uint32 `json:"cellIdentity"`
		AssociatedUeCount uint32 `json:"associatedUeCount"`
		IsNrCell          uint8  `json:"isNrCell"`
	}
	if len(p) < 8 {
		return nil
	}
	n := binary.LittleEndian.Uint16(p[0:2])
	out := map[string]interface{}{"numCells": n, "cells": []cell{}}
	cells := []cell{}
	off := 8
	for i := 0; i < int(n) && i < maxCells; i++ {
		if off+16 > len(p) {
			break
		}
		rec := p[off : off+16]
		off += 16
		cells = append(cells, cell{
			Mcc:               binary.LittleEndian.Uint16(rec[0:2]),
			Mnc:               binary.LittleEndian.Uint16(rec[2:4]),
			CellIdentity:      binary.LittleEndian.Uint32(rec[4:8]),
			AssociatedUeCount: binary.LittleEndian.Uint32(rec[8:12]),
			IsNrCell:          rec[12],
		})
	}
	out["cells"] = cells
	return out
}

/** @brief Decodes QueryS1Bearer response payload. */
func decodeS1(p []byte) interface{} {
	type row struct {
		Name     string `json:"ueName"`
		IPv4     string `json:"ipv4"`
		EnbTeid  uint32 `json:"enbTeid"`
		SgwTeid  uint32 `json:"sgwTeid"`
		BearerIx uint8  `json:"bearerIndex"`
	}
	if len(p) < 8 {
		return nil
	}
	n := binary.LittleEndian.Uint16(p[0:2])
	rows := []row{}
	off := 8
	for i := 0; i < int(n) && i < maxUE; i++ {
		if off+48 > len(p) {
			break
		}
		rec := p[off : off+48]
		off += 48
		rows = append(rows, row{
			Name:     strings.TrimRight(string(rec[0:32]), "\x00"),
			IPv4:     ipv4StringLE(binary.LittleEndian.Uint32(rec[32:36])),
			EnbTeid:  binary.LittleEndian.Uint32(rec[36:40]),
			SgwTeid:  binary.LittleEndian.Uint32(rec[40:44]),
			BearerIx: rec[44],
		})
	}
	return map[string]interface{}{"numUe": n, "bearers": rows}
}

/** @brief Decodes QueryLayer2 response payload. */
func decodeL2(p []byte) interface{} {
	type row struct {
		Name     string `json:"ueName"`
		IPv4     string `json:"ipv4"`
		RsrpDb10 int32  `json:"rsrpDb10"`
		RsrqDb10 int32  `json:"rsrqDb10"`
		DlPrbPct uint16 `json:"dlPrbPct"`
		UlPrbPct uint16 `json:"ulPrbPct"`
	}
	if len(p) < 8 {
		return nil
	}
	n := binary.LittleEndian.Uint16(p[0:2])
	rows := []row{}
	off := 8
	for i := 0; i < int(n) && i < maxUE; i++ {
		if off+48 > len(p) {
			break
		}
		rec := p[off : off+48]
		off += 48
		rows = append(rows, row{
			Name:     strings.TrimRight(string(rec[0:32]), "\x00"),
			IPv4:     ipv4StringLE(binary.LittleEndian.Uint32(rec[32:36])),
			RsrpDb10: int32(binary.LittleEndian.Uint32(rec[36:40])),
			RsrqDb10: int32(binary.LittleEndian.Uint32(rec[40:44])),
			DlPrbPct: binary.LittleEndian.Uint16(rec[44:46]),
			UlPrbPct: binary.LittleEndian.Uint16(rec[46:48]),
		})
	}
	return map[string]interface{}{"numUe": n, "layer2": rows}
}

/** @brief Decodes QueryNetworkMetrics response payload. */
func decodeNetMetrics(p []byte) interface{} {
	type row struct {
		Name             string `json:"ueName"`
		IPv4             string `json:"ipv4"`
		DlThroughputBps  int32  `json:"dlThroughputBps"`
		UlThroughputBps  int32  `json:"ulThroughputBps"`
		DlLatencyMs      int32  `json:"dlLatencyMs"`
		UlLatencyMs      int32  `json:"ulLatencyMs"`
		DlPacketLossMean int32  `json:"dlPacketLossMean"`
		UlPacketLossMean int32  `json:"ulPacketLossMean"`
		DlJitterMs       int32  `json:"dlJitterMs"`
		UlJitterMs       int32  `json:"ulJitterMs"`
		StatsSource      uint8  `json:"statsSource"`
		StatsSourceText  string `json:"statsSourceText"`
	}
	if len(p) < 8 {
		return nil
	}
	n := binary.LittleEndian.Uint16(p[0:2])
	rows := []row{}
	off := 8
	for i := 0; i < int(n) && i < maxUE; i++ {
		if off+netMetricsUeSize > len(p) {
			break
		}
		rec := p[off : off+netMetricsUeSize]
		off += netMetricsUeSize
		src := rec[68]
		st := "none"
		switch src {
		case 1:
			st = "lte_enb"
		case 2:
			st = "nr_gnb"
		}
		rows = append(rows, row{
			Name:             strings.TrimRight(string(rec[0:32]), "\x00"),
			IPv4:             ipv4StringLE(binary.LittleEndian.Uint32(rec[32:36])),
			DlThroughputBps:  int32(binary.LittleEndian.Uint32(rec[36:40])),
			UlThroughputBps:  int32(binary.LittleEndian.Uint32(rec[40:44])),
			DlLatencyMs:      int32(binary.LittleEndian.Uint32(rec[44:48])),
			UlLatencyMs:      int32(binary.LittleEndian.Uint32(rec[48:52])),
			DlPacketLossMean: int32(binary.LittleEndian.Uint32(rec[52:56])),
			UlPacketLossMean: int32(binary.LittleEndian.Uint32(rec[56:60])),
			DlJitterMs:       int32(binary.LittleEndian.Uint32(rec[60:64])),
			UlJitterMs:       int32(binary.LittleEndian.Uint32(rec[64:68])),
			StatsSource:      src,
			StatsSourceText:  st,
		})
	}
	return map[string]interface{}{
		"numUe":          n,
		"networkMetrics": rows,
		"notes": map[string]string{
			"dlJitterMs_ulJitterMs": "Absolute change in reported mean latency since the previous network_metrics response for this UE (-1 = first/unknown). Not RFC 3550 IPDV.",
			"packetLossMeans":       "Raw Simu5G UeStatsCollector discard-metric means; scales differ UL vs DL. -1 = no collector.",
		},
	}
}

/**
 * @brief Human-readable name for an RNI query type constant.
 * @param q Query type ID.
 * @return Short name (e.g. network_metrics).
 */
func queryName(q uint32) string {
	switch q {
	case QueryRab:
		return "rab_info"
	case QueryPlmn:
		return "plmn_info"
	case QueryS1Bearer:
		return "s1_bearer_info"
	case QueryLayer2:
		return "layer2_meas"
	case QueryNetworkMetrics:
		return "network_metrics"
	default:
		return fmt.Sprintf("unknown(%d)", q)
	}
}

/*
 * Copyright (c) 2025  The AdvantEDGE Authors
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
 * @file telemetry.go
 * @brief UE telemetry UDP decoder (simulator host S → sandbox host B).
 *
 * @details
 * Binary format from monaco-harbor UeTelemetryUdpSender: little-endian header
 * (magic 0x31544555) and fixed-size UE records (v1 WGS84, v2/v3 scene coords).
 * Used by meep-simu5g-mobility-adapter on UDP :9100.
 *
 * @ingroup simu5g_bridge_pkg
 */
package bridge

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

/** @defgroup telemetry_wire UE telemetry UDP wire format (little-endian). */
/** @{ */
const (
	magicUET1      uint32 = 0x31544555
	protoVersion1  uint16 = 1
	protoVersion2  uint16 = 2
	protoVersion3  uint16 = 3
	headerSize            = 32
	recordSizeV1V2        = 153
	recordSizeV3          = 157
)

/** @} */

/**
 * @brief UDP datagram header from UeTelemetryUdpSender.
 */
type Header struct {
	Magic       uint32
	Version     uint16
	Reserved    uint16
	Seq         uint64
	SimTimeSecs float64
	NumUE       uint16
	Pad         [6]byte
}

/**
 * @brief Fixed-layout UE record inside a telemetry datagram.
 */
type UERecord struct {
	IDLen     uint8
	ID        [64]byte
	A         float64 // v1: lat; v2: pos X (m)
	B         float64 // v1: lon; v2: pos Y (m)
	C         float64 // v1: alt; v2: pos Z (m)
	VelSceneX float64
	VelSceneY float64
	VelSceneZ float64
	SpeedMps  float64
	OrientQw  float64
	OrientQx  float64
	OrientQy  float64
	OrientQz  float64
}

/**
 * @brief Decoded telemetry batch (JSON-serializable).
 */
type TelemetryPacket struct {
	ProtoVersion uint16   `json:"protoVersion"`
	Seq          uint64   `json:"seq"`
	SimTimeSecs  float64  `json:"simTimeSecs"`
	NumUE        int      `json:"numUe"`
	UEs          []UELine `json:"ues"`
	GapDetected  bool     `json:"gapDetected"`
	ExpectedSeq  uint64   `json:"expectedSeq,omitempty"`
}

/**
 * @brief One UE line in a decoded telemetry packet.
 */
type UELine struct {
	ID        string  `json:"id"`
	IPv4      string  `json:"ipv4,omitempty"`
	SceneXM   float64 `json:"sceneXM,omitempty"`
	SceneYM   float64 `json:"sceneYM,omitempty"`
	SceneZM   float64 `json:"sceneZM,omitempty"`
	LatDeg    float64 `json:"latDeg"`
	LonDeg    float64 `json:"lonDeg"`
	AltM      float64 `json:"altM"`
	VelSceneX float64 `json:"velSceneX"`
	VelSceneY float64 `json:"velSceneY"`
	VelSceneZ float64 `json:"velSceneZ"`
	SpeedMps  float64 `json:"speedMps"`
	OrientQw  float64 `json:"orientQw"`
	OrientQx  float64 `json:"orientQx"`
	OrientQy  float64 `json:"orientQy"`
	OrientQz  float64 `json:"orientQz"`
}

/** @brief Expected next sequence number (0 = uninitialized). */
var nextSeq uint64

/**
 * @brief Decodes a telemetry UDP datagram from simulator host S.
 * @param pkt Raw UDP payload.
 * @param opts Scene reference for WGS84 conversion and JSONOut logging.
 * @return Parsed TelemetryPacket or validation/sequence error.
 */
func ParseTelemetryDatagram(pkt []byte, opts TelemetryOptions) (telemetry_packet *TelemetryPacket, err error) {

	if len(pkt) < headerSize {
		return nil, errors.New("telemetry datagram too short")
	}
	var h Header
	r := binary.LittleEndian
	base := pkt[:headerSize]
	h.Magic = r.Uint32(base[0:4])
	h.Version = r.Uint16(base[4:6])
	h.Reserved = r.Uint16(base[6:8])
	h.Seq = r.Uint64(base[8:16])
	h.SimTimeSecs = mathFloat64(base[16:24])
	h.NumUE = r.Uint16(base[24:26])
	copy(h.Pad[:], base[26:32])

	if h.Magic != magicUET1 {
		err = fmt.Errorf("ParseTelemetryDatagram: bad magic 0x%x", h.Magic)
		return nil, err
	}
	if h.Version != protoVersion1 && h.Version != protoVersion2 && h.Version != protoVersion3 {
		err = fmt.Errorf("ParseTelemetryDatagram: unsupported version %d", h.Version)
		return nil, err
	}
	rsz := recordSizeForVersion(h.Version)
	if rsz < 0 {
		err = fmt.Errorf("ParseTelemetryDatagram: bad record size for version %d", h.Version)
		return nil, err
	}
	expect := int(headerSize) + int(h.NumUE)*rsz
	if len(pkt) < expect {
		err = fmt.Errorf("ParseTelemetryDatagram: truncated payload: got %d need %d (numUe=%d)", len(pkt), expect, h.NumUE)
		return nil, err
	}

	gap := false
	var exp uint64
	if nextSeq == 0 {
		nextSeq = h.Seq + 1
	} else if h.Seq == nextSeq {
		nextSeq++
	} else if h.Seq > nextSeq {
		gap = true
		lost := h.Seq - nextSeq
		exp = nextSeq
		log.Info("ParseTelemetryDatagram: sequence gap: got " + strconv.FormatUint(h.Seq, 10) + " expected " + strconv.FormatUint(nextSeq, 10) + " (missing " + strconv.FormatUint(lost, 10) + " messages)")
		nextSeq = h.Seq + 1
	} else {
		err = fmt.Errorf("ParseTelemetryDatagram: out-of-order: seq=%d expected>=%d", h.Seq, nextSeq)
		return nil, err
	}

	ues := make([]UELine, 0, h.NumUE)
	off := headerSize
	for i := 0; i < int(h.NumUE); i++ {
		rec := base[off : off+rsz]
		off += rsz
		idLen := rec[0]
		if idLen > 63 {
			idLen = 63
		}
		id := string(rec[1 : 1+idLen])
		ur := parseUERecordCore(rec)

		line := UELine{
			ID:        id,
			VelSceneX: ur.VelSceneX, VelSceneY: ur.VelSceneY, VelSceneZ: ur.VelSceneZ,
			SpeedMps: ur.SpeedMps,
			OrientQw: ur.OrientQw, OrientQx: ur.OrientQx, OrientQy: ur.OrientQy, OrientQz: ur.OrientQz,
		}
		switch h.Version {
		case protoVersion1:
			line.LatDeg, line.LonDeg, line.AltM = ur.A, ur.B, ur.C
		case protoVersion2:
			line.SceneXM, line.SceneYM, line.SceneZM = ur.A, ur.B, ur.C
			line.LatDeg, line.LonDeg, line.AltM = sceneToWGS84(ur.A, ur.B, ur.C, opts.SceneLatDeg, opts.SceneLonDeg, opts.SceneAltM, opts.MetersPerDeg)
		case protoVersion3:
			line.SceneXM, line.SceneYM, line.SceneZM = ur.A, ur.B, ur.C
			line.LatDeg, line.LonDeg, line.AltM = sceneToWGS84(ur.A, ur.B, ur.C, opts.SceneLatDeg, opts.SceneLonDeg, opts.SceneAltM, opts.MetersPerDeg)
			line.IPv4 = ipv4StringFromRecordTail(rec)
		}
		ues = append(ues, line)
	} // End of 'for' statement

	telemetry_packet = &TelemetryPacket{
		ProtoVersion: h.Version,
		Seq:          h.Seq, SimTimeSecs: h.SimTimeSecs, NumUE: int(h.NumUE), UEs: ues,
		GapDetected: gap, ExpectedSeq: exp,
	}
	if opts.JSONOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(telemetry_packet); err != nil {
			err = fmt.Errorf("ParseTelemetryDatagram: json encode error: %v", err)
		}
	} else {
		log.Info("seq=" + strconv.FormatUint(h.Seq, 10) + " ver=" + strconv.FormatUint(uint64(h.Version), 10) + " t=" + strconv.FormatFloat(h.SimTimeSecs, 'f', -1, 64) + " n=" + strconv.FormatInt(int64(h.NumUE), 10) + " gap=" + strconv.FormatBool(gap))
	}

	return telemetry_packet, nil
}

/**
 * @brief Parses position, velocity, and orientation fields from a UE record slice.
 * @param rec Record bytes (v1/v2/v3 fixed size).
 */
func parseUERecordCore(rec []byte) UERecord {
	return UERecord{
		IDLen:     rec[0],
		A:         mathFloat64(rec[65:73]),
		B:         mathFloat64(rec[73:81]),
		C:         mathFloat64(rec[81:89]),
		VelSceneX: mathFloat64(rec[89:97]),
		VelSceneY: mathFloat64(rec[97:105]),
		VelSceneZ: mathFloat64(rec[105:113]),
		SpeedMps:  mathFloat64(rec[113:121]),
		OrientQw:  mathFloat64(rec[121:129]),
		OrientQx:  mathFloat64(rec[129:137]),
		OrientQy:  mathFloat64(rec[137:145]),
		OrientQz:  mathFloat64(rec[145:153]),
	}
}

/**
 * @brief Returns per-UE record byte length for a protocol version.
 * @param v Protocol version (1, 2, or 3).
 * @return Record size or -1 if unsupported.
 */
func recordSizeForVersion(v uint16) int {
	switch v {
	case protoVersion1, protoVersion2:
		return recordSizeV1V2
	case protoVersion3:
		return recordSizeV3
	default:
		return -1
	}
}

/**
 * @brief Extracts IPv4 string from v3 record tail (bytes 153–156).
 */
func ipv4StringFromRecordTail(rec []byte) string {
	if len(rec) < recordSizeV3 {
		return ""
	}
	ip := net.IP(rec[153:157])
	if len(ip) != net.IPv4len || ip.Equal(net.IPv4zero) {
		return ""
	}
	return ip.String()
}

/**
 * @brief Converts scene coordinates to WGS84 degrees.
 *
 * @details Matches INET SimpleGeographicCoordinateSystem::computeGeographicCoordinate;
 * cosine factor uses scene reference latitude, not the point latitude.
 *
 * Scene XY must already be INET geographic metres (OSM NW origin, Y south).
 * MonacoVeinsInetManager remaps TraCI UTM playground XY into that frame on S;
 * using SUMO origBoundary metres here would shift UEs off the EdgeNative map.
 *
 * @param x Scene X (m), y Scene Y (m), z Scene Z (m).
 * @param sceneLatDeg Reference latitude for the scenario origin.
 * @param sceneLonDeg Reference longitude.
 * @param sceneAltM Reference altitude (m).
 * @param metersPerDeg Meters per degree for lat/lon scaling.
 * @return latDeg, lonDeg, altM.
 */
func sceneToWGS84(x, y, z, sceneLatDeg, sceneLonDeg, sceneAltM, metersPerDeg float64) (latDeg, lonDeg, altM float64) {
	sceneLatRad := sceneLatDeg * math.Pi / 180.0
	latDeg = sceneLatDeg - y/metersPerDeg
	lonDeg = sceneLonDeg + x/metersPerDeg/math.Cos(math.Abs(sceneLatRad))
	altM = z - sceneAltM
	return latDeg, lonDeg, altM
}

/** @brief Reads little-endian float64 from eight bytes. */
func mathFloat64(b []byte) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(b))
}

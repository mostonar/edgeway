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
 * @file protocol.go
 * @brief TCP bridge framing between sandbox host B and simulator host S.
 *
 * @details
 * Wire format: 8-byte header (magic 0x5335, version, type, length) followed by
 * payload. DATA_UL carries uplink IP from B; DATA_DL carries downlink IP to B.
 *
 * @see docs/Simu5G/interfaces/bridge-protocol.md
 * @ingroup simu5g_bridge_pkg
 */
package bridge

import (
	"encoding/binary"
	"time"
)

/** @defgroup bridge_framing TCP bridge frame constants */
/** @{ */
const (
	FrameMagic   = 0x5335 // 'S5' — frame magic number
	FrameVersion = 0x01   // Protocol version

	FrameTypeDataUL  = 0x01 // Uplink user IP (B → S)
	FrameTypeDataDL  = 0x02 // Downlink user IP (S → B)
	FrameTypeControl = 0x10 // Control plane message

	FrameHeaderSize       = 8  // Bytes in frame header
	DataPayloadHeaderSize = 11 // ueId(2) + flags(1) + simTime(8)
)

const (
	ControlCodeHello    = 0x01 // Session hello
	ControlCodeHelloAck = 0x02 // Hello acknowledgment
	ControlCodePing     = 0x03 // Keepalive ping
	ControlCodePong     = 0x04 // Keepalive pong
	ControlCodeUEAttach = 0x05 // UE attach notification
	ControlCodeUEDetach = 0x06 // UE detach notification
)

/** @} */

/**
 * @brief Parsed 8-byte TCP frame header.
 */
type FrameHeader struct {
	Magic   uint16
	Version uint8
	Type    uint8
	Length  uint32
}

/**
 * @brief DATA_UL payload: UE index, flags, simulation time, raw IP packet.
 */
type DataULPayload struct {
	UeID    uint16
	Flags   uint8
	SimTime int64
	Packet  []byte
}

/**
 * @brief DATA_DL payload: destination index, flags, simulation time, raw IP packet.
 */
type DataDLPayload struct {
	DestID  uint16
	Flags   uint8
	SimTime int64
	Packet  []byte
}

/**
 * @brief CONTROL frame payload: opcode and optional JSON body.
 */
type ControlPayload struct {
	Code uint8
	Body []byte
}

/**
 * @brief Maps one sandbox UE pod CIDR to a Simu5G module path on S.
 */
type UEMapping struct {
	UeID             string `json:"ueId"`
	SandboxIPCidr    string `json:"sandboxIpCidr"`
	Simu5gModulePath string `json:"simu5gModulePath"`
}

/**
 * @brief Maps one MEC application pod CIDR to a Simu5G endpoint on S.
 */
type MECEndpoint struct {
	EndpointID       string `json:"endpointId"`
	SandboxIPCidr    string `json:"sandboxIpCidr"`
	Simu5gModulePath string `json:"simu5gModulePath"`
}

/**
 * @brief Maps a POA/cell identifier to Simu5G gNodeB module path.
 */
type CellMapping struct {
	PoaID            string `json:"poaId"`
	CellID           uint32 `json:"cellId"`
	Simu5gModulePath string `json:"simu5gModulePath"`
}

/**
 * @brief Runtime tuning (MTU, queues, real-time tolerance).
 */
type RuntimeOptions struct {
	MTU                 int `json:"mtu"`
	MaxPendingPackets   int `json:"maxPendingPackets"`
	RealTimeToleranceMs int `json:"realTimeToleranceMs"`
}

/**
 * @brief Builds a complete DATA_UL frame for TCP transmission B → S.
 * @param ueID Index into ueMappings (0 if unknown).
 * @param packet Raw IPv4/IPv6 packet bytes from TUN.
 * @param simTimeNs Simulation timestamp in nanoseconds (0 = use wall clock).
 * @return Serialized frame including header and payload.
 */
func EncodeDataULFrame(ueID uint16, packet []byte, simTimeNs int64) []byte {
	if simTimeNs == 0 {
		simTimeNs = time.Now().UnixNano()
	}
	payloadLen := DataPayloadHeaderSize + len(packet)
	frame := make([]byte, FrameHeaderSize+payloadLen)

	binary.BigEndian.PutUint16(frame[0:2], FrameMagic)
	frame[2] = FrameVersion
	frame[3] = FrameTypeDataUL
	binary.BigEndian.PutUint32(frame[4:8], uint32(payloadLen))

	off := FrameHeaderSize
	binary.BigEndian.PutUint16(frame[off:off+2], ueID)
	off += 2
	frame[off] = 0
	off++
	binary.BigEndian.PutUint64(frame[off:off+8], uint64(simTimeNs))
	off += 8
	copy(frame[off:], packet)

	return frame
}

/**
 * @brief Extracts the IP packet from a validated DATA_DL payload body.
 * @param payload Frame payload after header (destId + flags + simTime + packet).
 * @return IP packet bytes and true on success.
 */
func PacketFromDataDLFrame(payload []byte) ([]byte, bool) {
	if len(payload) <= DataPayloadHeaderSize {
		return nil, false
	}
	out := make([]byte, len(payload)-DataPayloadHeaderSize)
	copy(out, payload[DataPayloadHeaderSize:])
	return out, true
}

/**
 * @brief Builds a minimal HELLO control frame from sandbox host B.
 * @param role Client role string (e.g. "bridge").
 * @return Serialized CONTROL frame with JSON capability body.
 */
func EncodeControlHello(role string) []byte {
	body := []byte(`{"role":"` + role + `","version":"0.1"}`)
	payloadLen := 4 + len(body)
	frame := make([]byte, FrameHeaderSize+payloadLen)
	binary.BigEndian.PutUint16(frame[0:2], FrameMagic)
	frame[2] = FrameVersion
	frame[3] = FrameTypeControl
	binary.BigEndian.PutUint32(frame[4:8], uint32(payloadLen))
	frame[FrameHeaderSize] = ControlCodeHello
	copy(frame[FrameHeaderSize+4:], body)
	return frame
}

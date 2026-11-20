package bridge

import (
	"encoding/binary"
	"math"
	"net"
	"testing"
)

func putF64(b []byte, v float64) {
	binary.LittleEndian.PutUint64(b, math.Float64bits(v))
}

func encodeTelemetryV3(seq uint64, simTime float64, id, ipv4 string, sceneX, sceneY, sceneZ, speed float64) []byte {
	pkt := make([]byte, headerSize+recordSizeV3)
	le := binary.LittleEndian
	le.PutUint32(pkt[0:4], magicUET1)
	le.PutUint16(pkt[4:6], protoVersion3)
	le.PutUint64(pkt[8:16], seq)
	putF64(pkt[16:24], simTime)
	le.PutUint16(pkt[24:26], 1)
	rec := pkt[headerSize:]
	if len(id) > 63 {
		id = id[:63]
	}
	rec[0] = byte(len(id))
	copy(rec[1:], id)
	putF64(rec[65:73], sceneX)
	putF64(rec[73:81], sceneY)
	putF64(rec[81:89], sceneZ)
	putF64(rec[113:121], speed)
	ip := net.ParseIP(ipv4).To4()
	if ip != nil {
		copy(rec[153:157], ip)
	}
	return pkt
}

/** @brief ParseTelemetryDatagram decodes a v3 scene-coordinate record to WGS-84. */
func TestParseTelemetryDatagramV3(t *testing.T) {
	nextSeq = 0
	opts := TelemetryOptions{
		SceneLatDeg:  43.734042,
		SceneLonDeg:  7.416587,
		MetersPerDeg: 111320,
	}
	pkt := encodeTelemetryV3(1, 12.5, "MonacoHarbor.ue_car[0]", "10.10.1.10", 499.7, 338.6, 1.5, 8.25)
	got, err := ParseTelemetryDatagram(pkt, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.NumUE != 1 || got.Seq != 1 || got.ProtoVersion != protoVersion3 {
		t.Fatalf("header %+v", got)
	}
	ue := got.UEs[0]
	if ue.ID != "MonacoHarbor.ue_car[0]" {
		t.Fatalf("id %q", ue.ID)
	}
	if ue.IPv4 != "10.10.1.10" {
		t.Fatalf("ipv4 %q", ue.IPv4)
	}
	if ue.LatDeg == 0 || ue.LonDeg == 0 {
		t.Fatalf("wgs84 lat=%v lon=%v", ue.LatDeg, ue.LonDeg)
	}
	if math.Abs(ue.SpeedMps-8.25) > 1e-9 {
		t.Fatalf("speed %v", ue.SpeedMps)
	}
}

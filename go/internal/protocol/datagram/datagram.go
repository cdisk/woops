package datagram

import (
	"encoding/binary"
	"fmt"
)

// Framing over Agent WSS (forward/reverse UDP tunnels):
// [2 BE hostLen][host][2 BE port][4 BE dataLen][data]
//
// For gateway→asset traffic, host/port identify the original UDP client.
// For asset→gateway replies, the same fields carry the reply destination.
func Encode(host string, port int, payload []byte) []byte {
	hb := []byte(host)
	out := make([]byte, 2+len(hb)+2+4+len(payload))
	binary.BigEndian.PutUint16(out[0:], uint16(len(hb)))
	copy(out[2:], hb)
	o := 2 + len(hb)
	binary.BigEndian.PutUint16(out[o:], uint16(port))
	o += 2
	binary.BigEndian.PutUint32(out[o:], uint32(len(payload)))
	o += 4
	copy(out[o:], payload)
	return out
}

func Decode(data []byte) (host string, port int, payload []byte, err error) {
	if len(data) < 8 {
		return "", 0, nil, fmt.Errorf("short frame")
	}
	hl := int(binary.BigEndian.Uint16(data[0:]))
	if len(data) < 2+hl+2+4 {
		return "", 0, nil, fmt.Errorf("short host")
	}
	host = string(data[2 : 2+hl])
	o := 2 + hl
	port = int(binary.BigEndian.Uint16(data[o:]))
	o += 2
	dl := int(binary.BigEndian.Uint32(data[o:]))
	o += 4
	if len(data) < o+dl {
		return "", 0, nil, fmt.Errorf("short payload")
	}
	return host, port, data[o : o+dl], nil
}

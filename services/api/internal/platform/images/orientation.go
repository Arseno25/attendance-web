package images

import (
	"bytes"
	"encoding/binary"
)

// Read only the bounded orientation entry; other metadata is discarded during encoding.
func orientation(raw []byte, format string) int {
	if format == "png" {
		for at := 8; at+12 <= len(raw); {
			size := int(binary.BigEndian.Uint32(raw[at : at+4]))
			if size < 0 || size > len(raw)-at-12 {
				return 1
			}
			if string(raw[at+4:at+8]) == "eXIf" {
				return tiffOrientation(raw[at+8 : at+8+size])
			}
			at += 12 + size
		}
	} else {
		for at := 2; at+4 <= len(raw); {
			if raw[at] != 0xff || raw[at+1] == 0xda || raw[at+1] == 0xd9 {
				return 1
			}
			if raw[at+1] == 0xff {
				at++
				continue
			}
			size := int(binary.BigEndian.Uint16(raw[at+2 : at+4]))
			if size < 2 || size > len(raw)-at-2 {
				return 1
			}
			data := raw[at+4 : at+2+size]
			if raw[at+1] == 0xe1 && bytes.HasPrefix(data, []byte("Exif\x00\x00")) {
				return tiffOrientation(data[6:])
			}
			at += 2 + size
		}
	}
	return 1
}
func tiffOrientation(data []byte) int {
	if len(data) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(data[2:4]) != 42 {
		return 1
	}
	offset := int(order.Uint32(data[4:8]))
	if offset < 8 || offset > len(data)-2 {
		return 1
	}
	count := int(order.Uint16(data[offset : offset+2]))
	offset += 2
	for i := 0; i < count && offset <= len(data)-12; i++ {
		entry := data[offset : offset+12]
		if order.Uint16(entry[:2]) == 0x0112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
			value := int(order.Uint16(entry[8:10]))
			if value >= 1 && value <= 8 {
				return value
			}
			return 1
		}
		offset += 12
	}
	return 1
}

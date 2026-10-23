// Package vita reads the small pieces of Vita file formats the installer needs.
package vita

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
)

// ParseSFO decodes a PARAM.SFO file into a map of string and integer values.
func ParseSFO(b []byte) (map[string]string, error) {
	if len(b) < 20 || !bytes.Equal(b[:4], []byte("\x00PSF")) {
		return nil, errors.New("not a PARAM.SFO file")
	}
	le := binary.LittleEndian
	keyTable := int(le.Uint32(b[8:]))
	dataTable := int(le.Uint32(b[12:]))
	count := int(le.Uint32(b[16:]))
	if count > 1024 || keyTable > len(b) || dataTable > len(b) || 20+count*16 > len(b) {
		return nil, errors.New("damaged PARAM.SFO")
	}
	out := make(map[string]string, count)
	for i := 0; i < count; i++ {
		e := b[20+i*16:]
		keyOff := keyTable + int(le.Uint16(e[0:]))
		format := le.Uint16(e[2:])
		length := int(le.Uint32(e[4:]))
		dataOff := dataTable + int(le.Uint32(e[12:]))
		if keyOff >= len(b) || dataOff < 0 || length < 0 || dataOff+length > len(b) {
			return nil, errors.New("damaged PARAM.SFO")
		}
		key := b[keyOff:]
		if n := bytes.IndexByte(key, 0); n >= 0 {
			key = key[:n]
		}
		data := b[dataOff : dataOff+length]
		switch format {
		case 0x0404:
			if len(data) >= 4 {
				out[string(key)] = itoa(le.Uint32(data))
			}
		default:
			out[string(key)] = strings.TrimRight(string(data), "\x00")
		}
	}
	return out, nil
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

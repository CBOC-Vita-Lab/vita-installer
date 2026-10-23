// Command macho prepares the Mac app on any build machine:
//
//	macho uuid FILE          add an LC_UUID load command (and re-sign arm64)
//	macho fat OUT IN1 IN2... join thin binaries into a universal binary
//
// Go 1.20 (needed for macOS 10.13 support) does not write LC_UUID, and
// macOS 15 refuses local network access to programs without one, which
// would stop the installer from finding the Vita on Wi-Fi.
package main

import (
	"bytes"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "uuid" {
		check(addUUID(os.Args[2]))
		return
	}
	if len(os.Args) >= 5 && os.Args[1] == "fat" {
		check(fat(os.Args[2], os.Args[3:]))
		return
	}
	fmt.Fprintln(os.Stderr, "usage: macho uuid FILE | macho fat OUT IN1 IN2...")
	os.Exit(2)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const (
	lcUUID          = 0x1b
	lcCodeSignature = 0x1d
	lcSegment64     = 0x19
)

func addUUID(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	le := binary.LittleEndian
	if len(data) < 32 || le.Uint32(data) != macho.Magic64 {
		return errors.New("not a 64-bit Mach-O file")
	}
	ncmds, sizeofcmds := le.Uint32(data[16:]), le.Uint32(data[20:])
	firstData := uint64(len(data))
	sigOff, sigSize := uint32(0), uint32(0)
	var textOff, textSize uint64
	off := uint32(32)
	for i := uint32(0); i < ncmds; i++ {
		cmd, size := le.Uint32(data[off:]), le.Uint32(data[off+4:])
		switch cmd {
		case lcUUID:
			return nil // already there
		case lcCodeSignature:
			sigOff, sigSize = le.Uint32(data[off+8:]), le.Uint32(data[off+12:])
		case lcSegment64:
			name := string(bytes.TrimRight(data[off+8:off+24], "\x00"))
			if name == "__TEXT" {
				textOff, textSize = le.Uint64(data[off+40:]), le.Uint64(data[off+48:])
			}
			nsects := le.Uint32(data[off+64:])
			for s := uint32(0); s < nsects; s++ {
				sec := data[off+72+s*80:]
				if o := uint64(le.Uint32(sec[48:])); o > 0 && o < firstData {
					firstData = o
				}
			}
		}
		off += size
	}
	end := 32 + sizeofcmds
	if uint64(end)+24 > firstData {
		return errors.New("no room for LC_UUID in the Mach-O header")
	}
	sum := sha256.Sum256(data)
	uuid := sum[:16]
	uuid[6] = uuid[6]&0x0f | 0x50 // a name-based style UUID
	uuid[8] = uuid[8]&0x3f | 0x80
	le.PutUint32(data[end:], lcUUID)
	le.PutUint32(data[end+4:], 24)
	copy(data[end+8:], uuid)
	le.PutUint32(data[16:], ncmds+1)
	le.PutUint32(data[20:], sizeofcmds+24)
	if sigOff != 0 {
		// The header changed, so the ad-hoc signature must be recomputed.
		if Size(int64(sigOff), "a.out") != int64(sigSize) {
			return errors.New("unexpected code signature size")
		}
		sig := make([]byte, sigSize)
		Sign(sig, bytes.NewReader(data[:sigOff]), "a.out", int64(sigOff), int64(textOff), int64(textSize), macho.Type(le.Uint32(data[12:])) == macho.TypeExec)
		copy(data[sigOff:], sig)
	}
	return os.WriteFile(path, data, 0o755)
}

func fat(out string, inputs []string) error {
	type slice struct {
		data          []byte
		cpu, sub      uint32
		offset, align uint32
	}
	var slices []slice
	offset := uint32(1 << 14)
	for _, p := range inputs {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := macho.Open(p)
		if err != nil {
			return err
		}
		slices = append(slices, slice{data: data, cpu: uint32(f.Cpu), sub: f.SubCpu, align: 14, offset: offset})
		f.Close()
		offset += (uint32(len(data)) + (1<<14 - 1)) &^ (1<<14 - 1)
	}
	buf := make([]byte, offset)
	be := binary.BigEndian
	be.PutUint32(buf[0:], macho.MagicFat)
	be.PutUint32(buf[4:], uint32(len(slices)))
	for i, s := range slices {
		h := buf[8+i*20:]
		be.PutUint32(h[0:], s.cpu)
		be.PutUint32(h[4:], s.sub)
		be.PutUint32(h[8:], s.offset)
		be.PutUint32(h[12:], uint32(len(s.data)))
		be.PutUint32(h[16:], s.align)
		copy(buf[s.offset:], s.data)
	}
	return os.WriteFile(out, buf, 0o755)
}

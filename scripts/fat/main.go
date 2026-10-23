// Command fat joins Mach-O binaries into one universal binary (like lipo -create).
package main

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: fat OUT IN1 IN2...")
		os.Exit(2)
	}
	type slice struct {
		data          []byte
		cpu, sub      uint32
		offset, align uint32
	}
	var slices []slice
	offset := uint32(1 << 14)
	for _, p := range os.Args[2:] {
		data, err := os.ReadFile(p)
		check(err)
		f, err := macho.Open(p)
		check(err)
		s := slice{data: data, cpu: uint32(f.Cpu), sub: f.SubCpu, align: 14, offset: offset}
		f.Close()
		slices = append(slices, s)
		offset += (uint32(len(data)) + (1<<14 - 1)) &^ (1<<14 - 1)
	}
	out := make([]byte, offset)
	be := binary.BigEndian
	be.PutUint32(out[0:], macho.MagicFat)
	be.PutUint32(out[4:], uint32(len(slices)))
	for i, s := range slices {
		h := out[8+i*20:]
		be.PutUint32(h[0:], s.cpu)
		be.PutUint32(h[4:], s.sub)
		be.PutUint32(h[8:], s.offset)
		be.PutUint32(h[12:], uint32(len(s.data)))
		be.PutUint32(h[16:], s.align)
		copy(out[s.offset:], s.data)
	}
	check(os.WriteFile(os.Args[1], out, 0o755))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

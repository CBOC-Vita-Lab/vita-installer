package fakevita

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
)

// SFO builds a PARAM.SFO with string values. TITLE_ID is written last.
func SFO(titleID, title, version string) []byte {
	keys := []string{"APP_VER", "TITLE", "TITLE_ID"}
	vals := []string{version, title, titleID}
	var keyTable, dataTable bytes.Buffer
	type ent struct{ ko, dl, dm, do uint32 }
	var ents []ent
	for i, k := range keys {
		ko := uint32(keyTable.Len())
		keyTable.WriteString(k)
		keyTable.WriteByte(0)
		v := []byte(vals[i])
		dl := uint32(len(v) + 1)
		dm := (dl + 3) &^ 3
		do := uint32(dataTable.Len())
		dataTable.Write(v)
		dataTable.Write(make([]byte, dm-uint32(len(v))))
		ents = append(ents, ent{ko, dl, dm, do})
	}
	for keyTable.Len()%4 != 0 {
		keyTable.WriteByte(0)
	}
	le := binary.LittleEndian
	hdr := make([]byte, 20+16*len(ents))
	copy(hdr, "\x00PSF")
	le.PutUint32(hdr[4:], 0x101)
	le.PutUint32(hdr[8:], uint32(len(hdr)))
	le.PutUint32(hdr[12:], uint32(len(hdr)+keyTable.Len()))
	le.PutUint32(hdr[16:], uint32(len(ents)))
	for i, e := range ents {
		b := hdr[20+i*16:]
		le.PutUint16(b[0:], uint16(e.ko))
		le.PutUint16(b[2:], 0x0204)
		le.PutUint32(b[4:], e.dl)
		le.PutUint32(b[8:], e.dm)
		le.PutUint32(b[12:], e.do)
	}
	return append(append(hdr, keyTable.Bytes()...), dataTable.Bytes()...)
}

// MakeVPK writes a small installable VPK. extra adds more files.
func MakeVPK(path, titleID, title string, extra map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	files := map[string][]byte{
		"eboot.bin":                              append([]byte("SCE\x00fake eboot "), bytes.Repeat([]byte{7}, 4096)...),
		"sce_sys/param.sfo":                      SFO(titleID, title, "01.00"),
		"sce_sys/icon0.png":                      []byte("\x89PNG\r\n\x1a\nfake"),
		"sce_sys/livearea/contents/template.xml": []byte("<livearea/>"),
	}
	for k, v := range extra {
		files[k] = v
	}
	for name, data := range files {
		w, _ := z.Create(name)
		w.Write(data)
	}
	if err := z.Close(); err != nil {
		return err
	}
	return f.Close()
}

// PSPISO builds a tiny PSP disc image that identifies as gameID (for example
// ULUS-10054). marker is extra text at the end, for tests to recognise a disc.
func PSPISO(gameID, marker string) []byte {
	const s = 2048
	img := make([]byte, 24*s)
	pvd := img[16*s:]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	root := pvd[156:]
	root[0] = 34
	binary.LittleEndian.PutUint32(root[2:], 20)
	binary.LittleEndian.PutUint32(root[10:], s)
	rec := img[20*s:]
	name := "UMD_DATA.BIN;1"
	rec[0] = byte(33 + len(name) + 1)
	binary.LittleEndian.PutUint32(rec[2:], 22)
	rec[32] = byte(len(name))
	copy(rec[33:], name)
	copy(img[22*s:], gameID+"|0000000000000001|0001|G")
	copy(img[23*s:], marker)
	return img
}

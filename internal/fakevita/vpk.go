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

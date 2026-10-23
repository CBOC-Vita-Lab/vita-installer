package galaxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRealDisc cooks a real extraction and compares it with the reference
// cooker's hashes. GALAXY_SRC=<extracted disc> GALAXY_CATALOG=<json> GALAXY_OUT=<scratch>.
func TestRealDisc(t *testing.T) {
	src, catPath, out := os.Getenv("GALAXY_SRC"), os.Getenv("GALAXY_CATALOG"), os.Getenv("GALAXY_OUT")
	if src == "" {
		t.Skip("set GALAXY_SRC to run")
	}
	os.RemoveAll(out)
	defer os.RemoveAll(out)
	start := time.Now()
	if err := Cook(src, out, Options{Movies: os.Getenv("GALAXY_NO_MOVIES") == ""}); err != nil {
		t.Fatal(err)
	}
	t.Logf("cooked in %s", time.Since(start))
	var cat struct {
		Files []struct {
			Path   string
			Bytes  int64
			Sha256 string
		}
	}
	b, _ := os.ReadFile(catPath)
	json.Unmarshal(b, &cat)
	bad, ok := 0, 0
	for _, f := range cat.Files {
		if os.Getenv("GALAXY_NO_MOVIES") != "" && filepath.Ext(f.Path) == ".thp" {
			continue
		}
		fh, err := os.Open(filepath.Join(out, filepath.FromSlash(f.Path)))
		if err != nil {
			bad++
			if bad < 40 {
				t.Errorf("missing %s", f.Path)
			}
			continue
		}
		h := sha256.New()
		n, _ := io.Copy(h, fh)
		fh.Close()
		if n != f.Bytes || hex.EncodeToString(h.Sum(nil)) != f.Sha256 {
			bad++
			if bad < 40 {
				t.Errorf("differs %s (%d vs %d bytes)", f.Path, n, f.Bytes)
			}
			continue
		}
		ok++
	}
	t.Logf("%d match, %d differ", ok, bad)
}

func TestRealTHP(t *testing.T) {
	src, dst := os.Getenv("THP_SRC"), os.Getenv("THP_DST")
	if src == "" {
		t.Skip("set THP_SRC")
	}
	if err := cookTHP(src, dst); err != nil {
		t.Fatal(err)
	}
	if q := os.Getenv("THP_Q"); q != "" {
		start := time.Now()
		var stats MovieStats
		var err error
		if stats, err = ReencodeTHP(dst, dst+".q", 55, 0, nil); err != nil {
			t.Fatal(err)
		}
		t.Logf("re-encoded in %s: %+v", time.Since(start), stats)
	}
}

// TestRealTHPQuality decodes the original and re-encoded movie and reports PSNR.
func TestRealTHPQuality(t *testing.T) {
	a, b := os.Getenv("THP_A"), os.Getenv("THP_B")
	if a == "" {
		t.Skip("set THP_A and THP_B")
	}
	fa, _ := readFrames(t, a)
	fb, _ := readFrames(t, b)
	if len(fa) != len(fb) {
		t.Fatalf("frames %d vs %d", len(fa), len(fb))
	}
	var sum, worst float64
	for i := range fa {
		if i%10 != 0 {
			continue
		}
		pa, err := decodeTHPFrame(fa[i])
		if err != nil {
			t.Fatal(err)
		}
		pb, err := decodeTHPFrame(fb[i])
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		var se float64
		for y := 0; y < pa.h; y++ {
			for x := 0; x < pa.w; x++ {
				d := float64(pa.y[y*pa.yStride+x]) - float64(pb.y[y*pb.yStride+x])
				se += d * d
			}
		}
		mse := se / float64(pa.w*pa.h)
		sum += mse
		if mse > worst {
			worst = mse
		}
	}
	n := float64((len(fa) + 9) / 10)
	t.Logf("frames %d, mean luma MSE %.2f (PSNR %.1f dB), worst %.2f", len(fa), sum/n, 10*math.Log10(255*255/(sum/n)), worst)
}

func readFrames(t *testing.T, path string) ([][]byte, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	n, size, comp, off := int(le.Uint32(data[0x14:])), int(le.Uint32(data[0x18:])), int(le.Uint32(data[0x20:])), int(le.Uint32(data[0x28:]))
	nc := int(le.Uint32(data[comp:]))
	var out [][]byte
	for i := 0; i < n; i++ {
		f := data[off : off+size]
		v := int(le.Uint32(f[8:]))
		out = append(out, f[8+4*nc:8+4*nc+v])
		off += size
		size = int(le.Uint32(f[0:]))
	}
	return out, len(data)
}

// TestRealPrepare runs the whole Galaxy preparation. GALAXY_PREP=<extracted disc> GALAXY_WORK=<scratch>.
func TestRealPrepare(t *testing.T) {
	src, work := os.Getenv("GALAXY_PREP"), os.Getenv("GALAXY_WORK")
	if src == "" {
		t.Skip("set GALAXY_PREP")
	}
	start := time.Now()
	last := ""
	out, err := Prepare(src, work, func(text string, d, tot int64) {
		if text != last {
			t.Logf("%6.1fs %s", time.Since(start).Seconds(), text)
			last = text
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	var size int64
	filepath.Walk(out, func(p string, i os.FileInfo, err error) error {
		if err == nil && !i.IsDir() {
			n++
			size += i.Size()
		}
		return nil
	})
	t.Logf("prepared %d files, %d bytes in %s", n, size, time.Since(start))
	movies, _ := filepath.Glob(filepath.Join(out, "files", "MovieData", "*.thp"))
	for _, m := range movies {
		st, _ := os.Stat(m)
		t.Logf("%s %d", filepath.Base(m), st.Size())
	}
}

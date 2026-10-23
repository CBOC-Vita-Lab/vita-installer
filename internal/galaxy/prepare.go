package galaxy

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/catalog"
)

//go:embed data/input-rmge01.json.gz
var inputCatalog []byte

//go:embed data/cooked-rmge01.json.gz
var cookedCatalog []byte

// Progress reports a phase and how far it is.
type Progress func(text string, done, total int64)

// MovieQuality is the JPEG quality the movies are re-encoded at (the port's
// tested setting).
const MovieQuality = 55

// Prepare turns an extracted Super Mario Galaxy (USA, RMGE01) disc into the
// port's data, in work/game. Every input and output file is checked against
// the reference cooker's results, then the movies are re-encoded smaller.
func Prepare(src, work string, progress Progress) (string, error) {
	if progress == nil {
		progress = func(string, int64, int64) {}
	}
	if _, err := os.Stat(filepath.Join(src, "sys", "fst.bin")); err != nil {
		src = filepath.Join(src, "DATA")
	}
	in, err := catalog.Parse(inputCatalog)
	if err != nil {
		return "", err
	}
	progress("Checking your game files", 0, 0)
	if err := in.CheckSizes(src, nil); err != nil {
		return "", fmt.Errorf("your extracted game is incomplete or a different version: %v. Extract the whole disc again", err)
	}
	out := filepath.Join(work, "game")
	os.RemoveAll(out)
	if err := Cook(src, out, Options{Movies: true, Progress: func(d, t int) {
		progress("Converting the game for the Vita", int64(d), int64(t))
	}}); err != nil {
		return "", err
	}
	want, err := catalog.Parse(cookedCatalog)
	if err != nil {
		return "", err
	}
	if err := want.Verify(out, nil, func(d, t int64) { progress("Checking the converted files", d, t) }); err != nil {
		return "", fmt.Errorf("the converted game does not match the expected result (%v). Extract the disc again", err)
	}
	movies, _ := filepath.Glob(filepath.Join(out, "files", "MovieData", "*.thp"))
	for i, m := range movies {
		label := fmt.Sprintf("Optimising movie %d of %d", i+1, len(movies))
		if _, err := ReencodeTHP(m, m+".q", MovieQuality, 0, func(d, t int) { progress(label, int64(d), int64(t)) }); err != nil {
			return "", fmt.Errorf("%s: %v", filepath.Base(m), err)
		}
		if err := os.Rename(m+".q", m); err != nil {
			return "", err
		}
	}
	return out, nil
}

// IsGalaxyStep reports whether a step name selects this preparation.
func IsGalaxyStep(name string) bool {
	return strings.EqualFold(name, "galaxy") || strings.EqualFold(name, "galaxy-cook")
}

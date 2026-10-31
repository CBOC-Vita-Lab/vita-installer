// Command fakebuilder stands in for a game's build program in tests: it reads
// the "disc", prints progress lines like a real builder and writes a tiny VPK.
// What it does depends on a marker inside the disc file: MODE:BAD fails with a
// message, MODE:SLOW never finishes, MODE:WRONG builds some other app.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

func main() {
	var out, in string
	progress := false
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--progress":
			progress = true
		case "--out":
			i++
			out = args[i]
		default:
			in = args[i]
		}
	}
	say := func(format string, a ...interface{}) {
		if progress {
			fmt.Printf(format+"\n", a...)
		}
	}
	// Leave traces next to the program (the working folder is the kit folder).
	if f, err := os.OpenFile("ran.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(f, in)
		f.Close()
	}
	os.WriteFile("tmpdir.txt", []byte(os.TempDir()), 0o644)

	disc, err := os.ReadFile(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot read the disc.")
		os.Exit(1)
	}
	switch {
	case bytes.Contains(disc, []byte("MODE:BAD")):
		say("@step Reading your disc")
		fmt.Fprintln(os.Stderr, "This is not the US version of Infected (ULUS-10054, 1.02).")
		os.Exit(1)
	case bytes.Contains(disc, []byte("MODE:SLOW")):
		say("@step Compiling (this is the long part)")
		time.Sleep(time.Minute)
	}
	say("@step Reading your disc")
	fmt.Println("this line is not a progress line")
	say("@progress nonsense here")
	say("@progress 5 0")
	say("@step Compiling (this is the long part)")
	for i := 1; i <= 4; i++ {
		say("@progress %d 4", i)
	}
	say("@progress 9 4") // more than the total: shown as done
	say("@step Packaging")
	title := "INFC00001"
	if bytes.Contains(disc, []byte("MODE:WRONG")) {
		title = "WRNG00001"
	}
	vpk := filepath.Join(out, "infected.vpk")
	if err := fakevita.MakeVPK(vpk, title, "Infected", map[string][]byte{"module/infected_core3.skprx": []byte("kernel plugin")}); err != nil {
		fmt.Fprintln(os.Stderr, "Cannot write the app.")
		os.Exit(1)
	}
	say("@done %s", vpk)
}

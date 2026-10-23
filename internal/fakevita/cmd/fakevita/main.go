// Command fakevita runs a pretend Vita for trying the installer without a console.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/CBOC-Vita-Lab/vita-installer/internal/fakevita"
)

func main() {
	root := flag.String("root", "fake-vita", "folder that holds the fake ux0 and ur0")
	prom := flag.Bool("vitashell", true, "accept VitaShell's PROM install command")
	companion := flag.Bool("companion", false, "also run a Vita Companion command port")
	speed := flag.Float64("speed", 0, "limit each transfer to this many MB/s (0 = unlimited)")
	flag.Parse()
	v, err := fakevita.Start(*root, *prom, *companion)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	v.Speed = int64(*speed * 1024 * 1024)
	fmt.Printf("fake Vita FTP on 127.0.0.1:%d", v.Port())
	if *companion {
		fmt.Printf(", companion on %d", v.CompanionPort())
	}
	fmt.Println()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}

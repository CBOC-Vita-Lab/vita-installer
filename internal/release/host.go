package release

import (
	"runtime"
	"sort"
	"strings"
)

// Host describes the computer the installer runs on, as far as building a
// game cares.
type Host struct {
	OS       string // Go's name: windows, darwin, linux
	Arch     string // Go's name: amd64, 386, arm64…
	MacMajor int    // macOS major version (14 for Sonoma), 0 when unknown or not a Mac
}

// CurrentHost reports this computer. Tests replace it.
var CurrentHost = func() Host {
	return Host{OS: runtime.GOOS, Arch: runtime.GOARCH, MacMajor: macMajor()}
}

// Blocked says why the game can't be built on h, or "" when it can. name is
// the game's name. It never downloads anything: it only reads the release's
// list of kits and what the computer is.
func (b *Build) Blocked(name string, h Host) string {
	if _, _, ok := b.KitFor(h.OS, h.Arch); !ok {
		return b.cannotBuild(name)
	}
	if h.OS == "darwin" && b.Require != nil && b.Require.MacOS > 0 && h.MacMajor > 0 && h.MacMajor < b.Require.MacOS {
		return b.cannotBuild(name)
	}
	return ""
}

// cannotBuild is the message for a computer the game can't be built on.
func (b *Build) cannotBuild(name string) string {
	if name == "" {
		name = "This game"
	}
	return name + " can't be built on this computer yet. It can be built on " + b.where() + "."
}

// where lists the computers that have a kit, in words.
func (b *Build) where() string {
	order := map[string]int{"Windows": 0, "macOS": 1, "Linux": 2}
	type opt struct {
		group, text string
	}
	seen := map[string]bool{}
	var opts []opt
	add := func(group, text string) {
		if !seen[text] {
			seen[text] = true
			opts = append(opts, opt{group, text})
		}
	}
	for k := range b.Kit {
		os, arch := k, ""
		if i := strings.IndexByte(k, '-'); i >= 0 {
			os, arch = k[:i], k[i+1:]
		}
		switch os {
		case "windows":
			switch arch {
			case "amd64":
				add("Windows", "Windows (64-bit Intel/AMD)")
			case "386":
				add("Windows", "Windows (32-bit)")
			case "arm64":
				add("Windows", "Windows (ARM)")
			default:
				add("Windows", "Windows")
			}
		case "darwin":
			if b.Require != nil && b.Require.MacOS > 0 {
				add("macOS", "macOS "+itoa(b.Require.MacOS)+" or newer")
			} else {
				add("macOS", "macOS")
			}
		case "linux":
			switch arch {
			case "amd64":
				add("Linux", "Linux (64-bit Intel/AMD)")
			case "arm64":
				add("Linux", "Linux (64-bit ARM)")
			default:
				add("Linux", "Linux")
			}
		}
	}
	sort.SliceStable(opts, func(i, j int) bool {
		if order[opts[i].group] != order[opts[j].group] {
			return order[opts[i].group] < order[opts[j].group]
		}
		return opts[i].text < opts[j].text
	})
	var t []string
	for _, o := range opts {
		t = append(t, o.text)
	}
	switch len(t) {
	case 0:
		return "no computer yet"
	case 1:
		return t[0]
	case 2:
		return t[0] + " or " + t[1]
	}
	return strings.Join(t[:len(t)-1], ", ") + ", or " + t[len(t)-1]
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

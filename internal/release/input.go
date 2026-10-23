package release

// Input is a need plus what the player has chosen for it.
type Input struct {
	Need
	Auto    bool   `json:"auto,omitempty"`    // added by the installer, not the release
	Path    string `json:"path,omitempty"`    // chosen file or folder
	State   string `json:"state"`             // "", "ok", "error", "skip"
	Message string `json:"message,omitempty"` // shown under the row
	Bytes   int64  `json:"bytes,omitempty"`   // about how much this adds to the install
}

// Satisfied reports whether the input does not block installation.
func (in *Input) Satisfied() bool {
	return in.State == "ok" || in.State == "skip" || (in.Optional && in.Path == "")
}

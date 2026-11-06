package release

import "testing"

// VitAdhoc 1.3 adds SNES games. A release that asks for them gets a slot for
// .sfc/.smc files, sent to its "snes" destination (ux0:ROMS/SNES if none).
func TestVitAdhocSNESSlot(t *testing.T) {
	for _, c := range []struct{ dest, want string }{
		{``, "ux0:ROMS/SNES"},
		{`,"snes":"ux0:data/GBAdhoc/snes"`, "ux0:data/GBAdhoc/snes"},
	} {
		m, err := fromCBOC([]byte(`{"schemaVersion":1,"titleId":"GBAD00001","displayName":"VitAdhoc","releaseState":"ready",
		 "package":{"path":"VitAdhoc.vpk"},"optionalInputs":[{"id":"roms","kind":"roms"},{"id":"snes","kind":"snes-roms"},{"id":"bios","kind":"gba-bios"}],
		 "destinations":{"data":"ux0:data/GBAdhoc/roms"` + c.dest + `}}`))
		if err != nil {
			t.Fatal(err)
		}
		var snes *Need
		for i := range m.Needs {
			if m.Needs[i].ID == "snes" {
				snes = &m.Needs[i]
			}
		}
		if snes == nil || snes.To != c.want || snes.Type != NeedFiles || !snes.Optional || join(snes.Ext) != ".sfc .smc" {
			t.Fatalf("snes slot %+v", snes)
		}
	}
}

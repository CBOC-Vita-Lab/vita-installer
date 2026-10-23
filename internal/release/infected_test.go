package release

import "testing"

func TestInfectedGetsHDTextureSlots(t *testing.T) {
	m, err := fromCBOC([]byte(`{"schemaVersion":1,"titleId":"INFC00001","displayName":"Infected","releaseState":"ready",
	 "package":{"path":"x.vpk"},"requiredInputs":[{"id":"game","kind":"infected-disc"},{"id":"shader","kind":"shader-compiler"}],
	 "destinations":{"data":"ux0:data/infected"},"protectedPaths":["ux0:data/infected/texpack_2x.bin"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range m.Needs {
		got = append(got, n.ID+"="+n.To)
	}
	want := "game=ux0:data/infected/Infected.iso shader=ur0:data/libshacccg.suprx hd-2x=ux0:data/infected/texpack_2x.bin hd-4x=ux0:data/infected/texpack_4x.bin"
	if s := join(got); s != want {
		t.Fatalf("needs %s", s)
	}
	if !m.Needs[2].Optional || m.Needs[3].MaxBytes == 0 {
		t.Fatal("HD slots must be optional and size-limited")
	}
}

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += " "
		}
		out += x
	}
	return out
}

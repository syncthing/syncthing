package upnp

import (
	"os"
	"testing"
)

func TestGetTechnicolorRootURL(t *testing.T) {
	r, _ := os.Open("testdata/technicolor.xml")
	_, action, err := getServiceURLReader("http://localhost:1234/", r)
	if err != nil {
		t.Fatal(err)
	}
	if action != "urn:schemas-upnp-org:service:WANPPPConnection:1" {
		t.Error("Unexpected action", action)
	}
}

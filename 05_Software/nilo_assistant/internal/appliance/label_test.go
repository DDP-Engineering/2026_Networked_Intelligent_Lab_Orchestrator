package appliance

import (
	"bytes"
	"strings"
	"testing"

	"github.com/liyue201/goqr"
)

func TestRenderLabelSVGIsDeterministicAndPrintable(t *testing.T) {
	label := Label{
		SerialName:        "nilo-p2rjmzdkf1",
		ManagementURL:     "https://nilo-p2rjmzdkf1.local:8443/",
		HardwareAddresses: []string{"AA:BB:CC:DD:EE:FF", "02:00:00:00:00:01", "aa:bb:cc:dd:ee:ff"},
	}
	first, err := RenderLabelSVG(label)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderLabelSVG(label)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("label output depends on arrival order or mutable state")
	}
	text := string(first)
	for _, want := range []string{
		`width="62mm"`, `height="29mm"`, `viewBox="0 0 620 290"`,
		"nilo-p2rjmzdkf1", "02:00:00:00:00:01", "aa:bb:cc:dd:ee:ff",
		"https://nilo-p2rjmzdkf1.local:8443/", `aria-label="NILO appliance label"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("label omitted %q", want)
		}
	}
	if strings.Count(text, "aa:bb:cc:dd:ee:ff") != 1 {
		t.Fatalf("label did not normalize and deduplicate MAC addresses: %s", text)
	}
	code, err := labelQRCode(label)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := goqr.Recognize(code.Image(512))
	if err != nil || len(decoded) != 1 || string(decoded[0].Payload) != label.ManagementURL {
		t.Fatalf("decoded QR = %+v, %v", decoded, err)
	}
}

func TestRenderLabelSVGRejectsUnsafeOrIncompleteData(t *testing.T) {
	tests := []Label{
		{},
		{SerialName: "nilo-bad<script>"},
		{SerialName: "nilo-p2rjmzdkf1", ManagementURL: "https://nilo-p2rjmzdkf1.local:8443/", HardwareAddresses: []string{"not-a-mac"}},
		{SerialName: "nilo-p2rjmzdkf1", ManagementURL: "https://other.local:8443/", HardwareAddresses: []string{"02:00:00:00:00:01"}},
		{SerialName: "nilo-p2rjmzdkf1", ManagementURL: "http://nilo-p2rjmzdkf1.local:8443/", HardwareAddresses: []string{"02:00:00:00:00:01"}},
	}
	for _, label := range tests {
		if output, err := RenderLabelSVG(label); err == nil || len(output) != 0 {
			t.Fatalf("RenderLabelSVG(%+v) = %q, %v", label, output, err)
		}
	}
}

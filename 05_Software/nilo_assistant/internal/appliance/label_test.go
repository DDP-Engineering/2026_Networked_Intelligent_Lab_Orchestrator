package appliance

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"strconv"
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
		`@media screen`, `width: 620px`, `height: 290px`,
		`@media print`, `width: 62mm`, `height: 29mm`,
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
	wantModuleWidth := 270.0/float64(len(code.Bitmap())) + 0.001
	if got := firstDarkModuleWidth(t, first); math.Abs(got-wantModuleWidth) > 0.001 {
		t.Fatalf("QR module width = %.3f, want %.3f for a 27 mm code", got, wantModuleWidth)
	}
	decoded, err := goqr.Recognize(rasterizeLabelSVG(t, first))
	if err != nil || len(decoded) != 1 || string(decoded[0].Payload) != label.ManagementURL {
		t.Fatalf("decoded QR = %+v, %v", decoded, err)
	}
}

func firstDarkModuleWidth(t *testing.T, document []byte) float64 {
	t.Helper()
	decoder := xml.NewDecoder(bytes.NewReader(document))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "rect" {
			continue
		}
		values := map[string]string{}
		for _, attribute := range start.Attr {
			values[attribute.Name.Local] = attribute.Value
		}
		if values["fill"] == "black" {
			return parseSVGNumber(t, values["width"])
		}
	}
	t.Fatal("label contains no dark QR module")
	return 0
}

func rasterizeLabelSVG(t *testing.T, document []byte) image.Image {
	t.Helper()
	const scale = 4
	canvas := image.NewGray(image.Rect(0, 0, 620*scale, 290*scale))
	decoder := xml.NewDecoder(bytes.NewReader(document))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "rect" {
			continue
		}
		values := map[string]string{}
		for _, attribute := range start.Attr {
			values[attribute.Name.Local] = attribute.Value
		}
		x := parseSVGNumber(t, values["x"])
		y := parseSVGNumber(t, values["y"])
		width := parseSVGNumber(t, values["width"])
		height := parseSVGNumber(t, values["height"])
		fill := color.White
		if values["fill"] == "black" {
			fill = color.Black
		}
		bounds := image.Rect(
			int(math.Round(x*scale)), int(math.Round(y*scale)),
			int(math.Round((x+width)*scale)), int(math.Round((y+height)*scale)),
		)
		draw.Draw(canvas, bounds, &image.Uniform{C: fill}, image.Point{}, draw.Src)
	}
	return canvas
}

func parseSVGNumber(t *testing.T, value string) float64 {
	t.Helper()
	if value == "" {
		return 0
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		t.Fatalf("parse SVG number %q: %v", value, err)
	}
	return number
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

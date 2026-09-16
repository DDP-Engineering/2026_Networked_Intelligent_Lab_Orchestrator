package appliance

import (
	"errors"
	"fmt"
	"html"
	"net"
	"sort"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

const permanentNameAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// Label contains the stable public fields printed on an appliance enclosure.
type Label struct {
	SerialName        string
	HardwareAddresses []string
}

// RenderLabelSVG builds the shared fixed-size label artifact. It derives the
// URL from the validated permanent name so callers cannot substitute an
// untrusted destination.
func RenderLabelSVG(label Label) ([]byte, error) {
	if !validPermanentName(label.SerialName) {
		return nil, errors.New("valid permanent appliance name is required")
	}
	addresses, err := normalizedHardwareAddresses(label.HardwareAddresses)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("at least one hardware address is required")
	}
	managementURL := "https://" + label.SerialName + ".local/"
	code, err := qrcode.New(managementURL, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("encode label QR: %w", err)
	}
	matrix := code.Bitmap()
	if len(matrix) == 0 {
		return nil, errors.New("QR encoder returned an empty matrix")
	}

	var output strings.Builder
	output.Grow(16 << 10)
	output.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	output.WriteString("\n")
	output.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="62mm" height="29mm" viewBox="0 0 620 290" role="img" aria-label="NILO appliance label">`)
	output.WriteString("\n<rect width=\"620\" height=\"290\" fill=\"white\"/>")
	output.WriteString("\n<desc>")
	output.WriteString(html.EscapeString(managementURL))
	output.WriteString("</desc>")
	module := 250.0 / float64(len(matrix))
	for row, cells := range matrix {
		for column, dark := range cells {
			if !dark {
				continue
			}
			fmt.Fprintf(&output, "\n<rect x=\"%.3f\" y=\"%.3f\" width=\"%.3f\" height=\"%.3f\" fill=\"black\"/>",
				10+float64(column)*module, 20+float64(row)*module, module+0.001, module+0.001)
		}
	}
	fmt.Fprintf(&output, "\n<text x=\"280\" y=\"78\" font-family=\"sans-serif\" font-size=\"30\" font-weight=\"700\">%s</text>", html.EscapeString(label.SerialName))
	output.WriteString("\n<text x=\"280\" y=\"112\" font-family=\"sans-serif\" font-size=\"17\">Hardware addresses</text>")
	for index, address := range addresses {
		fmt.Fprintf(&output, "\n<text x=\"280\" y=\"%d\" font-family=\"monospace\" font-size=\"18\">%s</text>", 140+index*25, address)
	}
	output.WriteString("\n<text x=\"280\" y=\"260\" font-family=\"sans-serif\" font-size=\"14\">Scan to open NILO</text>")
	output.WriteString("\n</svg>\n")
	return []byte(output.String()), nil
}

func validPermanentName(value string) bool {
	if len(value) != 15 || !strings.HasPrefix(value, "nilo-") {
		return false
	}
	for _, character := range value[5:] {
		if !strings.ContainsRune(permanentNameAlphabet, character) {
			return false
		}
	}
	return true
}

func normalizedHardwareAddresses(values []string) ([]string, error) {
	unique := make(map[string]bool, len(values))
	for _, value := range values {
		address, err := net.ParseMAC(value)
		if err != nil || len(address) != 6 {
			return nil, fmt.Errorf("invalid hardware address %q", value)
		}
		unique[address.String()] = true
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

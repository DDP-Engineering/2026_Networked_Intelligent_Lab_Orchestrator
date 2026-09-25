package httpapi

import (
	"testing"

	"nilo.local/nilo/internal/domain"
)

func TestArtifactPresentationUsesVerifiedMediaType(t *testing.T) {
	for _, test := range []struct {
		mediaType       string
		hints           []byte
		wantType        string
		wantDisposition string
	}{
		{"application/octet-stream", []byte(`{"filename":"capture.sr"}`), "application/octet-stream", `attachment; filename="capture.sr"`},
		{"image/jpeg", []byte(`{"filename":"snapshot.jpg"}`), "image/jpeg", `inline; filename="snapshot.jpg"`},
		{"image/png", []byte(`{"filename":"snapshot.png"}`), "image/png", `inline; filename="snapshot.png"`},
		{"image/jpeg\r\nX-Injected: yes", []byte(`{"filename":"evil.txt"}`), "application/octet-stream", `attachment; filename="artifact.bin"`},
	} {
		mediaType, disposition := artifactPresentation(domain.Artifact{MediaType: test.mediaType, PresentationHints: test.hints})
		if mediaType != test.wantType || disposition != test.wantDisposition {
			t.Fatalf("presentation = %q, %q; want %q, %q", mediaType, disposition, test.wantType, test.wantDisposition)
		}
	}
}

package bench

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

func TestCameraPreviewBoundsDecodedImageAndOutput(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 1600, 900))
	frame.Set(800, 450, color.RGBA{R: 255, A: 255})
	var original bytes.Buffer
	if err := jpeg.Encode(&original, frame, nil); err != nil {
		t.Fatal(err)
	}
	preview, err := boundedImagePreview(bytes.NewReader(original.Bytes()), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	configuration, _, err := image.DecodeConfig(bytes.NewReader(preview))
	if err != nil || configuration.Width != 1280 || configuration.Height != 720 || len(preview) > 2<<20 {
		t.Fatalf("bounded preview = %+v, %d bytes, %v", configuration, len(preview), err)
	}
	if _, err := boundedImagePreview(strings.NewReader("not an image"), "image/jpeg"); err != ErrInvalid {
		t.Fatalf("invalid image = %v", err)
	}
	if _, err := boundedImagePreview(bytes.NewReader(original.Bytes()), "text/html"); err != ErrInvalid {
		t.Fatalf("active media type = %v", err)
	}
}

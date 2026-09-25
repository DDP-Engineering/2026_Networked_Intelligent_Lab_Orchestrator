package bench

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"

	"nilo.local/nilo/internal/identity"
)

const previewMaxPixels = 2_000_000
const previewMaxDimension = 1280

// OpenArtifactPreview returns a bounded display copy of an owned camera image.
// The retained artifact remains untouched and available through OpenArtifact.
func (manager *Manager) OpenArtifactPreview(ctx context.Context, actor identity.Actor, id string) ([]byte, error) {
	job, err := manager.GetJob(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if job.PluginID != "camera" || job.Submission.Operation != "snapshot" {
		return nil, ErrInvalid
	}
	content, metadata, err := manager.OpenArtifact(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	defer content.Close()
	return boundedImagePreview(content, metadata.MediaType)
}

func boundedImagePreview(source io.Reader, mediaType string) ([]byte, error) {
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		return nil, ErrInvalid
	}
	encoded, err := io.ReadAll(io.LimitReader(source, (8<<20)+1))
	if err != nil || len(encoded) > 8<<20 {
		return nil, ErrInvalid
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(encoded))
	if err != nil || configuration.Width < 1 || configuration.Height < 1 ||
		configuration.Width > 4096 || configuration.Height > 4096 ||
		configuration.Width*configuration.Height > previewMaxPixels ||
		(format != "jpeg" && format != "png") ||
		(format == "jpeg" && mediaType != "image/jpeg") ||
		(format == "png" && mediaType != "image/png") {
		return nil, ErrInvalid
	}
	frame, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return nil, ErrInvalid
	}
	width, height := configuration.Width, configuration.Height
	if width > previewMaxDimension || height > previewMaxDimension {
		if width >= height {
			height = max(1, height*previewMaxDimension/width)
			width = previewMaxDimension
		} else {
			width = max(1, width*previewMaxDimension/height)
			height = previewMaxDimension
		}
	}
	output := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			original := color.RGBAModel.Convert(frame.At(x*configuration.Width/width, y*configuration.Height/height))
			output.Set(x, y, original)
		}
	}
	var preview bytes.Buffer
	if err := jpeg.Encode(&preview, output, &jpeg.Options{Quality: 75}); err != nil || preview.Len() > 2<<20 {
		return nil, ErrUnavailable
	}
	return preview.Bytes(), nil
}

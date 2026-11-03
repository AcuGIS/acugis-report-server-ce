package contentresource

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestInspectImagePNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	info, err := InspectImage(buf.Bytes(), "logo.PNG")
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "png" || info.Extension != ".png" || info.MimeType != "image/png" {
		t.Fatalf("got %+v", info)
	}
	if info.Width != 32 || info.Height != 16 {
		t.Fatalf("dims %dx%d", info.Width, info.Height)
	}
	if info.HasAlpha == nil {
		t.Fatal("expected has_alpha")
	}
}

func TestInspectImageRejectsGarbage(t *testing.T) {
	_, err := InspectImage([]byte("not an image"), "x.bin")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInspectImageSVG(t *testing.T) {
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="100" height="50" viewBox="0 0 100 50"><rect width="100" height="50" fill="red"/></svg>`)
	info, err := InspectImage(svg, "icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "svg" || info.MimeType != "image/svg+xml" {
		t.Fatalf("got %+v", info)
	}
	if info.Width != 100 || info.Height != 50 {
		t.Fatalf("dims %dx%d", info.Width, info.Height)
	}
	if info.HasAlpha == nil || !*info.HasAlpha {
		t.Fatal("svg should report alpha")
	}
}

func TestGenerateThumbnailPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	thumb, err := GenerateThumbnailPNG(buf.Bytes(), 128)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 128 || cfg.Height != 64 {
		t.Fatalf("thumb size %dx%d", cfg.Width, cfg.Height)
	}
}

func TestTypeImageHelpers(t *testing.T) {
	if !IsImageType("Image") || SupportsPaste(TypeImage) {
		t.Fatal("image type helpers")
	}
	if TypeLabel(TypeImage) != "Image" {
		t.Fatal(TypeLabel(TypeImage))
	}
	if !ExtensionAllowedForImage(".webp") {
		t.Fatal("webp should be allowed")
	}
}

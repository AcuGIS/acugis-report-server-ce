package contentresource

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/image/webp"
)

// Image resource type and related errors.
const (
	TypeImage = "image"
)

var (
	ErrInvalidImage = errors.New("invalid image file")
	ErrBadImageType = errors.New("unsupported image format")
)

// ImageFormat describes a supported image format. Additional formats can be
// registered without changing the Resource architecture.
type ImageFormat struct {
	Name       string   // e.g. "png"
	Extensions []string // with leading dot, lowercase
	MIMETypes  []string
}

// SupportedImageFormats is the registry of image formats accepted on upload.
// Append entries here to extend support later (logos, icons, etc. stay separate types).
var SupportedImageFormats = []ImageFormat{
	{Name: "png", Extensions: []string{".png"}, MIMETypes: []string{"image/png"}},
	{Name: "jpeg", Extensions: []string{".jpg", ".jpeg"}, MIMETypes: []string{"image/jpeg"}},
	{Name: "gif", Extensions: []string{".gif"}, MIMETypes: []string{"image/gif"}},
	{Name: "svg", Extensions: []string{".svg"}, MIMETypes: []string{"image/svg+xml"}},
	{Name: "webp", Extensions: []string{".webp"}, MIMETypes: []string{"image/webp"}},
}

// ImageInfo is extracted metadata for an image resource.
type ImageInfo struct {
	Format    string
	Extension string
	MimeType  string
	Width     int
	Height    int
	HasAlpha  *bool
}

// IsImageType reports whether resource_type is the Image resource type.
func IsImageType(resourceType string) bool {
	return strings.EqualFold(strings.TrimSpace(resourceType), TypeImage)
}

// ImageFormatNames returns supported format names for docs/UI.
func ImageFormatNames() []string {
	out := make([]string, 0, len(SupportedImageFormats))
	for _, f := range SupportedImageFormats {
		out = append(out, f.Name)
	}
	return out
}

// ExtensionAllowedForImage reports whether ext is a known image extension.
func ExtensionAllowedForImage(ext string) bool {
	ext = NormalizeExtension(ext)
	for _, f := range SupportedImageFormats {
		for _, e := range f.Extensions {
			if e == ext {
				return true
			}
		}
	}
	return false
}

// InspectImage validates image bytes and extracts metadata.
// filenameHint is used only as a secondary signal for extension preference.
func InspectImage(data []byte, filenameHint string) (*ImageInfo, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty file", ErrInvalidImage)
	}
	// Soft size guard (32 MiB). Larger assets can be enabled later per format.
	const maxImageBytes = 32 << 20
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("%w: file exceeds 32 MiB limit", ErrInvalidImage)
	}

	if info, err := inspectSVG(data); err == nil {
		return preferHintExtension(info, filenameHint), nil
	} else if !errors.Is(err, ErrBadImageType) {
		return nil, err
	}

	if info, err := inspectWEBP(data); err == nil {
		return preferHintExtension(info, filenameHint), nil
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	info, err := imageInfoFromDecoded(format, cfg, data, filenameHint)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func preferHintExtension(info *ImageInfo, filenameHint string) *ImageInfo {
	if info == nil {
		return nil
	}
	hint := ExtensionFromFilename(filenameHint)
	if hint == "" {
		return info
	}
	for _, f := range SupportedImageFormats {
		if f.Name != info.Format {
			continue
		}
		for _, e := range f.Extensions {
			if e == hint {
				info.Extension = hint
				return info
			}
		}
	}
	return info
}

func imageInfoFromDecoded(format string, cfg image.Config, data []byte, filenameHint string) (*ImageInfo, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "jpeg":
		format = "jpeg"
	case "jpg":
		format = "jpeg"
	case "png", "gif":
		// ok
	default:
		return nil, fmt.Errorf("%w: %s", ErrBadImageType, format)
	}

	spec := formatSpec(format)
	if spec == nil {
		return nil, fmt.Errorf("%w: %s", ErrBadImageType, format)
	}
	ext := spec.Extensions[0]
	hint := ExtensionFromFilename(filenameHint)
	for _, e := range spec.Extensions {
		if e == hint {
			ext = hint
			break
		}
	}
	info := &ImageInfo{
		Format:    format,
		Extension: ext,
		MimeType:  spec.MIMETypes[0],
		Width:     cfg.Width,
		Height:    cfg.Height,
	}
	if format == "png" || format == "gif" {
		alpha := imageHasAlpha(data, format)
		info.HasAlpha = &alpha
	} else if format == "jpeg" {
		alpha := false
		info.HasAlpha = &alpha
	}
	return info, nil
}

func formatSpec(name string) *ImageFormat {
	name = strings.ToLower(strings.TrimSpace(name))
	for i := range SupportedImageFormats {
		if SupportedImageFormats[i].Name == name {
			return &SupportedImageFormats[i]
		}
	}
	return nil
}

func inspectWEBP(data []byte) (*ImageInfo, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, ErrBadImageType
	}
	cfg, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: webp: %v", ErrInvalidImage, err)
	}
	spec := formatSpec("webp")
	alpha := webpLikelyHasAlpha(data)
	return &ImageInfo{
		Format:    "webp",
		Extension: ".webp",
		MimeType:  spec.MIMETypes[0],
		Width:     cfg.Width,
		Height:    cfg.Height,
		HasAlpha:  &alpha,
	}, nil
}

func webpLikelyHasAlpha(data []byte) bool {
	// VP8X feature flags or ALPH/VP8L chunks indicate alpha.
	if len(data) >= 20 && string(data[12:16]) == "VP8X" {
		flags := data[20]
		return flags&0x10 != 0
	}
	s := string(data)
	return strings.Contains(s, "ALPH") || strings.Contains(s, "VP8L")
}

func imageHasAlpha(data []byte, format string) bool {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return format == "png" || format == "gif"
	}
	b := img.Bounds()
	// Sample up to a limited number of pixels for large images.
	stepX := 1
	stepY := 1
	if b.Dx() > 64 {
		stepX = b.Dx() / 64
	}
	if b.Dy() > 64 {
		stepY = b.Dy() / 64
	}
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xffff {
				return true
			}
		}
	}
	return false
}

type svgRoot struct {
	XMLName xml.Name
	Width   string `xml:"width,attr"`
	Height  string `xml:"height,attr"`
	ViewBox string `xml:"viewBox,attr"`
}

func inspectSVG(data []byte) (*ImageInfo, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, ErrBadImageType
	}
	lower := bytes.ToLower(trimmed)
	looksSVG := bytes.Contains(lower, []byte("<svg")) ||
		(bytes.HasPrefix(lower, []byte("<?xml")) && bytes.Contains(lower, []byte("<svg")))
	if !looksSVG {
		return nil, ErrBadImageType
	}

	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	dec.Strict = false
	var root svgRoot
	found := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if strings.EqualFold(se.Name.Local, "svg") {
			_ = dec.DecodeElement(&root, &se)
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: svg root not found", ErrInvalidImage)
	}

	w, h := parseSVGSize(root.Width, root.Height, root.ViewBox)
	alpha := true
	spec := formatSpec("svg")
	return &ImageInfo{
		Format:    "svg",
		Extension: ".svg",
		MimeType:  spec.MIMETypes[0],
		Width:     w,
		Height:    h,
		HasAlpha:  &alpha,
	}, nil
}

func parseSVGSize(widthAttr, heightAttr, viewBox string) (w, h int) {
	w = parseSVGLength(widthAttr)
	h = parseSVGLength(heightAttr)
	if w > 0 && h > 0 {
		return w, h
	}
	parts := strings.Fields(strings.ReplaceAll(viewBox, ",", " "))
	if len(parts) == 4 {
		vw, _ := strconv.ParseFloat(parts[2], 64)
		vh, _ := strconv.ParseFloat(parts[3], 64)
		if w <= 0 && vw > 0 {
			w = int(vw + 0.5)
		}
		if h <= 0 && vh > 0 {
			h = int(vh + 0.5)
		}
	}
	return w, h
}

func parseSVGLength(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	i := 0
	for i < len(s) {
		r := rune(s[i])
		if unicode.IsDigit(r) || r == '.' || r == '-' || r == '+' {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return 0
	}
	f, err := strconv.ParseFloat(s[:i], 64)
	if err != nil || f <= 0 {
		return 0
	}
	return int(f + 0.5)
}

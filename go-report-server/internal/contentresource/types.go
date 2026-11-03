// Package contentresource implements first-class folder Resources (keys, certs, files).
package contentresource

import (
	"errors"
	"strings"
	"time"
)

// Resource type constants. New types can be added without schema changes.
const (
	TypePublicKey           = "public_key"
	TypePrivateKey          = "private_key"
	TypeClientCertificate   = "client_certificate"
	TypeCACertificate       = "ca_certificate"
	TypeGenericFile         = "generic_file"
	TypeServiceAccountJSON  = "service_account_json"
)

var (
	ErrUnavailable  = errors.New("content resources unavailable")
	ErrNotFound     = errors.New("resource not found")
	ErrInvalidInput = errors.New("invalid resource input")
	ErrBadType      = errors.New("unsupported resource type")
)

// Resource is a row from jasper_content_resource plus optional folder path.
type Resource struct {
	ID               int64     `json:"id"`
	UUID             string    `json:"uuid"`
	FolderID         int64     `json:"folder_id"`
	DisplayName      string    `json:"display_name"`
	ResourceType     string    `json:"resource_type"`
	OriginalFilename string    `json:"original_filename"`
	Extension        string    `json:"extension"`
	StorageFilename  string    `json:"storage_filename"`
	MimeType         string    `json:"mime_type"`
	SHA256           string    `json:"sha256"`
	SizeBytes        int64     `json:"size_bytes"`
	Width            *int      `json:"width,omitempty"`
	Height           *int      `json:"height,omitempty"`
	HasAlpha         *bool     `json:"has_alpha,omitempty"`
	CreatedBy        *int64    `json:"created_by,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	FolderPath       string    `json:"folder_path,omitempty"`
	// ResourcePath is the JRXML image path for this resource (e.g. "/david/family.png").
	// Built from folder hierarchy (skipping Home) + original_filename.
	ResourcePath string `json:"resource_path,omitempty"`
}

// ReplaceInput replaces binary content while preserving UUID, display name, folder, and type.
type ReplaceInput struct {
	OriginalFilename string
	Extension        string
	MimeType         string
	Data             []byte
}

// CreateInput creates a resource from upload bytes or pasted text.
type CreateInput struct {
	FolderID         int64
	DisplayName      string
	ResourceType     string
	OriginalFilename string
	Extension        string // explicit; include leading dot when present
	MimeType         string
	Data             []byte
	CreatedBy        int64
}

// UpdateInput patches resource metadata (not binary content).
type UpdateInput struct {
	DisplayName  *string
	ResourceType *string
	FolderID     *int64
}

// ListFilter filters list queries for browser and pickers.
type ListFilter struct {
	FolderID     int64
	ResourceType string
	Query        string
	Limit        int
}

// KnownTypes returns the initial supported resource types.
func KnownTypes() []string {
	return []string{
		TypePublicKey,
		TypePrivateKey,
		TypeClientCertificate,
		TypeCACertificate,
		TypeGenericFile,
		TypeServiceAccountJSON,
		TypeImage,
	}
}

// NormalizeType lowercases and validates a resource type.
// Unknown types are accepted when non-empty so future types need no schema change;
// empty or whitespace-only values are invalid.
func NormalizeType(t string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(t))
	if n == "" {
		return "", ErrBadType
	}
	return n, nil
}

// TypeLabel returns a human-readable label for known types.
func TypeLabel(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case TypePublicKey:
		return "Public Key"
	case TypePrivateKey:
		return "Private Key"
	case TypeClientCertificate:
		return "Client Certificate"
	case TypeCACertificate:
		return "CA Certificate"
	case TypeGenericFile:
		return "Generic File"
	case TypeServiceAccountJSON:
		return "Service Account JSON"
	case TypeImage:
		return "Image"
	default:
		return t
	}
}

// DefaultExtensionForType returns the extension (with leading dot) used when pasting text.
func DefaultExtensionForType(resourceType string) string {
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case TypePublicKey:
		return ".pub"
	case TypePrivateKey:
		return ".pem"
	case TypeClientCertificate, TypeCACertificate:
		return ".crt"
	case TypeServiceAccountJSON:
		return ".json"
	case TypeImage:
		return ".png"
	default:
		return ".bin"
	}
}

// DefaultMimeForType returns a default MIME type for a resource type.
func DefaultMimeForType(resourceType string) string {
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case TypePublicKey, TypePrivateKey, TypeClientCertificate, TypeCACertificate:
		return "application/x-pem-file"
	case TypeServiceAccountJSON:
		return "application/json"
	case TypeImage:
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// SupportsPaste reports whether paste-text input is appropriate for the type.
func SupportsPaste(resourceType string) bool {
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case TypePublicKey, TypePrivateKey, TypeClientCertificate, TypeCACertificate, TypeServiceAccountJSON:
		return true
	default:
		return false
	}
}

// StorageFilename builds the relative on-disk name: "{uuid}{extension}".
func StorageFilename(uuid, extension string) string {
	ext := strings.TrimSpace(extension)
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return strings.TrimSpace(uuid) + ext
}

// NormalizeExtension ensures a leading dot when non-empty, and lowercases.
func NormalizeExtension(ext string) string {
	ext = strings.TrimSpace(ext)
	if ext == "" {
		return ""
	}
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	// Cap length to column size.
	if len(ext) > 32 {
		ext = ext[:32]
	}
	return ext
}

// ExtensionFromFilename extracts the extension (with leading dot) from a filename.
// Callers must persist the result; do not re-derive at download time from original_filename alone
// when the stored extension column is available.
func ExtensionFromFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return ""
	}
	base := filename
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "\\"); i >= 0 {
		base = base[i+1:]
	}
	dot := strings.LastIndex(base, ".")
	if dot <= 0 || dot == len(base)-1 {
		return ""
	}
	return NormalizeExtension(base[dot:])
}

package license

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const installationFileName = "installation-id"

// installationIDPattern matches the central license service: lowercase UUID v4.
var installationIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ValidInstallationID reports whether id is a lowercase UUID v4.
func ValidInstallationID(id string) bool {
	return installationIDPattern.MatchString(id)
}

// NewInstallationID returns a random lowercase UUID v4.
// It is not derived from hostname, MAC, IP, or machine-id.
func NewInstallationID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate installation identity: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func installationPath(dir string) string {
	return filepath.Join(dir, installationFileName)
}

// LoadOrCreateInstallationID returns the durable installation UUID.
// A valid file is never overwritten. A malformed file is returned as an error
// and is not replaced.
func LoadOrCreateInstallationID(dir string) (string, error) {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", fmt.Errorf("create license state directory: %w", err)
	}
	path := installationPath(dir)
	raw, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(raw))
		if !ValidInstallationID(id) {
			return "", errIdentityInvalid()
		}
		return id, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	id, err := NewInstallationID()
	if err != nil {
		return "", err
	}
	if err := writeNewFile(path, []byte(id+"\n")); err != nil {
		if os.IsExist(err) {
			return readInstallationID(path)
		}
		return "", err
	}
	return readInstallationID(path)
}

// ReadInstallationID returns the existing Report Server installation UUID.
// It does not create a file and does not replace a malformed one.
func ReadInstallationID(dir string) (string, error) {
	raw, err := os.ReadFile(installationPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return "", errMissing()
		}
		return "", err
	}
	id := strings.TrimSpace(string(raw))
	if !ValidInstallationID(id) {
		return "", errIdentityInvalid()
	}
	return id, nil
}

func readInstallationID(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(raw))
	if !ValidInstallationID(id) {
		return "", errIdentityInvalid()
	}
	return id, nil
}

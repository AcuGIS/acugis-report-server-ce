package license

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dirMode         = 0o750
	fileMode        = 0o640
	licenseFileName = "license.lic"
	// DesignerLicenseFileName is the Report Designer Web ACL1. It is not license.lic.
	DesignerLicenseFileName = "report-designer-web.lic"
	licenseTempGlob         = ".acugis-license-*.tmp"
)

// atomicWriteHook is nil in production. Tests use it to fail a replacement
// before the destination is replaced.
var atomicWriteHook func(step string) error

func licensePath(dir string) string {
	return filepath.Join(dir, licenseFileName)
}

// ReadLicenseToken returns the stored Professional ACL1 without verification.
func ReadLicenseToken(dir string) (string, error) {
	return ReadLicenseFile(dir, licenseFileName)
}

// ReadLicenseFile returns one stored ACL1 without verification.
func ReadLicenseFile(dir, filename string) (string, error) {
	name, err := safeLicenseFilename(filename)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return "", errMissing()
		}
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errInvalid()
	}
	return token, nil
}

// SaveVerifiedLicense writes a Professional token only after local verification succeeds.
// The previous file is left in place until the replacement is complete.
func SaveVerifiedLicense(dir, token string, v *Verifier, installationID string) error {
	return SaveVerifiedProductLicense(dir, licenseFileName, token, v, installationID, ProductReportServerProfessional)
}

// SaveVerifiedProductLicense writes an ACL1 for one product after signature,
// product, installation, and expiry checks succeed. The previous file stays
// in place until the replacement is complete. The activation key is never written.
func SaveVerifiedProductLicense(dir, filename, token string, v *Verifier, installationID, product string) error {
	name, err := safeLicenseFilename(filename)
	if err != nil {
		return err
	}
	product = strings.TrimSpace(product)
	if product == "" {
		product = ProductReportServerProfessional
	}
	lic, err := ParseIdentity(token, v, installationID, product)
	if err != nil {
		return err
	}
	if v == nil || !v.now().Before(lic.ExpiresAt) {
		return errExpired()
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create license state directory: %w", err)
	}
	return writeFileAtomic(filepath.Join(dir, name), []byte(strings.TrimSpace(token)+"\n"))
}

func safeLicenseFilename(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return "", errInvalid()
	}
	return name, nil
}

func writeNewFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".acugis-id-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := writeSyncClose(tmp, body); err != nil {
		return err
	}
	if err := os.Link(tmpName, path); err != nil {
		return err
	}
	return nil
}

func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	cleanupTemps(dir)
	tmp, err := os.CreateTemp(dir, licenseTempGlob)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	removeTmp := true
	defer func() {
		_ = tmp.Close()
		if removeTmp {
			_ = os.Remove(tmpName)
		}
	}()
	if err := writeSyncClose(tmp, body); err != nil {
		return err
	}
	if atomicWriteHook != nil {
		if err := atomicWriteHook("before_rename"); err != nil {
			return err
		}
	}
	if err := replaceFile(tmpName, path); err != nil {
		return err
	}
	removeTmp = false
	_ = syncDir(dir)
	return nil
}

func writeSyncClose(f *os.File, body []byte) error {
	n, err := f.Write(body)
	if err != nil {
		return fmt.Errorf("write license file: %w", err)
	}
	if n != len(body) {
		return fmt.Errorf("write license file: short write")
	}
	if err := f.Chmod(fileMode); err != nil && runtime.GOOS != "windows" {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func replaceFile(tmpName, path string) error {
	if err := os.Rename(tmpName, path); err == nil {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return os.Rename(tmpName, path)
	}
	backup := path + ".replacing"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	err = f.Sync()
	if err != nil && runtime.GOOS == "windows" {
		return nil
	}
	return err
}

func cleanupTemps(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, licenseTempGlob))
	if err != nil {
		return
	}
	for _, name := range matches {
		_ = os.Remove(name)
	}
}

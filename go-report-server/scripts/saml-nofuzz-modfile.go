// Prints an alternate go.mod for production builds.
//
// github.com/crewjam/saml/xmlenc/fuzz.go is a go-fuzz harness with no build
// tag. It embeds a test RSA private key in a package-level variable, so the
// key is linked into every Professional binary that imports SAML. Go refuses
// to -overlay files inside GOMODCACHE, so this copies the module outside the
// cache, replaces that harness with third_party/xmlenc_fuzz_exclude.go (which
// contains no key), and points a temporary go.mod at the copy.
//
// The private key is not an AcuGIS license key and is not used to decrypt
// SAML assertions. License verification uses the Ed25519 public key in
// internal/license.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	stubPath := filepath.Join("third_party", "xmlenc_fuzz_exclude.go")
	stub, err := os.ReadFile(stubPath)
	if err != nil {
		fail(fmt.Errorf("read fuzz exclusion stub: %w", err))
	}
	if strings.Contains(string(stub), "PRIVATE KEY") {
		fail(fmt.Errorf("fuzz exclusion stub must not contain private key material"))
	}

	modDir, required, err := samlModuleDir()
	if err != nil {
		fail(err)
	}
	if modDir == "" {
		if required {
			fail(fmt.Errorf("github.com/crewjam/saml is required but its module directory is unavailable"))
		}
		fmt.Println("")
		return
	}
	srcFuzz := filepath.Join(modDir, "xmlenc", "fuzz.go")
	embeds, err := fuzzEmbedsPrivateKey(srcFuzz)
	if err != nil {
		fail(err)
	}
	if !embeds {
		// Upstream no longer compiles a private key into normal builds.
		fmt.Println("")
		return
	}

	dst, err := os.MkdirTemp("", "crewjam-saml-")
	if err != nil {
		fail(err)
	}
	if err := copyTree(modDir, dst); err != nil {
		fail(err)
	}
	fuzzDst := filepath.Join(dst, "xmlenc", "fuzz.go")
	if err := os.Chmod(fuzzDst, 0o644); err != nil && !os.IsNotExist(err) {
		fail(err)
	}
	if err := os.WriteFile(fuzzDst, stub, 0o644); err != nil {
		fail(err)
	}
	written, err := os.ReadFile(fuzzDst)
	if err != nil {
		fail(err)
	}
	if strings.Contains(string(written), "PRIVATE KEY") {
		fail(fmt.Errorf("patched xmlenc/fuzz.go still contains private key material"))
	}

	modfile, err := writeModFile(dst)
	if err != nil {
		fail(err)
	}
	fmt.Println(modfile)
}

func samlModuleDir() (dir string, required bool, err error) {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "", false, err
	}
	if strings.Contains(string(raw), "replace github.com/crewjam/saml ") {
		return "", false, fmt.Errorf("go.mod already replaces github.com/crewjam/saml; refusing to stack a second copy")
	}
	required = strings.Contains(string(raw), "github.com/crewjam/saml ")
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/crewjam/saml")
	out, listErr := cmd.Output()
	if listErr != nil {
		if required {
			return "", true, fmt.Errorf("locate github.com/crewjam/saml: %w", listErr)
		}
		return "", false, nil
	}
	dir = strings.TrimSpace(string(out))
	if dir == "" && required {
		return "", true, fmt.Errorf("github.com/crewjam/saml has no module directory")
	}
	return dir, required, nil
}

func fuzzEmbedsPrivateKey(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !strings.Contains(string(b), "PRIVATE KEY") {
		return false, nil
	}
	head := string(b)
	if len(head) > 240 {
		head = head[:240]
	}
	if strings.Contains(head, "gofuzz") && (strings.Contains(head, "//go:build") || strings.Contains(head, "+build")) {
		return false, nil
	}
	return true, nil
}

func writeModFile(samlDir string) (string, error) {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "saml-modfile-")
	if err != nil {
		return "", err
	}
	modPath := filepath.Join(dir, "samlbuild.mod")
	sumSrc, err := os.ReadFile("go.sum")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "samlbuild.sum"), sumSrc, 0o644); err != nil {
		return "", err
	}
	repl := filepath.ToSlash(samlDir)
	if strings.ContainsAny(repl, " \t") {
		repl = `"` + repl + `"`
	}
	var b strings.Builder
	b.Write(raw)
	if !strings.HasSuffix(string(raw), "\n") {
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\nreplace github.com/crewjam/saml => %s\n", repl)
	if err := os.WriteFile(modPath, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return modPath, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "saml-nofuzz-modfile: %v\n", err)
	os.Exit(1)
}

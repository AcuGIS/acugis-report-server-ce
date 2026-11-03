// Sync repository-root openapi/*.yaml into internal/openapi/spec/ for go:embed.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	srcDir := filepath.Join(root, "openapi")
	dstDir := filepath.Join(root, "internal", "openapi", "spec")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		fail(err)
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		fail(err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(dstDir, e.Name())
		if err := copyFile(src, dst); err != nil {
			fail(err)
		}
		n++
	}
	fmt.Printf("synced %d yaml files to %s\n", n, dstDir)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

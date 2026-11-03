package packaging

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageReleaseVerifiesMapRendererBeforeCopy(t *testing.T) {
	src := string(mustRead(t, filepath.Join(repoRoot(t), "scripts", "package-release.sh")))
	if !strings.Contains(src, "verify-map-renderer-api.sh") {
		t.Fatal("package-release.sh does not run the MapRenderer API check")
	}
	start := strings.Index(src, "stage_tree() {")
	end := strings.Index(src, "write_manifest() {")
	if start < 0 || end < start {
		t.Fatal("stage_tree function not found")
	}
	body := src[start:end]
	verifyAt := strings.Index(body, `verify_packaged_map_renderer "$WAR"`)
	copyAt := strings.Index(body, `cp -a "$WAR"`)
	if verifyAt < 0 || copyAt < 0 || verifyAt > copyAt {
		t.Fatal("stage_tree must verify the WAR MapRenderer API before copying it")
	}
	mainAt := strings.Index(src, "\nmain() {")
	if mainAt < 0 {
		t.Fatal("main() not found")
	}
	mainBody := src[mainAt:]
	mainVerify := strings.Index(mainBody, "verify_packaged_map_renderer")
	mainStage := strings.Index(mainBody, "stage_tree")
	if mainVerify < 0 || mainStage < 0 || mainVerify > mainStage {
		t.Fatal("main() must verify MapRenderer before staging")
	}
}

func TestWarExposesNineArgumentRender(t *testing.T) {
	root := repoRoot(t)
	war := filepath.Join(root, "stand-alone-tomcat-10", "dist", "acugis-report-server.war")
	if _, err := os.Stat(war); err != nil {
		t.Skip("report server WAR is produced by stand-alone-tomcat-10/build.sh and is not part of this source tree")
	}
	extracted := filepath.Join(t.TempDir(), "acugis-report-map.jar")
	extractWarEntry(t, war, "WEB-INF/lib/acugis-report-map.jar", extracted)
	assertJavapNineArg(t, extracted)
	runMapRendererVerify(t, "--war", war)
}

func TestPackageReleaseRejectsSevenArgumentMapRenderer(t *testing.T) {
	dir := t.TempDir()
	jarPath := filepath.Join(dir, "acugis-report-map.jar")
	compileMapRendererJar(t, dir, jarPath, sevenArgMapRenderer)
	warPath := filepath.Join(dir, "legacy-map.war")
	writeZipWar(t, jarPath, warPath)

	bash := findBash(t)
	script := filepath.Join(repoRoot(t), "scripts", "verify-map-renderer-api.sh")
	cmd := exec.Command(bash, bashPath(script), "--war", bashPath(warPath))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("release check accepted a 7-argument MapRenderer WAR:\n%s", out)
	}
	text := string(out)
	if !strings.Contains(text, "latitudeField") || !strings.Contains(text, "does not expose") {
		t.Fatalf("release check error:\n%s", text)
	}
}

func TestVerifyScriptAcceptsNineArgumentMapRenderer(t *testing.T) {
	dir := t.TempDir()
	jarPath := filepath.Join(dir, "acugis-report-map.jar")
	compileMapRendererJar(t, dir, jarPath, nineArgMapRenderer)
	warPath := filepath.Join(dir, "current-map.war")
	writeZipWar(t, jarPath, warPath)
	runMapRendererVerify(t, "--jar", jarPath)
	runMapRendererVerify(t, "--war", warPath)
}

const sevenArgMapRenderer = `package com.acugis.map;
import java.awt.image.BufferedImage;
import java.sql.Connection;
public final class MapRenderer {
    public static BufferedImage render(Connection connection, String sql, int width, int height, String styleJson, String basemap, String srs) {
        return null;
    }
}
`

const nineArgMapRenderer = `package com.acugis.map;
import java.awt.image.BufferedImage;
import java.sql.Connection;
public final class MapRenderer {
    public static BufferedImage render(Connection connection, String sql, int width, int height, String styleJson, String basemap, String srs, String latitudeField, String longitudeField) {
        return null;
    }
}
`

func compileMapRendererJar(t *testing.T, dir, jarPath, source string) {
	t.Helper()
	srcDir := filepath.Join(dir, "src", "com", "acugis", "map")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(srcDir, "MapRenderer.java")
	if err := os.WriteFile(srcFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	classes := filepath.Join(dir, "classes")
	if err := os.MkdirAll(classes, 0o755); err != nil {
		t.Fatal(err)
	}
	javac := lookTool(t, "javac")
	cmd := exec.Command(javac, "-encoding", "UTF-8", "-d", classes, srcFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	jarTool := lookTool(t, "jar")
	cmd = exec.Command(jarTool, "cf", jarPath, "-C", classes, ".")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jar: %v\n%s", err, out)
	}
}

func assertJavapNineArg(t *testing.T, jarPath string) {
	t.Helper()
	javap := lookTool(t, "javap")
	cmd := exec.Command(javap, "-public", "-classpath", jarPath, "com.acugis.map.MapRenderer")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("javap %s: %v\n%s", jarPath, err, out)
	}
	flat := strings.Join(strings.Fields(string(out)), "")
	const needle = "render(java.sql.Connection,java.lang.String,int,int,java.lang.String,java.lang.String,java.lang.String,java.lang.String,java.lang.String)"
	if !strings.Contains(flat, needle) {
		t.Fatalf("javap missing 9-argument render in %s:\n%s", jarPath, out)
	}
}

func runMapRendererVerify(t *testing.T, flag, path string) {
	t.Helper()
	bash := findBash(t)
	script := filepath.Join(repoRoot(t), "scripts", "verify-map-renderer-api.sh")
	cmd := exec.Command(bash, bashPath(script), flag, bashPath(path))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify %s %s: %v\n%s", flag, path, err, out)
	}
}

func extractWarEntry(t *testing.T, warPath, name, dest string) {
	t.Helper()
	r, err := zip.OpenReader(warPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		out, err := os.Create(dest)
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		if _, err := out.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("%s missing %s", warPath, name)
}

func writeZipWar(t *testing.T, jarPath, warPath string) {
	t.Helper()
	data, err := os.ReadFile(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(warPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create("WEB-INF/lib/acugis-report-map.jar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func lookTool(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("%s is required on PATH", name)
	}
	return p
}

package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSourceRevisionPackaging(t *testing.T) {
	bash := findBash(t)
	root := repoRoot(t)
	helper := filepath.Join(root, "scripts", "source-revision.sh")
	release := filepath.Join(root, "scripts", "package-release.sh")
	deb := filepath.Join(root, "scripts", "package-deb.sh")
	for _, path := range []string{helper, release, deb} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	releaseSrc := string(mustRead(t, release))
	debSrc := string(mustRead(t, deb))
	for _, src := range []string{releaseSrc, debSrc} {
		if !strings.Contains(src, "resolve_source_revision") {
			t.Fatal("package script does not resolve the source revision")
		}
		withoutGoProbe := strings.ReplaceAll(src, "go version 2>/dev/null || echo unknown", "")
		if strings.Contains(withoutGoProbe, "echo unknown") || strings.Contains(src, "revision=unknown") {
			t.Fatal("package script can still record revision=unknown")
		}
	}
	if !strings.Contains(releaseSrc, "revision=${REVISION}") {
		t.Fatal("RELEASE manifest does not record the resolved revision")
	}

	sha := "0123456789abcdef0123456789abcdef01234567"
	if len(sha) != 40 {
		t.Fatal("fixture SHA must be a full commit id")
	}

	t.Run("explicit revision without git", func(t *testing.T) {
		dir := t.TempDir()
		out := filepath.Join(dir, "RELEASE")
		script := `
set -euo pipefail
source "$HELPER"
rev="$(REVISION="$SHA" resolve_source_revision "$EMPTY")"
cat > "$OUT" <<EOF
product=AcuGIS Report Server
version=1.0.16
edition=oss
architecture=linux-amd64
revision=${rev}
EOF
grep -qx "revision=${SHA}" "$OUT"
printf '%s\n' "$rev"
`
		got := runBash(t, bash, script, map[string]string{
			"HELPER": bashPath(helper),
			"EMPTY":  bashPath(dir),
			"OUT":    bashPath(out),
			"SHA":    sha,
		})
		if strings.TrimSpace(got) != sha {
			t.Fatalf("revision %q", got)
		}
	})

	t.Run("git derived revision", func(t *testing.T) {
		dir := t.TempDir()
		setup := `
set -euo pipefail
cd "$DIR"
git init -q
git config user.email test@example.com
git config user.name test
git config core.autocrlf false
printf 'a\n' > a.txt
git add a.txt
git commit -q -m init
source "$HELPER"
unset REVISION
rev="$(resolve_source_revision "$DIR")"
want="$(git rev-parse HEAD)"
test "$rev" = "$want"
printf '%s\n' "$rev"
`
		got := lastLine(runBash(t, bash, setup, map[string]string{
			"DIR":    bashPath(dir),
			"HELPER": bashPath(helper),
		}))
		if len(got) != 40 {
			t.Fatalf("git revision %q", got)
		}
	})

	t.Run("recorded SOURCE_REVISION without git", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "SOURCE_REVISION"), []byte(sha+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		script := `
set -euo pipefail
source "$HELPER"
unset REVISION
rev="$(resolve_source_revision "$DIR")"
test "$rev" = "$SHA"
printf '%s\n' "$rev"
`
		got := strings.TrimSpace(runBash(t, bash, script, map[string]string{
			"HELPER": bashPath(helper),
			"DIR":    bashPath(dir),
			"SHA":    sha,
		}))
		if got != sha {
			t.Fatalf("recorded revision %q", got)
		}
	})

	t.Run("release build fails without revision or git", func(t *testing.T) {
		for _, edition := range []string{"oss"} {
			dir := t.TempDir()
			if err := copyFile(filepath.Join(dir, "scripts", "source-revision.sh"), helper); err != nil {
				t.Fatal(err)
			}
			if err := copyFile(filepath.Join(dir, "scripts", "package-release.sh"), release); err != nil {
				t.Fatal(err)
			}
			if err := copyFile(filepath.Join(dir, "scripts", "package-deb.sh"), deb); err != nil {
				t.Fatal(err)
			}
			for _, script := range []string{
				filepath.Join(dir, "scripts", "package-release.sh"),
				filepath.Join(dir, "scripts", "package-deb.sh"),
			} {
				cmd := exec.Command(bash, bashPath(script), edition, "1.0.16")
				cmd.Env = envWithout("REVISION")
				out, err := cmd.CombinedOutput()
				if err == nil {
					t.Fatalf("%s succeeded without a revision:\n%s", edition, out)
				}
				text := string(out)
				if !strings.Contains(text, "source revision is required") && !strings.Contains(text, "cannot determine source revision") {
					t.Fatalf("%s error:\n%s", edition, text)
				}
				if strings.Contains(text, "revision=unknown") || strings.Contains(text, "Compiling") {
					t.Fatalf("release continued without a revision:\n%s", text)
				}
			}
		}
	})
}

func TestDockerSourceRevisionAndRuntimeOwner(t *testing.T) {
	root := repoRoot(t)
	makeText := string(mustRead(t, filepath.Join(root, "Makefile")))
	goMake := string(mustRead(t, filepath.Join(root, "go-report-server", "Makefile")))
	docker := string(mustRead(t, filepath.Join(root, "docker", "report-server", "Dockerfile")))
	entry := string(mustRead(t, filepath.Join(root, "docker", "report-server", "docker-entrypoint.sh")))
	for _, src := range []string{makeText, goMake} {
		if !strings.Contains(src, "resolve_source_revision") {
			t.Fatal("docker Makefile does not resolve SOURCE_REVISION on the host")
		}
		if strings.Count(src, "--build-arg SOURCE_REVISION=$(SOURCE_REVISION)") < 1 {
			t.Fatal("docker Makefile does not pass SOURCE_REVISION")
		}
	}
	if strings.Contains(docker, "SOURCE_REVISION=unknown") {
		t.Fatal("Dockerfile still defaults SOURCE_REVISION to unknown")
	}
	if !strings.Contains(docker, `org.opencontainers.image.revision="${SOURCE_REVISION}"`) {
		t.Fatal("Dockerfile does not label the image with the source revision")
	}
	if !strings.Contains(docker, "DefaultAppVersion=${VERSION}") {
		t.Fatal("Dockerfile stopped stamping the application version")
	}
	if !strings.Contains(entry, `JRS_REPORTS_OWNER="$(id -un)"`) || !strings.Contains(entry, `JRS_REPORTS_GROUP="$(id -gn)"`) {
		t.Fatal("entrypoint does not default report ownership to the runtime user")
	}

	bash := findBash(t)
	script := `
set -euo pipefail
unset JRS_REPORTS_OWNER JRS_REPORTS_GROUP
if [[ -z "${JRS_REPORTS_OWNER:-}" ]]; then export JRS_REPORTS_OWNER="$(id -un)"; fi
if [[ -z "${JRS_REPORTS_GROUP:-}" ]]; then export JRS_REPORTS_GROUP="$(id -gn)"; fi
test "$JRS_REPORTS_OWNER" = "$(id -un)"
test "$JRS_REPORTS_GROUP" = "$(id -gn)"
export JRS_REPORTS_OWNER=tomcat
export JRS_REPORTS_GROUP=tomcat
if [[ -z "${JRS_REPORTS_OWNER:-}" ]]; then export JRS_REPORTS_OWNER="$(id -un)"; fi
if [[ -z "${JRS_REPORTS_GROUP:-}" ]]; then export JRS_REPORTS_GROUP="$(id -gn)"; fi
test "$JRS_REPORTS_OWNER" = tomcat
test "$JRS_REPORTS_GROUP" = tomcat
`
	runBash(t, bash, script, nil)
}

func findBash(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		for _, p := range []string{
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files\Git\usr\bin\bash.exe`,
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	if p, err := exec.LookPath("bash"); err == nil {
		low := strings.ToLower(filepath.Clean(p))
		if runtime.GOOS == "windows" && (strings.Contains(low, `\windows\system32\`) || strings.Contains(low, `\windowsapps\`)) {
			t.Fatal("Git bash is required; Windows bash.exe is the WSL launcher")
		}
		return p
	}
	t.Fatal("bash is required")
	return ""
}

func envWithout(key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return root
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func bashPath(path string) string {
	p := filepath.ToSlash(path)
	if len(p) >= 2 && p[1] == ':' {
		return "/" + strings.ToLower(p[:1]) + p[2:]
	}
	return p
}

func runBash(t *testing.T, bash, script string, env map[string]string) string {
	t.Helper()
	cmd := exec.Command(bash, "-c", script)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash: %v\n%s", err, out)
	}
	return string(out)
}

func copyFile(dst, src string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b = []byte(strings.ReplaceAll(string(b), "\r\n", "\n"))
	return os.WriteFile(dst, b, 0o644)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

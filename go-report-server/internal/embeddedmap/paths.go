package embeddedmap

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/fileown"
)

// Paths holds filesystem locations for map PNG cache and Tomcat path translation.
type Paths struct {
	JRXMLDir                  string
	JasperReportsContainerDir string
	FileOwner                 string
	FileGroup                 string
}

// CacheDir returns {JRXMLDir}/_map_cache, creating it if needed.
func (p Paths) CacheDir() (string, error) {
	dir := filepath.Join(p.JRXMLDir, "_map_cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p.Own(p.JRXMLDir)
	p.Own(dir)
	return dir, nil
}

// Own applies configured ownership/mode so the Tomcat process can read files.
func (p Paths) Own(path string) {
	_ = fileown.Apply(path, p.FileOwner, p.FileGroup)
}

// TomcatPath rewrites a host path under JRXMLDir to the path Tomcat sees.
// When the container dir matches the host reports dir (standalone), returns the
// absolute host path. Otherwise rewrites to the container mount base.
func (p Paths) TomcatPath(hostPath string) string {
	reportsDir, _ := filepath.Abs(p.JRXMLDir)
	hostReal, err := filepath.Abs(hostPath)
	if err != nil {
		hostReal = hostPath
	}
	reportsDirSlash := filepath.ToSlash(reportsDir)
	hostRealSlash := filepath.ToSlash(hostReal)

	containerBase := strings.TrimRight(strings.ReplaceAll(p.JasperReportsContainerDir, `\`, `/`), "/")
	if containerBase == "" {
		containerBase = reportsDirSlash
	}

	rel := ""
	if strings.HasPrefix(strings.ToLower(hostRealSlash), strings.ToLower(reportsDirSlash)) {
		rel = strings.TrimPrefix(hostRealSlash[len(reportsDirSlash):], "/")
		rel = strings.TrimPrefix(rel, "/")
	} else {
		rel = "_map_cache/" + filepath.Base(hostReal)
	}

	// Standalone: host path == Tomcat path (same filesystem). Prefer absolute so
	// FileRepositoryService(resolveAbsolute=true) can open the PNG directly.
	if sameReportsPath(containerBase, reportsDirSlash) {
		return hostRealSlash
	}
	return containerBase + "/" + rel
}

func sameReportsPath(a, b string) bool {
	a = strings.TrimRight(strings.ReplaceAll(a, `\`, `/`), "/")
	b = strings.TrimRight(strings.ReplaceAll(b, `\`, `/`), "/")
	return strings.EqualFold(a, b)
}

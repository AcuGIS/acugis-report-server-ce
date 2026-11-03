package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/resourcefs"
)

const (
	folderHome    int64 = 1
	folderDavidG  int64 = 10
	folderFinance int64 = 20
)

type queryCapture struct {
	queries []url.Values
}

func (c *queryCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q, _ := url.ParseQuery(r.URL.RawQuery)
	c.queries = append(c.queries, q)
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte("<html>ok</html>"))
}

type resourceExecEnv struct {
	normal *Server
	embed  *Server
	store  *reports.Store
	jrxml  string
	paths  string
	json   string
	ncap   *queryCapture
	ecap   *queryCapture
}

func newResourceExecEnv(t *testing.T, membership map[string]int64) *resourceExecEnv {
	t.Helper()
	jrxml := t.TempDir()
	jsonDir := t.TempDir()
	paths := t.TempDir()
	store := reports.NewStore(jsonDir, t.TempDir(), jrxml)
	ncap := &queryCapture{}
	ecap := &queryCapture{}
	normalJ := httptest.NewServer(ncap)
	embedJ := httptest.NewServer(ecap)
	t.Cleanup(normalJ.Close)
	t.Cleanup(embedJ.Close)

	slash := func(id int64) (string, error) {
		switch id {
		case folderDavidG:
			return "/DavidG", nil
		case folderFinance:
			return "/Finance", nil
		case folderHome:
			return "", nil
		default:
			t.Fatalf("unexpected folder %d", id)
			return "", nil
		}
	}
	locate := func(key string) (int64, bool) {
		id, ok := membership[key]
		return id, ok
	}
	build := func(authDisabled bool, jasperURL string, authSvc *auth.Service) *Server {
		cfg := testConfig()
		cfg.AuthDisabled = authDisabled
		cfg.EmbedTokenSecret = boundTestSecret
		cfg.EmbedAllowedOrigins = []string{"https://property-pro.example.com"}
		cfg.EmbedTokenTTL = time.Minute
		cfg.Jasper.ReportURL = jasperURL
		cfg.Jasper.ImageURL = jasperURL + "/img"
		cfg.Jasper.Timeout = 2 * time.Second
		cfg.Paths.JRXMLDir = jrxml
		cfg.Paths.ReportJSONDir = jsonDir
		cfg.Paths.ResourcePathsDir = paths
		gw := jasper.NewGateway(cfg.Jasper, jasper.Options{ReportExists: func(string) bool { return true }})
		deps := Dependencies{
			Gateway: gw,
			Jasper:  gw.Client(),
			Reports: store,
			Auth:    authSvc,
		}
		if authDisabled {
			deps.ACL = authDisabledACL()
			deps.Auth = auth.NewService(nil, auth.Options{AuthDisabled: true, CookieName: "jrs_session"})
		} else {
			deps.ACL = allowAuthenticatedACL{}
		}
		s := New(cfg, deps, slog.Default())
		s.testReportFolder = locate
		s.testSlashFolder = slash
		return s
	}
	return &resourceExecEnv{
		normal: build(true, normalJ.URL, nil),
		embed:  build(false, embedJ.URL, embedAuthService()),
		store:  store,
		jrxml:  jrxml,
		paths:  paths,
		json:   jsonDir,
		ncap:   ncap,
		ecap:   ecap,
	}
}

func (e *resourceExecEnv) writeMeta(t *testing.T, key, base string) {
	t.Helper()
	body := []byte("{\"key\":\"" + key + "\",\"name\":\"" + key + "\"}\n")
	if err := os.WriteFile(filepath.Join(e.json, key+".report.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if base == "" {
		return
	}
	if err := e.store.SaveResourceBase(key, base); err != nil {
		t.Fatal(err)
	}
}

func (e *resourceExecEnv) run(t *testing.T, method, key string, embedded bool) url.Values {
	t.Helper()
	srv := e.normal
	cap := e.ncap
	var rdr io.Reader
	if method == http.MethodPost {
		rdr = strings.NewReader("_repFormat=html")
	}
	req := httptest.NewRequest(method, "/reports/execute?_repName="+key+"&_repFormat=html", rdr)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if embedded {
		srv = e.embed
		cap = e.ecap
		req.AddCookie(&http.Cookie{Name: "jrs_embed_token", Value: issueBoundToken(t, nil, key, time.Minute)})
	}
	before := len(cap.queries)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s embed=%v status=%d body=%s", method, key, embedded, rec.Code, rec.Body.String())
	}
	if len(cap.queries) != before+1 {
		t.Fatalf("jasper calls=%d", len(cap.queries))
	}
	return cap.queries[len(cap.queries)-1]
}

func baseFile(jrxml, key string) string {
	return filepath.Join(jrxml, key+".resource-base")
}

func readBaseFile(t *testing.T, jrxml, key string) string {
	t.Helper()
	b, err := os.ReadFile(baseFile(jrxml, key))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func putImage(t *testing.T, paths, base, payload string) string {
	t.Helper()
	full := filepath.Join(append([]string{paths}, append(strings.Split(base, "/"), "images", "logo.png")...)...)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func resolvedImage(t *testing.T, jrxml, paths, key string) string {
	t.Helper()
	full, err := resourcefs.NewStore(jrxml, paths).Resolve(key, "images/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestExecutePreservesDesignerResourceBaseInFolder(t *testing.T) {
	env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
	env.writeMeta(t, "testingone", "DavidG/testingone")
	meta := mustRead(t, filepath.Join(env.json, "testingone.report.json"))
	info, err := os.Stat(baseFile(env.jrxml, "testingone"))
	if err != nil {
		t.Fatal(err)
	}
	putImage(t, env.paths, "DavidG/testingone", "per-report-logo")
	putImage(t, env.paths, "DavidG", "shared-folder-logo")

	for i, method := range []string{http.MethodGet, http.MethodPost, http.MethodGet} {
		q := env.run(t, method, "testingone", false)
		if q.Get("_acugisResourceBase") != "DavidG/testingone" {
			t.Fatalf("normal run %d base=%q", i, q.Get("_acugisResourceBase"))
		}
	}
	for i, method := range []string{http.MethodGet, http.MethodPost} {
		q := env.run(t, method, "testingone", true)
		if q.Get("_acugisResourceBase") != "DavidG/testingone" {
			t.Fatalf("embed run %d base=%q", i, q.Get("_acugisResourceBase"))
		}
	}
	if got := readBaseFile(t, env.jrxml, "testingone"); got != "DavidG/testingone\n" {
		t.Fatalf("sidecar=%q", got)
	}
	after, err := os.Stat(baseFile(env.jrxml, "testingone"))
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(info.ModTime()) || after.Size() != info.Size() {
		t.Fatalf("sidecar mutated mtime %s -> %s", info.ModTime(), after.ModTime())
	}
	if string(mustRead(t, filepath.Join(env.json, "testingone.report.json"))) != string(meta) {
		t.Fatal("report json mutated")
	}
	if got := resolvedImage(t, env.jrxml, env.paths, "testingone"); got != "per-report-logo" {
		t.Fatalf("resolved %q", got)
	}
}

func TestExecutePreservesDesignerResourceBaseInHome(t *testing.T) {
	env := newResourceExecEnv(t, map[string]int64{"testingone": folderHome})
	env.writeMeta(t, "testingone", "testingone")
	putImage(t, env.paths, "testingone", "home-logo")

	q := env.run(t, http.MethodGet, "testingone", false)
	if q.Get("_acugisResourceBase") != "testingone" {
		t.Fatalf("normal base=%q", q.Get("_acugisResourceBase"))
	}
	q = env.run(t, http.MethodPost, "testingone", true)
	if q.Get("_acugisResourceBase") != "testingone" {
		t.Fatalf("embed base=%q", q.Get("_acugisResourceBase"))
	}
	if got := readBaseFile(t, env.jrxml, "testingone"); got != "testingone\n" {
		t.Fatalf("sidecar=%q", got)
	}
	if got := resolvedImage(t, env.jrxml, env.paths, "testingone"); got != "home-logo" {
		t.Fatalf("resolved %q", got)
	}
}

func TestExecuteKeepsLegacySharedFolderBase(t *testing.T) {
	env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
	env.writeMeta(t, "testingone", "DavidG")
	putImage(t, env.paths, "DavidG", "shared-logo")

	q := env.run(t, http.MethodGet, "testingone", false)
	if q.Get("_acugisResourceBase") != "DavidG" {
		t.Fatalf("normal base=%q", q.Get("_acugisResourceBase"))
	}
	q = env.run(t, http.MethodGet, "testingone", true)
	if q.Get("_acugisResourceBase") != "DavidG" {
		t.Fatalf("embed base=%q", q.Get("_acugisResourceBase"))
	}
	if got := readBaseFile(t, env.jrxml, "testingone"); got != "DavidG\n" {
		t.Fatalf("sidecar=%q", got)
	}
	if _, err := os.Stat(filepath.Join(env.paths, "DavidG", "testingone")); !os.IsNotExist(err) {
		t.Fatal("legacy base was converted into a per-report directory")
	}
	if got := resolvedImage(t, env.jrxml, env.paths, "testingone"); got != "shared-logo" {
		t.Fatalf("resolved %q", got)
	}
}

func TestExecuteMissingResourceBaseUsesCatalogFolder(t *testing.T) {
	t.Run("folder", func(t *testing.T) {
		env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
		env.writeMeta(t, "testingone", "")
		q := env.run(t, http.MethodGet, "testingone", false)
		if q.Get("_acugisResourceBase") != "DavidG" {
			t.Fatalf("base=%q", q.Get("_acugisResourceBase"))
		}
		if got := readBaseFile(t, env.jrxml, "testingone"); got != "DavidG\n" {
			t.Fatalf("fallback sidecar=%q", got)
		}
		info, err := os.Stat(baseFile(env.jrxml, "testingone"))
		if err != nil {
			t.Fatal(err)
		}
		_ = env.run(t, http.MethodPost, "testingone", false)
		_ = env.run(t, http.MethodGet, "testingone", true)
		after, err := os.Stat(baseFile(env.jrxml, "testingone"))
		if err != nil {
			t.Fatal(err)
		}
		if !after.ModTime().Equal(info.ModTime()) {
			t.Fatal("repeated execute rewrote the fallback sidecar")
		}
	})
	t.Run("home", func(t *testing.T) {
		env := newResourceExecEnv(t, map[string]int64{"testingone": folderHome})
		env.writeMeta(t, "testingone", "")
		q := env.run(t, http.MethodGet, "testingone", false)
		if q.Get("_acugisResourceBase") != "" {
			t.Fatalf("home fallback base=%q", q.Get("_acugisResourceBase"))
		}
		if _, err := os.Stat(baseFile(env.jrxml, "testingone")); !os.IsNotExist(err) {
			t.Fatal("home fallback created a resource-base file")
		}
	})
}

func TestExecuteDistinguishesMalformedAndStaleResourceBase(t *testing.T) {
	t.Run("malformed falls back to the catalog folder", func(t *testing.T) {
		env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
		env.writeMeta(t, "testingone", "")
		if err := os.WriteFile(baseFile(env.jrxml, "testingone"), []byte("../escape\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		q := env.run(t, http.MethodGet, "testingone", false)
		if q.Get("_acugisResourceBase") != "DavidG" {
			t.Fatalf("base=%q", q.Get("_acugisResourceBase"))
		}
		if got := readBaseFile(t, env.jrxml, "testingone"); got != "DavidG\n" {
			t.Fatalf("sidecar=%q", got)
		}
	})
	t.Run("stale designer base is not rewritten on execute", func(t *testing.T) {
		env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
		env.writeMeta(t, "testingone", "OldFolder/testingone")
		putImage(t, env.paths, "OldFolder/testingone", "stale-logo")
		q := env.run(t, http.MethodGet, "testingone", false)
		eq := env.run(t, http.MethodGet, "testingone", true)
		if q.Get("_acugisResourceBase") != "OldFolder/testingone" || eq.Get("_acugisResourceBase") != "OldFolder/testingone" {
			t.Fatalf("normal=%q embed=%q", q.Get("_acugisResourceBase"), eq.Get("_acugisResourceBase"))
		}
		if got := readBaseFile(t, env.jrxml, "testingone"); got != "OldFolder/testingone\n" {
			t.Fatalf("sidecar=%q", got)
		}
		if got := resolvedImage(t, env.jrxml, env.paths, "testingone"); got != "stale-logo" {
			t.Fatalf("resolved %q", got)
		}
	})
	t.Run("stale legacy base is not converted to a per-report directory", func(t *testing.T) {
		env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
		env.writeMeta(t, "testingone", "OldFolder")
		q := env.run(t, http.MethodGet, "testingone", false)
		if q.Get("_acugisResourceBase") != "OldFolder" {
			t.Fatalf("base=%q", q.Get("_acugisResourceBase"))
		}
		if got := readBaseFile(t, env.jrxml, "testingone"); got != "OldFolder\n" {
			t.Fatalf("sidecar=%q", got)
		}
	})
}

func TestFolderMoveRelocatesDesignerResourceBase(t *testing.T) {
	env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
	env.writeMeta(t, "testingone", "DavidG/testingone")
	putImage(t, env.paths, "DavidG/testingone", "moved-logo")

	env.normal.applyResourceBaseOnMove(context.Background(), "testingone", folderDavidG, folderFinance)
	if got := readBaseFile(t, env.jrxml, "testingone"); got != "Finance/testingone\n" {
		t.Fatalf("sidecar=%q", got)
	}
	if _, err := os.Stat(filepath.Join(env.paths, "DavidG", "testingone", "images", "logo.png")); !os.IsNotExist(err) {
		t.Fatal("resource stayed in the old per-report directory")
	}
	if got := resolvedImage(t, env.jrxml, env.paths, "testingone"); got != "moved-logo" {
		t.Fatalf("resolved %q", got)
	}
	env.normal.testReportFolder = func(string) (int64, bool) { return folderFinance, true }
	env.embed.testReportFolder = env.normal.testReportFolder
	q := env.run(t, http.MethodGet, "testingone", false)
	eq := env.run(t, http.MethodGet, "testingone", true)
	if q.Get("_acugisResourceBase") != "Finance/testingone" || eq.Get("_acugisResourceBase") != "Finance/testingone" {
		t.Fatalf("normal=%q embed=%q", q.Get("_acugisResourceBase"), eq.Get("_acugisResourceBase"))
	}

	env.normal.applyResourceBaseOnMove(context.Background(), "testingone", folderFinance, folderHome)
	if got := readBaseFile(t, env.jrxml, "testingone"); got != "testingone\n" {
		t.Fatalf("home sidecar=%q", got)
	}
	if _, err := os.Stat(filepath.Join(env.paths, "Finance", "testingone")); !os.IsNotExist(err) {
		t.Fatal("resource stayed under Finance")
	}
	if got := resolvedImage(t, env.jrxml, env.paths, "testingone"); got != "moved-logo" {
		t.Fatalf("home resolved %q", got)
	}
}

func TestFolderMoveLeavesLegacySharedDirectory(t *testing.T) {
	env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
	env.writeMeta(t, "testingone", "DavidG")
	shared := putImage(t, env.paths, "DavidG", "shared-logo")

	env.normal.applyResourceBaseOnMove(context.Background(), "testingone", folderDavidG, folderFinance)
	if got := readBaseFile(t, env.jrxml, "testingone"); got != "Finance\n" {
		t.Fatalf("sidecar=%q", got)
	}
	if _, err := os.Stat(filepath.Join(env.paths, "Finance", "testingone")); !os.IsNotExist(err) {
		t.Fatal("legacy move created a per-report directory")
	}
	if string(mustRead(t, shared)) != "shared-logo" {
		t.Fatal("shared folder resource was moved")
	}
}

func TestFolderMoveKeepsDesignerBaseWhenDestinationExists(t *testing.T) {
	env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
	env.writeMeta(t, "testingone", "DavidG/testingone")
	putImage(t, env.paths, "DavidG/testingone", "original-logo")
	putImage(t, env.paths, "Finance/testingone", "other-logo")

	env.normal.applyResourceBaseOnMove(context.Background(), "testingone", folderDavidG, folderFinance)
	if got := readBaseFile(t, env.jrxml, "testingone"); got != "DavidG/testingone\n" {
		t.Fatalf("sidecar changed to %q", got)
	}
	if got := resolvedImage(t, env.jrxml, env.paths, "testingone"); got != "original-logo" {
		t.Fatalf("resolved %q", got)
	}
	other := filepath.Join(env.paths, "Finance", "testingone", "images", "logo.png")
	if string(mustRead(t, other)) != "other-logo" {
		t.Fatal("destination resource was overwritten")
	}
}

func TestExecuteSaveAsCopyUsesItsOwnResourceBase(t *testing.T) {
	env := newResourceExecEnv(t, map[string]int64{
		"sales-report": folderDavidG,
		"revised-copy": folderDavidG,
	})
	env.writeMeta(t, "sales-report", "DavidG/sales-report")
	env.writeMeta(t, "revised-copy", "DavidG/revised-copy")
	putImage(t, env.paths, "DavidG/sales-report", "source-logo")
	putImage(t, env.paths, "DavidG/revised-copy", "copy-logo")

	q := env.run(t, http.MethodGet, "revised-copy", false)
	eq := env.run(t, http.MethodPost, "revised-copy", true)
	if q.Get("_acugisResourceBase") != "DavidG/revised-copy" || eq.Get("_acugisResourceBase") != "DavidG/revised-copy" {
		t.Fatalf("normal=%q embed=%q", q.Get("_acugisResourceBase"), eq.Get("_acugisResourceBase"))
	}
	if got := readBaseFile(t, env.jrxml, "revised-copy"); got != "DavidG/revised-copy\n" {
		t.Fatalf("copy sidecar=%q", got)
	}
	if got := readBaseFile(t, env.jrxml, "sales-report"); got != "DavidG/sales-report\n" {
		t.Fatalf("source sidecar=%q", got)
	}
	if got := resolvedImage(t, env.jrxml, env.paths, "revised-copy"); got != "copy-logo" {
		t.Fatalf("copy resolved %q", got)
	}
	if got := resolvedImage(t, env.jrxml, env.paths, "sales-report"); got != "source-logo" {
		t.Fatalf("source resolved %q", got)
	}
}

func TestPublishPreservesDesignerBaseAndRefreshesMissing(t *testing.T) {
	env := newResourceExecEnv(t, map[string]int64{"testingone": folderDavidG})
	env.writeMeta(t, "testingone", "DavidG/testingone")
	env.normal.refreshPublishedResourceBase(context.Background(), "testingone", folderDavidG)
	if got := readBaseFile(t, env.jrxml, "testingone"); got != "DavidG/testingone\n" {
		t.Fatalf("publish rewrote designer base to %q", got)
	}

	legacy := newResourceExecEnv(t, map[string]int64{"quarterly": folderDavidG})
	legacy.writeMeta(t, "quarterly", "")
	legacy.normal.refreshPublishedResourceBase(context.Background(), "quarterly", folderDavidG)
	if got := readBaseFile(t, legacy.jrxml, "quarterly"); got != "DavidG\n" {
		t.Fatalf("publish fallback=%q", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

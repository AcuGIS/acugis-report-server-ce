package server

import (
	_ "embed"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

//go:embed report_runner.html
var reportRunnerHTML string

type reportRunnerPageData struct {
	materialChromeData
	ReportFiles      []string
	ReportEndpointJS template.JS
}

var reportRunnerTemplate = template.Must(template.New("report_runner").Parse(reportRunnerHTML))

func (s *Server) browserReportEndpoint() string {
	ep := strings.TrimSpace(s.cfg.ReportEndpoint)
	if ep == "" {
		ep = "execute_report.php"
	}
	if strings.HasPrefix(ep, "http://") || strings.HasPrefix(ep, "https://") {
		return ep
	}
	if strings.HasPrefix(ep, "/") {
		return ep
	}
	if ep == "execute_report.php" {
		return s.goURL("/reports/execute")
	}
	return s.goURL("/reports/" + strings.TrimPrefix(ep, "./"))
}

func (s *Server) handleReportRunnerPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := auth.FromContext(r.Context())
	files, err := s.listReportRunnerFiles(r, user)
	if err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}

	data := reportRunnerPageData{
		materialChromeData: s.materialChrome("Report URL Builder"),
		ReportFiles:        files,
		ReportEndpointJS:   mustJSON(s.browserReportEndpoint()),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := reportRunnerTemplate.Execute(w, data); err != nil {
		s.logger.Error("render report runner", "error", err)
	}
}

func (s *Server) listReportRunnerFiles(r *http.Request, user *auth.User) ([]string, error) {
	if s.deps.Reports == nil {
		return nil, nil
	}
	items, err := s.deps.Reports.List()
	if err != nil {
		return nil, err
	}
	isAdmin := s.isPlatformAdmin(user)
	out := make([]string, 0, len(items))
	for _, item := range items {
		key := jasper.NormalizeReportKey(item.Key)
		if !jasper.IsValidReportKey(key) {
			continue
		}
		if !isAdmin {
			ok, err := s.canViewReport(r, user, key)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		out = append(out, key+".report.json")
	}
	sort.Strings(out)
	return out, nil
}

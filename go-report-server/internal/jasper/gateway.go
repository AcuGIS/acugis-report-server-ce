package jasper

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/config"
)

// Gateway is the only Go component that communicates with Tomcat/JasperReports.
type Gateway interface {
	ExecuteReport(ctx context.Context, req ExecuteRequest) (*ProxyResponse, error)
	ExecuteReportQuery(ctx context.Context, req ReportQueryRequest) (*ReportQueryResult, error)
	FetchReportImage(ctx context.Context, req ImageRequest) (*ProxyResponse, error)
	Status() Status
	Timeout() time.Duration
	Client() *Client
}

// ReportExistsFunc checks whether a report definition exists on disk.
type ReportExistsFunc func(reportName string) bool

type gateway struct {
	client       *Client
	reportExists ReportExistsFunc

	mu               sync.RWMutex
	lastSuccessfulAt *time.Time
	lastErrorAt      *time.Time
	lastErrorSummary string
	lastReportName   string
	lastFormat       string
	lastStatusCode   int
	lastConnectivity string
}

// Options configures a Gateway.
type Options struct {
	ReportJSONDir string
	ReportExists  ReportExistsFunc
}

// NewGateway constructs the Jasper gateway.
func NewGateway(cfg config.JasperConfig, opts Options) Gateway {
	client := New(cfg)
	exists := opts.ReportExists
	if exists == nil && opts.ReportJSONDir != "" {
		dir := opts.ReportJSONDir
		exists = func(reportName string) bool {
			path := filepath.Join(dir, reportName+".report.json")
			st, err := os.Stat(path)
			return err == nil && !st.IsDir()
		}
	}
	return &gateway{
		client:           client,
		reportExists:     exists,
		lastConnectivity: "unknown",
	}
}

// NewGatewayFromClient wraps an existing client (useful in tests).
func NewGatewayFromClient(client *Client, opts Options) Gateway {
	exists := opts.ReportExists
	if exists == nil && opts.ReportJSONDir != "" {
		dir := opts.ReportJSONDir
		exists = func(reportName string) bool {
			path := filepath.Join(dir, reportName+".report.json")
			st, err := os.Stat(path)
			return err == nil && !st.IsDir()
		}
	}
	return &gateway{
		client:           client,
		reportExists:     exists,
		lastConnectivity: "unknown",
	}
}

func (g *gateway) Client() *Client {
	if g == nil {
		return nil
	}
	return g.client
}

func (g *gateway) Timeout() time.Duration {
	if g == nil || g.client == nil {
		return config.DefaultJasperTimeout
	}
	return g.client.Timeout()
}

func (g *gateway) Status() Status {
	g.mu.RLock()
	defer g.mu.RUnlock()

	st := Status{
		Configured:       g.client != nil && g.client.Configured(),
		Timeout:          g.Timeout().String(),
		Connectivity:     g.lastConnectivity,
		LastErrorSummary: g.lastErrorSummary,
		LastReportName:   g.lastReportName,
		LastFormat:       g.lastFormat,
		LastStatusCode:   g.lastStatusCode,
	}
	if g.client != nil {
		st.ReportURL = g.client.ReportURL()
		st.ImageURL = g.client.ImageURL()
	}
	if g.lastSuccessfulAt != nil {
		t := *g.lastSuccessfulAt
		st.LastSuccessfulAt = &t
	}
	if g.lastErrorAt != nil {
		t := *g.lastErrorAt
		st.LastErrorAt = &t
	}
	if st.Connectivity == "" {
		st.Connectivity = "unknown"
	}
	return st
}

func (g *gateway) recordSuccess(reportName, format string, status int) {
	if g == nil {
		return
	}
	now := time.Now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastSuccessfulAt = &now
	g.lastConnectivity = "ok"
	g.lastReportName = reportName
	g.lastFormat = format
	g.lastStatusCode = status
	g.lastErrorSummary = ""
}

func (g *gateway) recordError(err error) {
	if g == nil || err == nil {
		return
	}
	now := time.Now().UTC()
	summary := err.Error()
	markError := true
	if ge, ok := AsError(err); ok {
		summary = ge.PublicMessage()
		markError = ge.Code == CodeUnavailable || ge.Code == CodeTimeout || ge.Code == CodeBadGateway || ge.Code == CodeInternal
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastErrorAt = &now
	g.lastErrorSummary = summary
	if markError {
		g.lastConnectivity = "error"
	}
}

func (g *gateway) setLastMeta(reportName, format string, status int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastReportName = reportName
	g.lastFormat = format
	g.lastStatusCode = status
}

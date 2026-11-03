package schedules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/arcgis"
	"github.com/acugis/acugis-report-server-go/internal/bigquery"
	"github.com/acugis/acugis-report-server-go/internal/cassandra"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/duckdb"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// Runner executes scheduled reports via Jasper HTTP.
type Runner struct {
	paths       Paths
	repo        *Repository
	executions  *ExecutionRepository
	mail        *Mailer
	templates   *EmailTemplateStore
	reportURL   string
	timeout     time.Duration
	Reports     *reports.Store
	Datasources datasources.Finder
	EmbeddedMap *embeddedmap.Service
	Edition     *edition.Service // optional; when set, blocks Professional-only schedule targets
	// ResolveResourceBytes loads content resource bytes by UUID (BigQuery SA JSON).
	// Optional; BigQuery schedules fail clearly when unset.
	ResolveResourceBytes func(ctx context.Context, resourceUUID string) ([]byte, error)
	// ArcGIS resolves jasper_arcgis credentials for Feature Server collection fill.
	// Nil leaves ArcGIS reports unprepared (they must not fall through to JDBC).
	ArcGIS arcgis.Catalog
	// GeoServer resolves jasper_geoserver credentials for WFS collection fill.
	// Nil leaves GeoServer reports unprepared (they must not fall through to JDBC).
	GeoServer geoserver.Catalog
	// ResourcePaths is the Report Server resource-paths directory. Scheduled CSV
	// reports resolve their published file through {key}.resource-base.
	ResourcePaths string
}

// RunnerConfig configures report execution.
type RunnerConfig struct {
	ReportURL string
	Timeout   time.Duration
}

// NewRunner constructs a schedule runner.
func NewRunner(paths Paths, repo *Repository, executions *ExecutionRepository, mail *Mailer, templates *EmailTemplateStore, cfg RunnerConfig) *Runner {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 600 * time.Second
	}
	return &Runner{
		paths:      paths,
		repo:       repo,
		executions: executions,
		mail:       mail,
		templates:  templates,
		reportURL:  strings.TrimRight(strings.TrimSpace(cfg.ReportURL), "/"),
		timeout:    cfg.Timeout,
	}
}

// Execute runs a schedule (manual or scheduled mode).
func (r *Runner) Execute(ctx context.Context, scheduleID int, mode string) ExecuteResult {
	if mode != "manual" && mode != "scheduled" {
		mode = "manual"
	}

	executionID := 0
	outputFile := ""
	emailStatus := ""

	defer func() {
		if rec := recover(); rec != nil {
			msg := fmt.Sprint(rec)
			r.log(scheduleID, "ERROR: "+msg)
			if r.executions != nil && executionID > 0 {
				_ = r.executions.Finish(ctx, executionID, "failed", outputFile, emailStatus, msg)
			}
		}
	}()

	r.log(scheduleID, fmt.Sprintf("START schedule=%d mode=%s", scheduleID, mode))

	if r.repo == nil {
		return r.fail(scheduleID, executionID, outputFile, emailStatus, "Database unavailable")
	}
	schedule, err := r.loadForRun(ctx, scheduleID, mode)
	if err != nil {
		return r.fail(scheduleID, executionID, outputFile, emailStatus, err.Error())
	}

	r.logContext(scheduleID, *schedule, mode)

	if r.executions != nil {
		executionID, err = r.executions.Create(ctx, scheduleID)
		if err != nil || executionID <= 0 {
			return r.fail(scheduleID, executionID, outputFile, emailStatus, "Failed to create execution history row")
		}
		r.log(scheduleID, fmt.Sprintf("Created execution_id=%d", executionID))
	}

	outputDir, err := r.paths.PrepareOutputDir(scheduleID)
	if err != nil {
		return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, err.Error())
	}

	baseName := sanitizeFilename(schedule.Filename)
	ext := schedule.Format
	if strings.EqualFold(ext, "html") {
		ext = "html"
	}
	outputFile = filepath.Join(outputDir, fmt.Sprintf("%s_%s.%s", baseName, time.Now().Format("20060102_150405"), ext))

	params := ParseURLParams(schedule.URLOptParams)
	query := url.Values{}
	query.Set("_repName", schedule.ReportName)
	query.Set("_dataSource", schedule.DatasourceName)
	query.Set("_repFormat", schedule.Format)
	for k, v := range params {
		query.Set(k, v)
	}

	if r.reportURL == "" {
		return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, "Jasper engine URL not configured")
	}

	if r.Reports != nil {
		if def, err := r.Reports.Get(schedule.ReportName); err == nil && def != nil && mapreport.IsMapReport(def) {
			ed := r.Edition
			if ed == nil && r.EmbeddedMap != nil {
				ed = r.EmbeddedMap.Edition
			}
			if ed != nil {
				if err := ed.RequireFeature(edition.FeatureMapReports); err != nil {
					return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, err.Error())
				}
			}
		}
	}

	qs := query.Encode()
	if r.EmbeddedMap != nil && r.Reports != nil {
		if def, err := r.Reports.Get(schedule.ReportName); err == nil && def != nil {
			if def.Raw == nil {
				def.Raw = map[string]any{}
			}
			if strings.TrimSpace(fmt.Sprint(def.Raw["report"])) == "" || fmt.Sprint(def.Raw["report"]) == "<nil>" {
				def.Raw["report"] = schedule.ReportName
			}
			filterParams := map[string]any{}
			for k, v := range params {
				filterParams[k] = v
			}
			mapResult := r.EmbeddedMap.AppendToQuery(ctx, def, qs, filterParams)
			if mapResult.Error != "" && embeddedmap.ReportEmbeddedMap(def) != nil {
				r.log(scheduleID, "Embedded map warning: "+mapResult.Error)
			}
			if mapResult.Query != "" {
				qs = mapResult.Query
			}
		}
	}
	httpMethod := http.MethodGet
	var reqBody io.Reader
	contentType := ""
	requestURL := r.reportURL + "?" + qs
	if filled, method, outQS, ct, rdr, ferr := r.prepareCassandraCollection(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "Cassandra collection fill attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	} else if filled, method, outQS, ct, rdr, ferr := r.prepareBigQueryCollection(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "BigQuery collection fill attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	} else if filled, method, outQS, ct, rdr, ferr := r.prepareArcGISCollection(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "ArcGIS collection fill attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	} else if filled, method, outQS, ct, rdr, ferr := r.prepareGeoServerCollection(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "GeoServer collection fill attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	} else if filled, method, outQS, ct, rdr, ferr := r.prepareCSVCollection(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "CSV collection fill attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	} else if filled, method, outQS, ct, rdr, ferr := r.prepareJSONCollection(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "JSON collection fill attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	} else if filled, method, outQS, ct, rdr, ferr := r.prepareExcelCollection(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "Excel collection fill attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	} else if filled, method, outQS, ct, rdr, ferr := r.prepareXMLCollection(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "XML collection fill attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	} else if filled, method, outQS, ct, rdr, ferr := r.prepareDuckDBJDBC(ctx, schedule.ReportName, schedule.DatasourceName, qs); filled {
		if ferr != nil {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, ferr.Error())
		}
		r.log(scheduleID, "DuckDB JDBC session attached")
		httpMethod = method
		contentType = ct
		reqBody = rdr
		if outQS != "" {
			requestURL = r.reportURL + "?" + outQS
		} else {
			requestURL = r.reportURL
		}
	}

	r.log(scheduleID, "Calling report execution")
	r.log(scheduleID, "Request URL="+requestURL)

	req, err := http.NewRequestWithContext(ctx, httpMethod, requestURL, reqBody)
	if err != nil {
		return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, err.Error())
	}
	req.Header.Set("X-JRS-Scheduled-Run", "1")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: r.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, err.Error())
	}
	r.log(scheduleID, fmt.Sprintf("Report HTTP status=%d bytes=%d content-type=%s", resp.StatusCode, len(respBody), resp.Header.Get("Content-Type")))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(string(respBody), " "))
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if snippet != "" {
			msg += ": " + snippet
		}
		return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, msg)
	}

	if err := validateReportBody(respBody, schedule.Format); err != nil {
		return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, err.Error())
	}

	if err := os.WriteFile(outputFile, respBody, 0o664); err != nil {
		return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, "Could not write output file: "+outputFile)
	}
	r.log(scheduleID, "Generated output file="+outputFile)

	if WantsEmail(*schedule) {
		r.log(scheduleID, "Sending email to="+schedule.Email)
		emailStatus = r.deliverEmail(ctx, *schedule, outputFile)
		if emailStatus == "no_recipients" || strings.HasPrefix(emailStatus, "failed") {
			return r.failFinish(ctx, scheduleID, executionID, outputFile, emailStatus, "Email delivery failed: "+emailStatus)
		}
		r.log(scheduleID, "Email success status="+emailStatus)
	} else {
		emailStatus = "skipped"
		r.log(scheduleID, "Email skipped (noemail or no recipients)")
	}

	if r.executions != nil && executionID > 0 {
		_ = r.executions.Finish(ctx, executionID, "success", outputFile, emailStatus, "")
	}
	r.log(scheduleID, fmt.Sprintf("DONE success execution_id=%d email_status=%s", executionID, emailStatus))

	return ExecuteResult{
		OK:          true,
		ExecutionID: executionID,
		OutputFile:  outputFile,
		EmailStatus: emailStatus,
	}
}

// prepareCassandraCollection fetches typed rows when the schedule datasource is
// Cassandra/Astra and returns a POST body with _collectionData for Tomcat.
func (r *Runner) prepareCassandraCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	if r == nil || r.Datasources == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	dsName := strings.TrimSpace(datasourceName)
	if dsName == "" {
		vals, _ := url.ParseQuery(rawQuery)
		dsName = strings.TrimSpace(vals.Get("_dataSource"))
	}
	if dsName == "" {
		return false, "", "", "", nil, nil
	}
	ds, ferr := r.Datasources.Find(ctx, dsName)
	if ferr != nil || ds == nil {
		return false, "", "", "", nil, nil
	}
	jrInfo := reports.LoadCollectionQueryInfoFromJRXMLFile(filepath.Join(r.Reports.JRXMLDir(), reportName+".jrxml"))
	if !ds.IsCassandra() && !jrInfo.IsCassandraCollection() {
		return false, "", "", "", nil, nil
	}
	if !ds.IsCassandra() && jrInfo.IsCassandraCollection() {
		return true, "", "", "", nil, fmt.Errorf(
			"datasource %q must be Database Type Apache Cassandra for this report (not JDBC/context.xml)",
			dsName,
		)
	}
	def, gerr := r.Reports.Get(reportName)
	if gerr != nil || def == nil {
		return true, "", "", "", nil, fmt.Errorf("Cassandra report definition not found")
	}
	cql := strings.TrimSpace(jrInfo.Query)
	if cql == "" {
		cql = strings.TrimSpace(def.QuerySQL())
	}
	if cql == "" {
		return true, "", "", "", nil, fmt.Errorf("Cassandra report has no CQL query")
	}
	params := jasper.ReportFillParams(rawQuery)
	rows, qerr := cassandra.FetchTypedRows(datasources.CassandraConfig(*ds), cql, params, 0)
	if qerr != nil {
		return true, "", "", "", nil, qerr
	}
	vals, _ := url.ParseQuery(rawQuery)
	if vals.Get("_dataSource") == "" {
		vals.Set("_dataSource", dsName)
		rawQuery = vals.Encode()
	}
	method, outQuery, contentType, body, aerr := jasper.AttachCollectionData(rawQuery, rows)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	return true, method, outQuery, contentType, body, nil
}

// prepareBigQueryCollection fetches typed rows when the schedule datasource is
// Google BigQuery and returns a POST body with _collectionData for Tomcat.
func (r *Runner) prepareBigQueryCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	if r == nil || r.Datasources == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	dsName := strings.TrimSpace(datasourceName)
	if dsName == "" {
		vals, _ := url.ParseQuery(rawQuery)
		dsName = strings.TrimSpace(vals.Get("_dataSource"))
	}
	if dsName == "" {
		return false, "", "", "", nil, nil
	}
	ds, ferr := r.Datasources.Find(ctx, dsName)
	if ferr != nil || ds == nil {
		return false, "", "", "", nil, nil
	}
	jrInfo := reports.LoadCollectionQueryInfoFromJRXMLFile(filepath.Join(r.Reports.JRXMLDir(), reportName+".jrxml"))
	if !ds.IsBigQuery() && !jrInfo.IsBigQueryCollection() {
		return false, "", "", "", nil, nil
	}
	if !ds.IsBigQuery() && jrInfo.IsBigQueryCollection() {
		return true, "", "", "", nil, fmt.Errorf(
			"datasource %q must be Database Type Google BigQuery for this report (not JDBC/context.xml)",
			dsName,
		)
	}
	def, gerr := r.Reports.Get(reportName)
	if gerr != nil || def == nil {
		return true, "", "", "", nil, fmt.Errorf("BigQuery report definition not found")
	}
	sql := strings.TrimSpace(jrInfo.Query)
	if sql == "" {
		sql = strings.TrimSpace(def.QuerySQL())
	}
	if sql == "" {
		return true, "", "", "", nil, fmt.Errorf("BigQuery report has no GoogleSQL query")
	}
	uuid := strings.TrimSpace(ds.CredentialsResourceUUID())
	if uuid == "" {
		return true, "", "", "", nil, fmt.Errorf("BigQuery datasource %q has no Service Account JSON resource configured", dsName)
	}
	if r.ResolveResourceBytes == nil {
		return true, "", "", "", nil, fmt.Errorf("content resources unavailable — cannot resolve BigQuery credentials")
	}
	credJSON, rerr := r.ResolveResourceBytes(ctx, uuid)
	if rerr != nil || len(credJSON) == 0 {
		return true, "", "", "", nil, fmt.Errorf("failed to load Service Account JSON resource for datasource %q", dsName)
	}
	params := jasper.ReportFillParams(rawQuery)
	rows, qerr := bigquery.FetchTypedRows(datasources.BigQueryConfig(*ds, credJSON), sql, params, 0)
	if qerr != nil {
		return true, "", "", "", nil, qerr
	}
	vals, _ := url.ParseQuery(rawQuery)
	if vals.Get("_dataSource") == "" {
		vals.Set("_dataSource", dsName)
		rawQuery = vals.Encode()
	}
	method, outQuery, contentType, body, aerr := jasper.AttachCollectionData(rawQuery, rows)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	return true, method, outQuery, contentType, body, nil
}

// prepareArcGISCollection fetches Feature Server attribute rows when the JRXML
// is an ArcGIS collection dataset and returns a POST body with _collectionData.
func (r *Runner) prepareArcGISCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	if r == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	jrxmlPath := filepath.Join(r.Reports.JRXMLDir(), reportName+".jrxml")
	jrInfo := reports.LoadCollectionQueryInfoFromJRXMLFile(jrxmlPath)
	if !jrInfo.IsArcGISCollection() {
		return false, "", "", "", nil, nil
	}
	if r.ArcGIS == nil {
		return true, "", "", "", nil, fmt.Errorf("ArcGIS connection catalog is unavailable")
	}
	def, gerr := r.Reports.Get(reportName)
	if gerr != nil || def == nil {
		return true, "", "", "", nil, fmt.Errorf("ArcGIS report definition not found")
	}
	name := arcgis.ResolveConnectionName(jrInfo.Connection, rawQuery, datasourceName)
	if name == "" {
		name = arcgis.ResolveConnectionName("", "", def.Datasource())
	}
	if name == "" {
		return true, "", "", "", nil, fmt.Errorf("ArcGIS report has no connection name (expected JRXML acugis.datasource.connection)")
	}
	cred, ferr := r.ArcGIS.FindArcGIS(ctx, name)
	if ferr != nil {
		if errors.Is(ferr, arcgis.ErrNotFound) {
			return true, "", "", "", nil, fmt.Errorf("ArcGIS connection %q was not found — add an ArcGIS Feature Server datasource with that name", name)
		}
		return true, "", "", "", nil, ferr
	}
	if strings.TrimSpace(cred.URL) == "" {
		return true, "", "", "", nil, fmt.Errorf("ArcGIS connection %q has no Feature Server URL", name)
	}
	sql := strings.TrimSpace(jrInfo.Query)
	if sql == "" {
		sql = strings.TrimSpace(def.QuerySQL())
	}
	if sql == "" {
		return true, "", "", "", nil, fmt.Errorf("ArcGIS report has no query (expected JRXML language=collection with acugis.datasource.provider=arcgis)")
	}
	params := arcgis.FillParams(rawQuery, jrxmlPath, def.Parameters())
	rows, qerr := arcgis.FetchTypedRows(cred, sql, params, 0)
	if qerr != nil {
		return true, "", "", "", nil, qerr
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	vals, _ := url.ParseQuery(rawQuery)
	if vals.Get("_dataSource") == "" {
		vals.Set("_dataSource", name)
		rawQuery = vals.Encode()
	}
	method, outQuery, contentType, body, aerr := jasper.AttachCollectionData(rawQuery, rows)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	return true, method, outQuery, contentType, body, nil
}

// prepareDuckDBJDBC attaches _duckdbSession when the schedule datasource is DuckDB
// so Tomcat opens DuckDB JDBC and fills via REPORT_CONNECTION.
func (r *Runner) prepareDuckDBJDBC(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	if r == nil || r.Datasources == nil || r.Reports == nil {
		return false, "", "", "", nil, nil
	}
	dsName := strings.TrimSpace(datasourceName)
	if dsName == "" {
		vals, _ := url.ParseQuery(rawQuery)
		dsName = strings.TrimSpace(vals.Get("_dataSource"))
	}
	if dsName == "" {
		return false, "", "", "", nil, nil
	}
	ds, ferr := r.Datasources.Find(ctx, dsName)
	if ferr != nil || ds == nil {
		return false, "", "", "", nil, nil
	}
	jrInfo := reports.LoadCollectionQueryInfoFromJRXMLFile(filepath.Join(r.Reports.JRXMLDir(), reportName+".jrxml"))
	if !ds.IsDuckDB() && !jrInfo.IsDuckDB() {
		return false, "", "", "", nil, nil
	}
	if !ds.IsDuckDB() && jrInfo.IsDuckDB() {
		return true, "", "", "", nil, fmt.Errorf(
			"datasource %q must be Database Type DuckDB for this report (DuckDB JDBC fill, not context.xml)",
			dsName,
		)
	}
	session, serr := duckdb.BuildJDBCSession(datasources.DuckDBConfig(*ds))
	if serr != nil {
		return true, "", "", "", nil, serr
	}
	vals, _ := url.ParseQuery(rawQuery)
	if vals.Get("_dataSource") == "" {
		vals.Set("_dataSource", dsName)
		rawQuery = vals.Encode()
	}
	method, outQuery, contentType, body, aerr := jasper.AttachDuckDBSession(rawQuery, session)
	if aerr != nil {
		return true, "", "", "", nil, aerr
	}
	return true, method, outQuery, contentType, body, nil
}

// prepareDuckDBCollection is a deprecated alias for prepareDuckDBJDBC.
func (r *Runner) prepareDuckDBCollection(ctx context.Context, reportName, datasourceName, rawQuery string) (ok bool, method, outQuery, contentType string, body io.Reader, err error) {
	return r.prepareDuckDBJDBC(ctx, reportName, datasourceName, rawQuery)
}

func (r *Runner) loadForRun(ctx context.Context, scheduleID int, mode string) (*Schedule, error) {
	schedule, err := r.repo.Get(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("Schedule not found")
	}
	if mode != "scheduled" {
		return schedule, nil
	}
	env := RuntimeEnv(r.paths, scheduleID)
	if len(env) == 0 {
		return schedule, nil
	}
	merged := ApplyRuntimeEnv(*schedule, env)
	return &merged, nil
}

func (r *Runner) deliverEmail(ctx context.Context, schedule Schedule, outputFile string) string {
	recipients := ParseRecipients(schedule.Email)
	if len(recipients) == 0 {
		return "no_recipients"
	}
	vars := map[string]string{
		"report_name": schedule.ReportName,
		"filename":    filepath.Base(outputFile),
		"run_date":    time.Now().Format("2006-01-02 15:04:05 MST"),
		"format":      schedule.Format,
	}
	subject := schedule.EmailSubject
	body := schedule.EmailBody
	bodyIsHTML := false
	if tmpl := strings.TrimSpace(schedule.EmailTemplate); tmpl != "" && r.templates != nil {
		if content, ok := r.templates.Load(tmpl); ok {
			body = content
			bodyIsHTML = r.templates.IsHTML(tmpl, content)
		}
	}
	if subject == "" {
		subject = "Report: {{report_name}}"
	}
	if body == "" {
		body = "Attached report {{report_name}} ({{format}}) generated {{run_date}}.\n"
	}
	subject = ApplyEmailPlaceholders(subject, vars)
	body = ApplyEmailPlaceholders(body, vars)
	if r.mail == nil {
		return "failed: SMTP not configured (set JRS_SMTP_HOST or install sendmail)"
	}
	result := r.mail.Send(ctx, recipients, subject, body, outputFile, schedule.Filename, bodyIsHTML)
	if result.OK {
		return "sent"
	}
	return "failed: " + result.Message
}

func (r *Runner) fail(scheduleID, executionID int, outputFile, emailStatus, msg string) ExecuteResult {
	r.log(scheduleID, "ERROR: "+msg)
	return ExecuteResult{OK: false, Message: msg, ExecutionID: executionID, OutputFile: outputFile, EmailStatus: emailStatus}
}

func (r *Runner) failFinish(ctx context.Context, scheduleID, executionID int, outputFile, emailStatus, msg string) ExecuteResult {
	r.log(scheduleID, "ERROR: "+msg)
	if r.executions != nil && executionID > 0 {
		_ = r.executions.Finish(ctx, executionID, "failed", outputFile, emailStatus, msg)
	}
	return ExecuteResult{OK: false, Message: msg, ExecutionID: executionID, OutputFile: outputFile, EmailStatus: emailStatus}
}

func (r *Runner) log(scheduleID int, message string) {
	_ = Mkdir(r.paths.LogDir)
	line := time.Now().Format("2006-01-02 15:04:05") + " " + message + "\n"
	f, err := os.OpenFile(r.paths.LogPath(scheduleID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o664)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line)
	_ = f.Close()
}

func (r *Runner) logContext(scheduleID int, schedule Schedule, mode string) {
	r.log(scheduleID, "Loaded env (mode="+mode+")")
	r.log(scheduleID, "EnvironmentFile="+ResolveEnvPath(r.paths, scheduleID))
	r.log(scheduleID, "Report name="+schedule.ReportName)
	r.log(scheduleID, "Datasource="+schedule.DatasourceName)
	r.log(scheduleID, "Format="+schedule.Format)
	if WantsEmail(schedule) {
		r.log(scheduleID, "Email recipients="+schedule.Email)
		if tmpl := strings.TrimSpace(schedule.EmailTemplate); tmpl != "" {
			r.log(scheduleID, "Email template="+tmpl)
		}
	} else {
		flag := "0"
		if schedule.NoEmail {
			flag = "1"
		}
		r.log(scheduleID, "Email=disabled (noemail="+flag+")")
	}
}

// WantsEmail reports whether email delivery should run.
func WantsEmail(schedule Schedule) bool {
	if schedule.NoEmail {
		return false
	}
	return strings.TrimSpace(schedule.Email) != ""
}

// ParseURLParams parses url_opt_params (PHP jrs_schedule_parse_url_params).
func ParseURLParams(raw string) map[string]string {
	params := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return params
	}
	if strings.Contains(raw, "=") {
		for _, pair := range regexp.MustCompile(`\s*,\s*`).Split(raw, -1) {
			if !strings.Contains(pair, "=") {
				continue
			}
			parts := strings.SplitN(pair, "=", 2)
			k := strings.TrimSpace(parts[0])
			if k != "" {
				params[k] = strings.TrimSpace(parts[1])
			}
		}
		return params
	}
	for _, token := range regexp.MustCompile(`\s*,\s*`).Split(raw, -1) {
		token = strings.TrimSpace(token)
		if token != "" {
			params[token] = ""
		}
	}
	return params
}

func validateReportBody(body []byte, format string) error {
	if len(body) == 0 {
		return fmt.Errorf("Report response body is empty")
	}
	format = strings.ToLower(format)
	if format == "pdf" && !strings.HasPrefix(string(body), "%PDF") {
		return fmt.Errorf("Report response is not a valid PDF (missing %%PDF header)")
	}
	if format == "xlsx" && !strings.HasPrefix(string(body), "PK\x03\x04") {
		return fmt.Errorf("Report response is not a valid XLSX archive")
	}
	return nil
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "report"
	}
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	return re.ReplaceAllString(name, "_")
}

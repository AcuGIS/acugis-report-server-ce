package schedules

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// Options bundles dependencies for server wiring.
type Options struct {
	Pool        *pgxpool.Pool
	Schema      string
	Paths       Paths
	Jasper      RunnerConfig
	Mail        MailConfig
	Systemd     SystemdOptions
	AppRoot     string
	Reports     *reports.Store
	Datasources datasources.Finder
	EmbeddedMap *embeddedmap.Service
}

// NewFromOptions constructs repositories and services from Options.
func NewFromOptions(opts Options) (
	repo *Repository,
	recipients *RecipientRepository,
	executions *ExecutionRepository,
	runner *Runner,
	systemd *SystemdManager,
	templates *EmailTemplateStore,
) {
	schema := opts.Schema
	if schema == "" {
		schema = "public"
	}
	repo = NewRepository(opts.Pool, schema)
	recipients = NewRecipientRepository(opts.Pool, schema)
	executions = NewExecutionRepository(opts.Pool, schema)
	templates = NewEmailTemplateStore(opts.Paths.EmailTmplDir)
	mailer := NewMailer(opts.Mail)
	runner = NewRunner(opts.Paths, repo, executions, mailer, templates, opts.Jasper)
	runner.Reports = opts.Reports
	runner.Datasources = opts.Datasources
	runner.EmbeddedMap = opts.EmbeddedMap
	systemdOpts := opts.Systemd
	defaults := DefaultSystemdOptions(opts.AppRoot)
	if systemdOpts.JasperHome == "" {
		systemdOpts.JasperHome = defaults.JasperHome
	}
	if systemdOpts.UnitDir == "" {
		systemdOpts.UnitDir = defaults.UnitDir
	}
	if systemdOpts.SystemDir == "" {
		systemdOpts.SystemDir = defaults.SystemDir
	}
	if systemdOpts.SystemctlBin == "" {
		systemdOpts.SystemctlBin = defaults.SystemctlBin
	}
	if systemdOpts.RunnerScript == "" {
		systemdOpts.RunnerScript = defaults.RunnerScript
	}
	if systemdOpts.PHPBinary == "" {
		systemdOpts.PHPBinary = defaults.PHPBinary
	}
	if systemdOpts.ServiceUser == "" {
		systemdOpts.ServiceUser = defaults.ServiceUser
	}
	if systemdOpts.GlobalEnvFile == "" {
		systemdOpts.GlobalEnvFile = defaults.GlobalEnvFile
	}
	// Bool zero-value cannot mean "unset". Re-read the env flag so a zero
	// SystemdOptions does not silently disable sudo on native installs.
	systemdOpts.UseSudo = envUseSudo()
	systemd = NewSystemdManager(opts.Paths, systemdOpts, repo)
	return repo, recipients, executions, runner, systemd, templates
}

// DefaultRunnerConfig builds Jasper runner settings.
func DefaultRunnerConfig(reportURL string, timeout time.Duration) RunnerConfig {
	return RunnerConfig{ReportURL: reportURL, Timeout: timeout}
}

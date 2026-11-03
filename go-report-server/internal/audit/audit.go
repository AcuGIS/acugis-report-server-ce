package audit

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

// Event is a single audit record. Handlers supply domain-specific fields;
// Record fills identity, request metadata, and timestamp when possible.
type Event struct {
	Category     string
	Action       string
	ObjectType   string
	ObjectID     string
	ObjectName   string
	Status       string
	DurationMS   *int64
	OutputFormat string
	OutputSize   *int64
	Details      map[string]any

	// Optional overrides (normally filled from request context).
	UserID    *int64
	Username  string
	SessionID string
	IPAddress string
	UserAgent string
	CreatedAt time.Time
	EventID   uuid.UUID
}

// Options configure the audit service.
type Options struct {
	Enabled        bool
	LogParameters  bool
	LogSQL         bool
	RetentionDays  int
	Logger         *slog.Logger
	CookieName     string // session cookie name for session_id hash prefix
}

// EventWriter persists audit events. *Repository implements this.
type EventWriter interface {
	Configured() bool
	Insert(ctx context.Context, e Event) error
}

// Service is the central audit recorder.
type Service struct {
	writer EventWriter
	repo   *Repository
	opts   Options
	logger *slog.Logger
}

// NewService constructs an audit service. A nil repository disables persistence
// but Record remains safe to call.
func NewService(repo *Repository, opts Options) *Service {
	return newService(repo, repo, opts)
}

// NewServiceWithWriter constructs a service with a custom writer (tests).
func NewServiceWithWriter(w EventWriter, opts Options) *Service {
	var repo *Repository
	if r, ok := w.(*Repository); ok {
		repo = r
	}
	return newService(w, repo, opts)
}

func newService(w EventWriter, repo *Repository, opts Options) *Service {
	if opts.RetentionDays <= 0 {
		opts.RetentionDays = 90
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{writer: w, repo: repo, opts: opts, logger: logger}
}

// Enabled reports whether audit recording is turned on.
func (s *Service) Enabled() bool {
	return s != nil && s.opts.Enabled
}

// LogParameters reports whether report parameters should be stored in details.
func (s *Service) LogParameters() bool {
	return s != nil && s.opts.LogParameters
}

// LogSQL reports whether raw SQL may be stored in details (default false).
func (s *Service) LogSQL() bool {
	return s != nil && s.opts.LogSQL
}

// RetentionDays returns the configured retention window.
func (s *Service) RetentionDays() int {
	if s == nil || s.opts.RetentionDays <= 0 {
		return 90
	}
	return s.opts.RetentionDays
}

// Repository exposes the underlying repository (for admin queries).
func (s *Service) Repository() *Repository {
	if s == nil {
		return nil
	}
	return s.repo
}

// Record persists an audit event. Failures are logged and never returned to callers.
func (s *Service) Record(ctx context.Context, e Event) {
	if s == nil || !s.opts.Enabled {
		return
	}
	if s.writer == nil || !s.writer.Configured() {
		return
	}
	if strings.TrimSpace(e.Category) == "" || strings.TrimSpace(e.Action) == "" {
		s.logger.Warn("audit: skipped event with empty category or action")
		return
	}
	if e.Status == "" {
		e.Status = StatusSuccess
	}
	if e.EventID == uuid.Nil {
		e.EventID = uuid.New()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	if e.Details != nil {
		e.Details = SanitizeDetailsMap(e.Details)
	}

	if err := s.writer.Insert(ctx, e); err != nil {
		s.logger.Error("audit: failed to record event",
			"error", err,
			"category", e.Category,
			"action", e.Action,
			"event_id", e.EventID.String(),
		)
	}
}

// RecordRequest fills identity and request metadata from r, then records the event.
func (s *Service) RecordRequest(r *http.Request, e Event) {
	if s == nil || !s.opts.Enabled || r == nil {
		return
	}
	s.fillFromRequest(r, &e)
	s.Record(r.Context(), e)
}

func (s *Service) fillFromRequest(r *http.Request, e *Event) {
	if e.IPAddress == "" {
		e.IPAddress = auth.ClientIPFromRequest(r)
	}
	if e.UserAgent == "" {
		e.UserAgent = r.UserAgent()
	}
	if e.SessionID == "" {
		e.SessionID = sessionIDFromRequest(r, s.opts.CookieName)
	}
	if e.UserID == nil || e.Username == "" {
		if u, ok := auth.UserFromContext(r.Context()); ok && u != nil {
			if e.UserID == nil && u.ID > 0 {
				id := u.ID
				e.UserID = &id
			}
			if e.Username == "" {
				e.Username = u.Username
			}
		}
	}
}

func sessionIDFromRequest(r *http.Request, cookieName string) string {
	if r == nil {
		return ""
	}
	name := strings.TrimSpace(cookieName)
	if name == "" {
		name = "jrs_session"
	}
	c, err := r.Cookie(name)
	if err != nil || c == nil || strings.TrimSpace(c.Value) == "" {
		return ""
	}
	hash, err := auth.HashSessionToken(c.Value)
	if err != nil {
		return ""
	}
	return auth.TokenHashPrefix(hash)
}

// DurationMS returns a pointer to milliseconds for Duration fields.
func DurationMS(d time.Duration) *int64 {
	ms := d.Milliseconds()
	return &ms
}

// Int64Ptr returns a pointer to n.
func Int64Ptr(n int64) *int64 {
	return &n
}

// ExportFormatAction maps an output format to execute vs export for audit actions.
func ExportFormatAction(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	switch f {
	case "", "html", "html2":
		return ActionReportExecute
	case "pdf", "xlsx", "xls", "csv", "docx", "doc", "odt", "ods", "pptx", "rtf", "xml", "json":
		return ActionReportExport
	default:
		return ActionReportExecute
	}
}

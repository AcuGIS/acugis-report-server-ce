package jasper

import (
	"log/slog"
	"time"
)

// RequestLog fields for structured gateway logging.
// Passwords, datasource credentials, and auth tokens must never be logged.
type RequestLog struct {
	RequestID      string
	RemoteIP       string
	User           string
	ReportName     string
	Format         string
	Method         string
	Path           string
	Status         int
	Bytes          int64
	Duration       time.Duration
	ErrorCode      string
	ErrorSummary   string
}

// LogRequest writes a structured info/error log for a gateway request.
func LogRequest(logger *slog.Logger, entry RequestLog) {
	if logger == nil {
		logger = slog.Default()
	}
	attrs := []any{
		slog.String("request_id", entry.RequestID),
		slog.String("remote_ip", entry.RemoteIP),
		slog.String("user", entry.User),
		slog.String("report_name", entry.ReportName),
		slog.String("format", entry.Format),
		slog.String("method", entry.Method),
		slog.String("path", entry.Path),
		slog.Int("status", entry.Status),
		slog.Int64("bytes", entry.Bytes),
		slog.Duration("duration", entry.Duration),
	}
	if entry.ErrorCode != "" {
		attrs = append(attrs,
			slog.String("error_code", entry.ErrorCode),
			slog.String("error_summary", entry.ErrorSummary),
		)
		logger.Error("jasper_gateway", attrs...)
		return
	}
	logger.Info("jasper_gateway", attrs...)
}

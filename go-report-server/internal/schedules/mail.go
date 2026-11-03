package schedules

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// MailConfig holds SMTP settings (PHP config smtp_* / JRS_SMTP_*).
type MailConfig struct {
	Host string
	Port int
	User string
	Pass string
	From string
	// Helo is the EHLO/HELO local name. Empty → derived from From domain.
	// Many providers reject Go's default "localhost" (RFC 2821).
	Helo string
}

// MailConfigFromEnv reads JRS_SMTP_* environment variables.
func MailConfigFromEnv() MailConfig {
	port := 587
	if raw := strings.TrimSpace(os.Getenv("JRS_SMTP_PORT")); raw != "" {
		if n, err := parseInt(raw); err == nil && n > 0 {
			port = n
		}
	}
	return MailConfig{
		Host: strings.TrimSpace(os.Getenv("JRS_SMTP_HOST")),
		Port: port,
		User: os.Getenv("JRS_SMTP_USER"),
		Pass: os.Getenv("JRS_SMTP_PASS"),
		From: strings.TrimSpace(os.Getenv("JRS_SMTP_FROM")),
		Helo: strings.TrimSpace(os.Getenv("JRS_SMTP_HELO")),
	}
}

// Mailer sends schedule emails with attachments.
type Mailer struct {
	cfg MailConfig
}

// NewMailer constructs a mailer.
// When Host is empty, Send falls back to local sendmail (PHP mail() parity).
func NewMailer(cfg MailConfig) *Mailer {
	if cfg.Port <= 0 {
		cfg.Port = 587
	}
	return &Mailer{cfg: cfg}
}

// Configured reports whether SMTP host is set (sendmail fallback may still work).
func (m *Mailer) Configured() bool {
	return m != nil && strings.TrimSpace(m.cfg.Host) != ""
}

// SendResult reports email delivery outcome.
type SendResult struct {
	OK      bool
	Message string
}

// Send delivers email to recipients with an attachment.
func (m *Mailer) Send(ctx context.Context, recipients []string, subject, body, attachmentPath, attachmentName string, bodyIsHTML bool) SendResult {
	recipients = filterRecipients(recipients)
	if len(recipients) == 0 {
		return SendResult{OK: false, Message: "No recipients"}
	}
	if _, err := os.Stat(attachmentPath); err != nil {
		return SendResult{OK: false, Message: "Attachment missing"}
	}
	if m == nil {
		return SendResult{OK: false, Message: "SMTP not configured"}
	}

	from := m.cfg.From
	if from == "" {
		if m.cfg.User != "" {
			from = m.cfg.User
		} else {
			from = "jasper-reports@localhost"
		}
	}

	msg, err := buildMIMEMessage(from, recipients, subject, body, attachmentPath, attachmentName, bodyIsHTML)
	if err != nil {
		return SendResult{OK: false, Message: err.Error()}
	}

	// PHP jrs_schedule_send_mail: use SMTP when host set, otherwise mail()/sendmail.
	if strings.TrimSpace(m.cfg.Host) == "" {
		return m.sendViaSendmail(ctx, msg)
	}

	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	helo := strings.TrimSpace(m.cfg.Helo)
	if helo == "" {
		helo = heloNameFromAddress(from)
	}
	if helo == "" {
		helo = strings.TrimSpace(m.cfg.Host)
	}
	var auth smtp.Auth
	if m.cfg.User != "" {
		auth = smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
	}
	if err := sendMailContext(ctx, addr, helo, auth, from, recipients, msg); err != nil {
		return SendResult{OK: false, Message: err.Error()}
	}
	return SendResult{OK: true, Message: "sent"}
}

func (m *Mailer) sendViaSendmail(ctx context.Context, msg []byte) SendResult {
	bin := findSendmail()
	if bin == "" {
		return SendResult{OK: false, Message: "SMTP not configured (set JRS_SMTP_HOST or install sendmail)"}
	}
	cmd := exec.CommandContext(ctx, bin, "-t", "-i")
	cmd.Stdin = bytes.NewReader(msg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return SendResult{OK: false, Message: "sendmail failed: " + detail}
		}
		return SendResult{OK: false, Message: "sendmail failed: " + err.Error()}
	}
	return SendResult{OK: true, Message: "sent"}
}

func findSendmail() string {
	if p := strings.TrimSpace(os.Getenv("JRS_SENDMAIL_PATH")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	for _, candidate := range []string{
		"/usr/sbin/sendmail",
		"/usr/lib/sendmail",
		"/sbin/sendmail",
	} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	if p, err := exec.LookPath("sendmail"); err == nil {
		return p
	}
	return ""
}

func buildMIMEMessage(from string, recipients []string, subject, body, attachmentPath, attachmentName string, bodyIsHTML bool) ([]byte, error) {
	boundary, err := randomBoundary()
	if err != nil {
		return nil, err
	}
	attachmentName = filepath.Base(attachmentName)
	if attachmentName == "" || attachmentName == "." {
		attachmentName = filepath.Base(attachmentPath)
	}

	var buf strings.Builder
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(recipients, ", ") + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n\r\n")

	contentType := "text/plain"
	if bodyIsHTML {
		contentType = "text/html"
	}
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: " + contentType + "; charset=utf-8\r\n\r\n")
	buf.WriteString(body + "\r\n")

	data, err := os.ReadFile(attachmentPath)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	chunked := chunkSplit(encoded, 76)

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: application/octet-stream; name=\"" + attachmentName + "\"\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString("Content-Disposition: attachment; filename=\"" + attachmentName + "\"\r\n\r\n")
	buf.WriteString(chunked + "\r\n")
	buf.WriteString("--" + boundary + "--\r\n")
	return []byte(buf.String()), nil
}

func sendMailContext(ctx context.Context, addr, helo string, auth smtp.Auth, from string, to []string, msg []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(host, portStr))
	if err != nil {
		return fmt.Errorf("SMTP connect failed: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	helo = strings.TrimSpace(helo)
	if helo == "" || strings.EqualFold(helo, "localhost") {
		helo = host
	}
	if err := client.Hello(helo); err != nil {
		return fmt.Errorf("SMTP HELO failed: %w", err)
	}

	// Submission ports usually require STARTTLS before AUTH.
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("SMTP STARTTLS failed: %w", err)
		}
	}

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("SMTP auth failed: %w", err)
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// heloNameFromAddress returns the domain part of an email address for EHLO.
func heloNameFromAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	at := strings.LastIndexByte(addr, '@')
	if at < 0 || at+1 >= len(addr) {
		return ""
	}
	domain := strings.TrimSpace(addr[at+1:])
	domain = strings.Trim(domain, "[]>")
	if domain == "" || strings.EqualFold(domain, "localhost") {
		return ""
	}
	return domain
}

// ParseRecipients splits and validates email addresses.
func ParseRecipients(emailField string) []string {
	parts := regexp.MustCompile(`[,;\s]+`).Split(emailField, -1)
	seen := map[string]struct{}{}
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || !strings.Contains(p, "@") {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func filterRecipients(recipients []string) []string {
	var out []string
	for _, r := range recipients {
		r = strings.TrimSpace(r)
		if r != "" {
			out = append(out, r)
		}
	}
	return out
}

func randomBoundary() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "jrs_" + fmt.Sprintf("%x", b), nil
}

func chunkSplit(s string, chunkLen int) string {
	if chunkLen <= 0 {
		return s
	}
	var parts []string
	for len(s) > chunkLen {
		parts = append(parts, s[:chunkLen])
		s = s[chunkLen:]
	}
	if s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\r\n")
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

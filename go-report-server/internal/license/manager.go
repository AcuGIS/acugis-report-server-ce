package license

import (
	"context"
	"crypto/ed25519"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	StatusNotActivated         = "not_activated"
	StatusActive               = "active"
	StatusExpired              = "expired"
	StatusInvalid              = "invalid"
	StatusWrongProduct         = "wrong_product"
	StatusInstallationMismatch = "installation_mismatch"
	StatusIdentityInvalid      = "identity_invalid"
	StatusVerifier             = "verifier"

	RefreshResultOK              = "ok"
	RefreshResultTransport       = "transport_error"
	RefreshResultLicenseInactive = "license_inactive"
	RefreshResultDenied          = "refresh_denied"
	RefreshResultExpired         = "license_expired"
	RefreshResultInvalid         = "invalid_replacement"
)

// Status is the admin-safe license view. It never includes the ACL1 token,
// activation key, signature, or public key.
type Status struct {
	InstalledPackageCapability string `json:"installed_package_capability"`
	EffectiveEdition           string `json:"effective_edition"`
	LicensePresent             bool   `json:"license_present"`
	LicenseValid               bool   `json:"license_valid"`
	ProductCode                string `json:"product_code,omitempty"`
	LicenseID                  string `json:"license_id,omitempty"`
	InstallationID             string `json:"installation_id,omitempty"`
	IssuedAt                   string `json:"issued_at,omitempty"`
	ExpiresAt                  string `json:"expires_at,omitempty"`
	LicenseType                string `json:"license_type,omitempty"`
	LastRefreshAt              string `json:"last_refresh_at,omitempty"`
	LastRefreshResult          string `json:"last_refresh_result,omitempty"`
	Status                     string `json:"status"`
	Message                    string `json:"message,omitempty"`
}

// Options configures a Manager. PublicKey is required for verification.
type Options struct {
	StateDir       string
	ServerURL      string
	ActivateURL    string
	RefreshURL     string
	TrialURL       string
	TrialStatusURL string
	ProductVersion string
	PublicKey      ed25519.PublicKey
	Now            func() time.Time
	Logger         *slog.Logger
	HTTP           *http.Client
	// ProductCode selects the ACL1 product. Empty means Report Server Professional.
	ProductCode string
	// LicenseFile is the token filename inside StateDir. Empty means license.lic.
	LicenseFile string
	// RequireExistingInstallation reads installation-id and does not create one.
	// Report Designer Web uses this so it cannot mint a second installation identity.
	RequireExistingInstallation bool
}

// Manager is the process-local entitlement source.
type Manager struct {
	mu                      sync.RWMutex
	dir                     string
	version                 string
	verifier                *Verifier
	client                  *Client
	clock                   func() time.Time
	logger                  *slog.Logger
	installationID          string
	identityErr             error
	token                   string
	verified                *VerifiedLicense
	identityLic             *VerifiedLicense
	status                  string
	message                 string
	lastRefreshAt           time.Time
	lastRefresh             string
	product                 string
	licenseFile             string
	requireExistingIdentity bool
}

// NewManager builds a manager. A missing public key fails closed to Community.
func NewManager(opts Options) (*Manager, error) {
	if len(opts.PublicKey) != ed25519.PublicKeySize {
		return nil, errVerifier()
	}
	dir := strings.TrimSpace(opts.StateDir)
	if dir == "" {
		dir = "/var/lib/acugis-report-server"
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		dir:     dir,
		version: strings.TrimSpace(opts.ProductVersion),
		verifier: &Verifier{
			PublicKey: opts.PublicKey,
		},
		client: &Client{
			ServerURL:           opts.ServerURL,
			ActivateEndpoint:    opts.ActivateURL,
			RefreshEndpoint:     opts.RefreshURL,
			TrialEndpoint:       opts.TrialURL,
			TrialStatusEndpoint: opts.TrialStatusURL,
			HTTP:                opts.HTTP,
		},
		clock:  opts.Now,
		logger: logger,
		status: StatusNotActivated,
	}
	m.product = strings.TrimSpace(opts.ProductCode)
	if m.product == "" {
		m.product = ProductReportServerProfessional
	}
	m.licenseFile = strings.TrimSpace(opts.LicenseFile)
	if m.licenseFile == "" {
		m.licenseFile = licenseFileName
	}
	m.requireExistingIdentity = opts.RequireExistingInstallation
	m.client.ProductCode = m.product
	m.verifier.Now = m.now
	return m, nil
}

// StateDir returns the durable license directory.
func (m *Manager) StateDir() string {
	if m == nil {
		return ""
	}
	return m.dir
}

func (m *Manager) now() time.Time {
	if m != nil && m.clock != nil {
		return m.clock().UTC()
	}
	return time.Now().UTC()
}

// Load reads the installation ID and any stored token. A missing token is
// not an error. A malformed installation ID is retained and reported.
func (m *Manager) Load() error {
	if m == nil {
		return errVerifier()
	}
	id, err := m.loadInstallationID()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.installationID = ""
	m.identityErr = nil
	if err != nil {
		m.identityErr = err
		m.token = ""
		m.verified = nil
		m.identityLic = nil
		if m.requireExistingIdentity && AsMissing(err) {
			m.status = StatusNotActivated
			m.message = "Report Server installation identity is not initialized."
			return nil
		}
		m.status = StatusIdentityInvalid
		m.message = "The installation identity file is invalid and was not replaced."
		return err
	}
	m.installationID = id
	token, rerr := ReadLicenseFile(m.dir, m.licenseFile)
	if rerr != nil {
		if AsMissing(rerr) {
			m.token = ""
			m.verified = nil
			m.identityLic = nil
			m.status = StatusNotActivated
			m.message = m.inactiveMessage()
			return nil
		}
		m.token = ""
		m.verified = nil
		m.identityLic = nil
		m.status = StatusInvalid
		m.message = "The stored license could not be read."
		return nil
	}
	m.token = token
	m.applyTokenLocked(token)
	return nil
}

func AsMissing(err error) bool {
	e, ok := As(err)
	return ok && e.Code == CodeMissing
}

func (m *Manager) applyTokenLocked(token string) {
	m.verified = nil
	m.identityLic = nil
	ident, err := ParseIdentity(token, m.verifier, m.installationID, "")
	if err != nil {
		m.setStatusFromErrorLocked(err, nil)
		return
	}
	m.identityLic = ident
	if ident.ProductCode != m.product {
		m.status = StatusWrongProduct
		m.message = m.wrongProductMessage()
		m.identityLic = ident
		return
	}
	if !m.now().Before(ident.ExpiresAt) {
		m.status = StatusExpired
		m.message = m.expiredMessageFor(ident)
		m.identityLic = ident
		return
	}
	m.verified = ident
	m.status = StatusActive
	m.message = ""
}

func (m *Manager) setStatusFromErrorLocked(err error, ident *VerifiedLicense) {
	e, ok := As(err)
	if !ok {
		m.status = StatusInvalid
		m.message = "The installed license is invalid."
		return
	}
	switch e.Code {
	case CodeExpired:
		m.status = StatusExpired
		m.message = "Professional features are currently unavailable."
		if ident != nil {
			m.identityLic = ident
		}
	case CodeWrongProduct:
		m.status = StatusWrongProduct
		m.message = "The installed license is not for AcuGIS Report Server Professional."
	case CodeMismatch:
		m.status = StatusInstallationMismatch
		m.message = "The installed license is for a different installation."
	case CodeSignature, CodeInvalid, CodeUnknownProduct:
		m.status = StatusInvalid
		m.message = "The installed license is invalid."
	default:
		m.status = StatusInvalid
		m.message = "The installed license is invalid."
	}
}

// ProfessionalEntitled reports whether this process may enable Professional.
// The OSS binary always returns false. Expiry is evaluated on each call.
func (m *Manager) ProfessionalEntitled() bool {
	if m == nil || !packageCapable || m.product != ProductReportServerProfessional {
		return false
	}
	m.mu.RLock()
	lic := m.verified
	m.mu.RUnlock()
	if lic == nil {
		return false
	}
	return m.now().Before(lic.ExpiresAt)
}

// Status returns the current admin-safe view.
// DesignerEntitled reports whether this installation may provide Report Designer Web.
// It does not enable Report Server Professional, and it does not consult packageCapable.
func (m *Manager) DesignerEntitled() bool {
	if m == nil || m.product != ProductReportDesignerWeb {
		return false
	}
	m.mu.RLock()
	lic := m.verified
	m.mu.RUnlock()
	if lic == nil || lic.ProductCode != ProductReportDesignerWeb {
		return false
	}
	return m.now().Before(lic.ExpiresAt)
}

func (m *Manager) Status() Status {
	if m == nil {
		return Status{InstalledPackageCapability: capabilityName(), EffectiveEdition: "community", Status: StatusVerifier}
	}
	if m.product == ProductReportDesignerWeb {
		return m.designerWebStatus()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	st := Status{
		InstalledPackageCapability: capabilityName(),
		EffectiveEdition:           "community",
		LicensePresent:             m.token != "",
		InstallationID:             m.installationID,
		Status:                     m.status,
		Message:                    m.message,
		LastRefreshResult:          m.lastRefresh,
	}
	if !m.lastRefreshAt.IsZero() {
		st.LastRefreshAt = m.lastRefreshAt.UTC().Format(time.RFC3339)
	}
	lic := m.verified
	if lic == nil {
		lic = m.identityLic
	}
	if lic != nil {
		st.ProductCode = lic.ProductCode
		st.LicenseID = lic.LicenseID
		st.IssuedAt = lic.IssuedAt.UTC().Format(time.RFC3339)
		st.ExpiresAt = lic.ExpiresAt.UTC().Format(time.RFC3339)
		st.LicenseType = lic.LicenseType
	}
	if m.identityErr != nil {
		st.Status = StatusIdentityInvalid
		st.Message = "The installation identity file is invalid and was not replaced."
	}
	if !packageCapable {
		st.EffectiveEdition = "community"
		st.LicenseValid = false
		if st.LicensePresent && m.verified != nil {
			st.Status = StatusInvalid
			st.Message = "This package cannot enable Report Server Professional."
		}
		return st
	}
	valid := m.verified != nil && m.now().Before(m.verified.ExpiresAt)
	st.LicenseValid = valid
	if valid {
		st.EffectiveEdition = "professional"
		st.Status = StatusActive
		if m.lastRefresh != "" && m.lastRefresh != RefreshResultOK {
			st.Message = "Last refresh failed: " + refreshReason(m.lastRefresh) + ". License remains valid until " + st.ExpiresAt + "."
		}
		return st
	}
	if m.verified != nil && !m.now().Before(m.verified.ExpiresAt) {
		st.EffectiveEdition = "community"
		st.Status = StatusExpired
		st.LicenseValid = false
		st.Message = m.expiredMessageFor(m.verified)
	}
	return st
}

func capabilityName() string {
	if packageCapable {
		return "professional"
	}
	return "community"
}

func refreshReason(code string) string {
	switch code {
	case RefreshResultTransport:
		return "licensing server unreachable"
	case RefreshResultLicenseInactive:
		return "license is not active"
	case RefreshResultDenied:
		return "refresh was denied"
	case RefreshResultExpired:
		return "license has expired"
	case RefreshResultInvalid:
		return "replacement license was rejected"
	default:
		return "refresh failed"
	}
}

// Activate exchanges a key for an ACL1, verifies it locally, then stores it.
// The activation key is not stored.
func (m *Manager) Activate(ctx context.Context, activationKey string) (Status, error) {
	if m == nil {
		return Status{}, errVerifier()
	}
	if m.product == ProductReportServerProfessional && !packageCapable {
		return m.Status(), &Error{Code: CodeVerifier, Message: "This package cannot enable Report Server Professional."}
	}
	m.mu.RLock()
	id := m.installationID
	identErr := m.identityErr
	m.mu.RUnlock()
	if identErr != nil || !ValidInstallationID(id) {
		if id == "" && identErr == nil {
			if err := m.Load(); err != nil {
				return m.Status(), err
			}
			m.mu.RLock()
			id = m.installationID
			identErr = m.identityErr
			m.mu.RUnlock()
		}
		if identErr != nil || !ValidInstallationID(id) {
			if AsMissing(identErr) {
				return m.Status(), identErr
			}
			return m.Status(), errIdentityInvalid()
		}
	}
	result, err := m.client.Activate(ctx, activationKey, id, m.version)
	if err != nil {
		return m.Status(), err
	}
	lic, err := m.verifyProduct(result.License, id)
	if err != nil {
		return m.Status(), err
	}
	if err := SaveVerifiedProductLicense(m.dir, m.licenseFile, result.License, m.verifier, id, m.product); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.token = strings.TrimSpace(result.License)
	m.verified = lic
	m.identityLic = lic
	m.status = StatusActive
	m.message = ""
	m.lastRefreshAt = m.now()
	m.lastRefresh = RefreshResultOK
	m.mu.Unlock()
	m.logger.Info("license: activation succeeded", "product", m.product, "license_id", lic.LicenseID, "expires", lic.ExpiresAt.UTC().Format(time.RFC3339))
	return m.Status(), nil
}

// Refresh asks the license service for a replacement token.
// A transport failure keeps a locally valid token. An invalid replacement
// is not saved.
func (m *Manager) Refresh(ctx context.Context) (Status, error) {
	if m == nil {
		return Status{}, errVerifier()
	}
	m.mu.RLock()
	token := m.token
	id := m.installationID
	m.mu.RUnlock()
	if strings.TrimSpace(token) == "" || !ValidInstallationID(id) {
		return m.Status(), errMissing()
	}
	ident, err := ParseIdentity(token, m.verifier, id, m.product)
	if err != nil {
		m.noteRefresh(RefreshResultInvalid, "")
		return m.Status(), err
	}
	if !WithinRefreshGrace(ident, m.now()) {
		m.noteRefresh(RefreshResultExpired, "")
		return m.Status(), errExpired()
	}
	result, err := m.client.Refresh(ctx, token, id)
	if err != nil {
		code := RefreshResultTransport
		var api *APIError
		if errors.As(err, &api) && api.Authoritative {
			switch api.Code {
			case "license_inactive":
				code = RefreshResultLicenseInactive
			case "refresh_denied":
				code = RefreshResultDenied
			case "license_expired":
				code = RefreshResultExpired
			default:
				code = api.Code
			}
		}
		m.noteRefresh(code, "")
		if IsTransport(err) && (m.ProfessionalEntitled() || m.DesignerEntitled()) {
			m.logger.Info("license: refresh failed; using locally valid entitlement", "until", m.Status().ExpiresAt)
		} else {
			m.logger.Info("license: refresh failed", "result", code)
		}
		return m.Status(), err
	}
	lic, err := m.verifyProduct(result.License, id)
	if err != nil {
		m.noteRefresh(RefreshResultInvalid, "")
		m.logger.Info("license: refresh replacement rejected")
		return m.Status(), err
	}
	if err := SaveVerifiedProductLicense(m.dir, m.licenseFile, result.License, m.verifier, id, m.product); err != nil {
		m.noteRefresh(RefreshResultInvalid, "")
		return m.Status(), err
	}
	m.mu.Lock()
	m.token = strings.TrimSpace(result.License)
	m.verified = lic
	m.identityLic = lic
	m.status = StatusActive
	m.message = ""
	m.lastRefreshAt = m.now()
	m.lastRefresh = RefreshResultOK
	m.mu.Unlock()
	m.logger.Info("license: refresh succeeded", "expires", lic.ExpiresAt.UTC().Format(time.RFC3339))
	return m.Status(), nil
}

func (m *Manager) noteRefresh(code, _ string) {
	m.mu.Lock()
	m.lastRefreshAt = m.now()
	m.lastRefresh = code
	m.mu.Unlock()
}

// LogStartup writes a safe entitlement line.
func (m *Manager) LogStartup() {
	if m == nil {
		return
	}
	if m.product == ProductReportDesignerWeb {
		st := m.Status()
		if st.LicenseValid {
			m.logger.Info("license: valid Report Designer Web entitlement", "expires", st.ExpiresAt)
			return
		}
		m.logger.Info("license: Report Designer Web is not licensed", "status", st.Status)
		return
	}
	st := m.Status()
	switch {
	case !packageCapable:
		m.logger.Info("license: open source package; effective edition=community")
	case st.LicenseValid:
		m.logger.Info("license: valid Professional entitlement", "expires", st.ExpiresAt)
	case st.Status == StatusExpired:
		m.logger.Info("license: Professional license expired; effective edition=community", "expires", st.ExpiresAt)
	case st.Status == StatusIdentityInvalid:
		m.logger.Error("license: installation identity is invalid; effective edition=community")
	default:
		m.logger.Info("license: no Professional license installed; effective edition=community", "status", st.Status)
	}
}

// RunRefreshLoop attempts one refresh, then repeats every RefreshInterval.
// It returns when ctx is cancelled. Startup must not wait for this call.
func (m *Manager) RunRefreshLoop(ctx context.Context) {
	if m == nil {
		return
	}
	m.refreshBestEffort(ctx)
	ticker := time.NewTicker(RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refreshBestEffort(ctx)
		}
	}
}

func (m *Manager) loadInstallationID() (string, error) {
	if m.requireExistingIdentity {
		return ReadInstallationID(m.dir)
	}
	return LoadOrCreateInstallationID(m.dir)
}

func (m *Manager) verifyProduct(token, installationID string) (*VerifiedLicense, error) {
	lic, err := ParseIdentity(token, m.verifier, installationID, m.product)
	if err != nil {
		return nil, err
	}
	if !m.now().Before(lic.ExpiresAt) {
		return nil, errExpired()
	}
	return lic, nil
}

func (m *Manager) inactiveMessage() string {
	if m.product == ProductReportDesignerWeb {
		return "Report Designer Web is not licensed."
	}
	return "Report Server Professional is not activated."
}

func (m *Manager) wrongProductMessage() string {
	if m.product == ProductReportDesignerWeb {
		return "The installed license is not for Report Designer Web."
	}
	return "The installed license is not for AcuGIS Report Server Professional."
}

func (m *Manager) expiredMessage() string {
	return m.expiredMessageFor(nil)
}

func (m *Manager) expiredMessageFor(lic *VerifiedLicense) string {
	if lic != nil && lic.LicenseType == LicenseTypeTrial {
		return "The trial has expired."
	}
	if m.product == ProductReportDesignerWeb {
		return "Report Designer Web is not licensed."
	}
	return "Professional features are currently unavailable."
}

// TrialOffer is the license-service view of whether a trial can be started.
// Checked is false when this process is not configured to ask the service.
type TrialOffer struct {
	Checked      bool
	TrialEnabled bool
	Eligible     bool
	Used         bool
	Status       string
	TrialDays    int
	ExpiresAt    string
}

// TrialOffer asks the license service. A transport failure is Status unknown
// and does not change a locally valid ACL1.
func (m *Manager) TrialOffer(ctx context.Context) TrialOffer {
	if m == nil {
		return TrialOffer{}
	}
	m.mu.RLock()
	id := m.installationID
	m.mu.RUnlock()
	if !ValidInstallationID(id) {
		return TrialOffer{}
	}
	st, ok, err := m.client.TrialStatus(ctx, id)
	if !ok {
		return TrialOffer{}
	}
	if err != nil {
		return TrialOffer{Checked: true, Status: "unknown"}
	}
	return TrialOffer{
		Checked:      true,
		TrialEnabled: st.TrialEnabled,
		Eligible:     st.Eligible,
		Used:         st.Used,
		Status:       st.Status,
		TrialDays:    st.TrialDays,
		ExpiresAt:    st.ExpiresAt,
	}
}

// StartTrial requests a normal ACL1 trial and stores it after local verification.
// The license service chooses the expiration. This does not bypass entitlement checks.
func (m *Manager) StartTrial(ctx context.Context) (Status, error) {
	if m == nil {
		return Status{}, errVerifier()
	}
	if m.product == ProductReportServerProfessional && !packageCapable {
		return m.Status(), &Error{Code: CodeVerifier, Message: "This package cannot enable Report Server Professional."}
	}
	m.mu.RLock()
	id := m.installationID
	identErr := m.identityErr
	m.mu.RUnlock()
	if identErr != nil || !ValidInstallationID(id) {
		if id == "" && identErr == nil {
			if err := m.Load(); err != nil {
				return m.Status(), err
			}
			m.mu.RLock()
			id = m.installationID
			identErr = m.identityErr
			m.mu.RUnlock()
		}
		if identErr != nil || !ValidInstallationID(id) {
			if AsMissing(identErr) {
				return m.Status(), identErr
			}
			return m.Status(), errIdentityInvalid()
		}
	}
	result, err := m.client.StartTrial(ctx, id)
	if err != nil {
		return m.Status(), err
	}
	lic, err := m.verifyProduct(result.License, id)
	if err != nil {
		return m.Status(), err
	}
	if err := SaveVerifiedProductLicense(m.dir, m.licenseFile, result.License, m.verifier, id, m.product); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.token = strings.TrimSpace(result.License)
	m.verified = lic
	m.identityLic = lic
	m.status = StatusActive
	m.message = ""
	m.lastRefreshAt = m.now()
	m.lastRefresh = RefreshResultOK
	m.mu.Unlock()
	m.logger.Info("license: trial started", "product", m.product, "license_id", lic.LicenseID, "expires", lic.ExpiresAt.UTC().Format(time.RFC3339))
	return m.Status(), nil
}

func (m *Manager) designerWebStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st := Status{
		InstalledPackageCapability: "report-designer-web",
		EffectiveEdition:           "not_licensed",
		LicensePresent:             m.token != "",
		InstallationID:             m.installationID,
		Status:                     m.status,
		Message:                    m.message,
		LastRefreshResult:          m.lastRefresh,
	}
	if !m.lastRefreshAt.IsZero() {
		st.LastRefreshAt = m.lastRefreshAt.UTC().Format(time.RFC3339)
	}
	lic := m.verified
	if lic == nil {
		lic = m.identityLic
	}
	if lic != nil {
		st.ProductCode = lic.ProductCode
		st.LicenseID = lic.LicenseID
		st.IssuedAt = lic.IssuedAt.UTC().Format(time.RFC3339)
		st.ExpiresAt = lic.ExpiresAt.UTC().Format(time.RFC3339)
		st.LicenseType = lic.LicenseType
	}
	if m.identityErr != nil && !AsMissing(m.identityErr) {
		st.Status = StatusIdentityInvalid
		st.Message = "The installation identity file is invalid and was not replaced."
		st.LicenseValid = false
		return st
	}
	valid := m.verified != nil && m.verified.ProductCode == ProductReportDesignerWeb && m.now().Before(m.verified.ExpiresAt)
	st.LicenseValid = valid
	if valid {
		st.EffectiveEdition = "licensed"
		st.Status = StatusActive
		if m.lastRefresh != "" && m.lastRefresh != RefreshResultOK {
			st.Message = "Last refresh failed: " + refreshReason(m.lastRefresh) + ". License remains valid until " + st.ExpiresAt + "."
		}
		return st
	}
	if m.verified != nil && !m.now().Before(m.verified.ExpiresAt) {
		st.EffectiveEdition = "not_licensed"
		st.Status = StatusExpired
		st.LicenseValid = false
		st.Message = m.expiredMessageFor(m.verified)
	}
	return st
}

func (m *Manager) refreshBestEffort(ctx context.Context) {
	m.mu.RLock()
	token := m.token
	m.mu.RUnlock()
	if strings.TrimSpace(token) == "" {
		return
	}
	_, _ = m.Refresh(ctx)
}

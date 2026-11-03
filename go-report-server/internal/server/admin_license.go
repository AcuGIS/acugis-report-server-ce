package server

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/license"
)

//go:embed admin_license.html
var adminLicenseHTML string

type designerLicenseView struct {
	Installed   bool
	Label       string
	Status      license.Status
	ShowKey     bool
	ShowRefresh bool
}

type trialView struct {
	ShowStart       bool
	ShowActive      bool
	ShowExpired     bool
	ShowUsed        bool
	ShowUnavailable bool
	DaysRemaining   int
	TrialDays       int
	ExpiresOn       string
	RemainingLabel  string
	ButtonLabel     string
}

type adminLicensePageData struct {
	materialChromeData
	SidebarNav       template.HTML
	LogoutURL        string
	Status           license.Status
	Designer         designerLicenseView
	ProTrial         trialView
	DesignerTrial    trialView
	CommunityPackage bool
	Message          string
	Error            string
	ShowKey          bool
	ShowRefresh      bool
}

var adminLicenseTemplate = template.Must(template.New("admin_license").Parse(adminLicenseHTML))

func (s *Server) handleAdminLicensePage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzHTML(w, r, err)
		return
	}
	data := adminLicensePageData{
		materialChromeData: s.materialChrome("License"),
		SidebarNav:         s.renderSidebarNavHTML("administration"),
		LogoutURL:          s.goURL("/logout"),
		Status:             s.licenseStatus(),
		Designer:           s.designerLicenseView(),
	}
	if r.Method == http.MethodPost {
		if !sameOriginPost(r) {
			data.Error = "The license request was rejected."
			s.fillTrialViews(r.Context(), &data)
			s.renderAdminLicense(w, data)
			return
		}
		if err := r.ParseForm(); err != nil {
			data.Error = "The license request was rejected."
			s.fillTrialViews(r.Context(), &data)
			s.renderAdminLicense(w, data)
			return
		}
		switch strings.TrimSpace(r.FormValue("action")) {
		case "activate":
			if s.license == nil {
				data.Error = "License activation is not available."
			} else if _, err := s.license.Activate(r.Context(), r.FormValue("activation_key")); err != nil {
				data.Error = safeLicenseErr(err)
			} else {
				data.Message = "Report Server Professional is active."
			}
		case "refresh":
			if s.license == nil {
				data.Error = "License refresh is not available."
			} else if _, err := s.license.Refresh(r.Context()); err != nil {
				data.Error = safeLicenseErr(err)
			} else {
				data.Message = "License refreshed."
			}
		case "activate_designer":
			if !s.designerConfigured() || s.designerLicense == nil {
				data.Error = "Report Designer Web is not installed."
			} else if _, err := s.designerLicense.Activate(r.Context(), r.FormValue("activation_key_designer")); err != nil {
				data.Error = safeDesignerLicenseErr(err)
			} else {
				data.Message = "Report Designer Web is licensed."
			}
		case "refresh_designer":
			if !s.designerConfigured() || s.designerLicense == nil {
				data.Error = "Report Designer Web is not installed."
			} else if _, err := s.designerLicense.Refresh(r.Context()); err != nil {
				data.Error = safeDesignerLicenseErr(err)
			} else {
				data.Message = "Report Designer Web license refreshed."
			}
		case "start_trial":
			if s.license == nil {
				data.Error = "License activation is not available."
			} else if s.licenseStatus().InstalledPackageCapability != "professional" {
				data.Error = "This package cannot enable Report Server Professional."
			} else if _, err := s.license.StartTrial(r.Context()); err != nil {
				data.Error = safeLicenseErr(err)
			} else {
				data.Message = "Report Server Professional trial is active."
			}
		case "start_trial_designer":
			if !s.designerConfigured() || s.designerLicense == nil {
				data.Error = "Report Designer Web is not installed."
			} else if _, err := s.designerLicense.StartTrial(r.Context()); err != nil {
				data.Error = safeDesignerLicenseErr(err)
			} else {
				data.Message = "Report Designer Web trial is active."
			}
		default:
			data.Error = "The license request was rejected."
		}
		data.Status = s.licenseStatus()
		data.Designer = s.designerLicenseView()
	}
	s.fillTrialViews(r.Context(), &data)
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data.ShowKey = data.Status.Status != license.StatusActive
	data.ShowRefresh = data.Status.LicensePresent
	s.renderAdminLicense(w, data)
}

func (s *Server) renderAdminLicense(w http.ResponseWriter, data adminLicensePageData) {
	data.ShowKey = data.Status.Status != license.StatusActive || data.Status.LicenseType == license.LicenseTypeTrial
	data.ShowRefresh = data.Status.LicensePresent
	if data.Designer.Installed {
		data.Designer.ShowKey = data.Designer.Status.Status != license.StatusActive || data.Designer.Status.LicenseType == license.LicenseTypeTrial
		data.Designer.ShowRefresh = data.Designer.Status.LicensePresent
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := adminLicenseTemplate.Execute(w, data); err != nil && s.logger != nil {
		s.logger.Error("render license page", "error", err)
	}
}

func (s *Server) handleAPIAdminLicenseStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireLicenseAdmin(w, r); !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, s.licenseStatus())
}

func (s *Server) handleAPIAdminLicenseActivate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireLicenseAdmin(w, r); !ok {
		return
	}
	if !sameOriginPost(r) {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "The license request was rejected."})
		return
	}
	if s.license == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "License activation is not available."})
		return
	}
	var body struct {
		ActivationKey string `json:"activation_key"`
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The license request was rejected."})
		return
	}
	if err := json.Unmarshal(raw, &body); err != nil || strings.TrimSpace(body.ActivationKey) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "An activation key is required."})
		return
	}
	st, err := s.license.Activate(r.Context(), body.ActivationKey)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": safeLicenseErr(err), "license": st})
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAPIAdminDesignerLicenseStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireLicenseAdmin(w, r); !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, s.designerLicenseView())
}

func (s *Server) handleAPIAdminDesignerLicenseActivate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireLicenseAdmin(w, r); !ok {
		return
	}
	if !sameOriginPost(r) {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "The license request was rejected."})
		return
	}
	if !s.designerConfigured() || s.designerLicense == nil {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "Report Designer Web is not installed."})
		return
	}
	var body struct {
		ActivationKey string `json:"activation_key"`
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The license request was rejected."})
		return
	}
	if err := json.Unmarshal(raw, &body); err != nil || strings.TrimSpace(body.ActivationKey) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "An activation key is required."})
		return
	}
	st, err := s.designerLicense.Activate(r.Context(), body.ActivationKey)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": safeDesignerLicenseErr(err), "license": st})
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAPIAdminDesignerLicenseRefresh(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireLicenseAdmin(w, r); !ok {
		return
	}
	if !sameOriginPost(r) {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "The license request was rejected."})
		return
	}
	if !s.designerConfigured() || s.designerLicense == nil {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "Report Designer Web is not installed."})
		return
	}
	st, err := s.designerLicense.Refresh(r.Context())
	if err != nil {
		code := http.StatusBadGateway
		if license.IsAuthoritative(err) {
			code = http.StatusConflict
		}
		s.writeJSON(w, code, map[string]any{"error": safeDesignerLicenseErr(err), "license": st})
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAPIAdminLicenseRefresh(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireLicenseAdmin(w, r); !ok {
		return
	}
	if !sameOriginPost(r) {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "The license request was rejected."})
		return
	}
	if s.license == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "License refresh is not available."})
		return
	}
	st, err := s.license.Refresh(r.Context())
	if err != nil {
		code := http.StatusBadGateway
		if license.IsAuthoritative(err) {
			code = http.StatusConflict
		}
		s.writeJSON(w, code, map[string]any{"error": safeLicenseErr(err), "license": st})
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAPIAdminLicenseTrial(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireLicenseAdmin(w, r); !ok {
		return
	}
	if !sameOriginPost(r) {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "The license request was rejected."})
		return
	}
	if !rejectTrialRequestBody(w, r, s) {
		return
	}
	if s.license == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "License activation is not available."})
		return
	}
	if s.licenseStatus().InstalledPackageCapability != "professional" {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "This package cannot enable Report Server Professional."})
		return
	}
	st, err := s.license.StartTrial(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": safeLicenseErr(err), "license": st})
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAPIAdminDesignerLicenseTrial(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireLicenseAdmin(w, r); !ok {
		return
	}
	if !sameOriginPost(r) {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "The license request was rejected."})
		return
	}
	if !rejectTrialRequestBody(w, r, s) {
		return
	}
	if !s.designerConfigured() || s.designerLicense == nil {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "Report Designer Web is not installed."})
		return
	}
	st, err := s.designerLicense.StartTrial(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": safeDesignerLicenseErr(err), "license": st})
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}

func rejectTrialRequestBody(w http.ResponseWriter, r *http.Request, s *Server) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The license request was rejected."})
		return false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body struct{}
	if err := dec.Decode(&body); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The license request was rejected."})
		return false
	}
	return true
}

func (s *Server) fillTrialViews(ctx context.Context, data *adminLicensePageData) {
	if data == nil {
		return
	}
	now := time.Now().UTC()
	data.CommunityPackage = s != nil && s.license != nil && data.Status.InstalledPackageCapability != "professional"
	if s != nil && s.license != nil && data.Status.InstalledPackageCapability == "professional" {
		data.ProTrial = composeTrialView(data.Status, s.license.TrialOffer(ctx), now)
	}
	if s != nil && s.designerConfigured() && s.designerLicense != nil {
		data.DesignerTrial = composeTrialView(data.Designer.Status, s.designerLicense.TrialOffer(ctx), now)
		if data.DesignerTrial.ShowActive {
			data.Designer.Label = "Trial active"
		}
		if data.DesignerTrial.ShowExpired {
			data.Designer.Label = "Trial expired"
		}
	}
}

func composeTrialView(local license.Status, offer license.TrialOffer, now time.Time) trialView {
	if local.Status == license.StatusIdentityInvalid {
		return trialView{}
	}
	if local.LicenseValid && local.LicenseType == license.LicenseTypeTrial {
		exp := parseTrialTime(local.ExpiresAt)
		return trialView{
			ShowActive:     true,
			DaysRemaining:  trialDaysRemaining(exp, now),
			ExpiresOn:      formatTrialDay(exp),
			RemainingLabel: trialRemainingLabel(exp, now),
		}
	}
	if local.LicenseValid {
		return trialView{}
	}
	if local.Status == license.StatusExpired && local.LicenseType == license.LicenseTypeTrial {
		return trialView{ShowExpired: true, ExpiresOn: formatTrialDay(parseTrialTime(local.ExpiresAt))}
	}
	if !offer.Checked {
		return trialView{}
	}
	switch offer.Status {
	case "available":
		if offer.Eligible && !offer.Used && offer.TrialDays > 0 {
			return trialView{ShowStart: true, TrialDays: offer.TrialDays, ButtonLabel: fmt.Sprintf("Start %d-Day Trial", offer.TrialDays)}
		}
	case "active":
		// The license service can reissue a still-active trial after a lost
		// response. There is no local token in this branch, so the same
		// Start Trial action installs that existing ACL1.
		days := offer.TrialDays
		if days <= 0 {
			days = 14
		}
		return trialView{ShowStart: true, TrialDays: days, ButtonLabel: fmt.Sprintf("Start %d-Day Trial", days)}
	case "expired":
		exp := parseTrialTime(offer.ExpiresAt)
		if exp.IsZero() {
			exp = parseTrialTime(local.ExpiresAt)
		}
		return trialView{ShowExpired: true, ExpiresOn: formatTrialDay(exp)}
	case "suspended", "revoked":
		return trialView{ShowUsed: true}
	case "unavailable", "unknown":
		return trialView{ShowUnavailable: true}
	}
	if offer.Used {
		return trialView{ShowUsed: true}
	}
	return trialView{}
}

func parseTrialTime(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func formatTrialDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("January 2, 2006")
}

func trialDaysRemaining(expires, now time.Time) int {
	if expires.IsZero() || !expires.After(now) {
		return 0
	}
	return int(expires.Sub(now) / (24 * time.Hour))
}

func trialRemainingLabel(expires, now time.Time) string {
	if expires.IsZero() || !expires.After(now) {
		return ""
	}
	if expires.Sub(now) < 24*time.Hour {
		return "Less than 1 day remaining"
	}
	days := int(expires.Sub(now) / (24 * time.Hour))
	if days == 1 {
		return "1 day remaining"
	}
	return fmt.Sprintf("%d days remaining", days)
}

func (s *Server) requireLicenseAdmin(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return nil, false
	}
	return user, true
}

func (s *Server) designerLicenseView() designerLicenseView {
	installed := s != nil && s.designerConfigured()
	st := license.Status{Status: license.StatusNotActivated, Message: "Report Designer Web is not installed."}
	if s != nil && s.designerLicense != nil {
		st = s.designerLicense.Status()
	}
	label := "Not installed"
	if !installed {
		st.Message = "Report Designer Web is not installed."
		st.LicenseID = ""
		st.ExpiresAt = ""
	}
	if installed {
		switch st.Status {
		case license.StatusActive:
			label = "Licensed"
		case license.StatusExpired:
			label = "Expired"
		case license.StatusNotActivated:
			label = "Not licensed"
		default:
			label = "Invalid license"
		}
	}
	return designerLicenseView{
		Installed:   installed,
		Label:       label,
		Status:      st,
		ShowKey:     installed && s != nil && s.designerLicense != nil && st.Status != license.StatusActive,
		ShowRefresh: installed && s != nil && s.designerLicense != nil && st.LicensePresent,
	}
}

func (s *Server) licenseStatus() license.Status {
	if s == nil || s.license == nil {
		return license.Status{
			InstalledPackageCapability: "community",
			EffectiveEdition:           string(s.Edition().CurrentEdition()),
			Status:                     license.StatusNotActivated,
			Message:                    "License activation is not available.",
		}
	}
	return s.license.Status()
}

func sameOriginPost(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}
	originHost := canonicalHost(u.Host, scheme)
	requestHost := canonicalHost(publicRequestHost(r), requestScheme(r))
	if originHost == "" || requestHost == "" {
		return false
	}
	return originHost == requestHost
}

func safeDesignerLicenseErr(err error) string {
	if e, ok := license.As(err); ok && e.Code == license.CodeWrongProduct {
		return "The license is not for Report Designer Web."
	}
	if e, ok := license.As(err); ok && e.Code == license.CodeMissing {
		return "Report Server installation identity is not initialized."
	}
	return safeLicenseErr(err)
}

func safeLicenseErr(err error) string {
	if err == nil {
		return ""
	}
	if api, ok := err.(*license.APIError); ok && api.Message != "" {
		return api.Message
	}
	if e, ok := license.As(err); ok {
		switch e.Code {
		case license.CodeWrongProduct:
			return "The license is not for AcuGIS Report Server Professional."
		case license.CodeMismatch:
			return "The license is for a different installation."
		case license.CodeSignature, license.CodeInvalid:
			return "The license could not be verified."
		case license.CodeExpired:
			return "The license has expired."
		case license.CodeIdentityInvalid:
			return "The installation identity file is invalid and was not replaced."
		case license.CodeVerifier:
			return e.Message
		default:
			return "The license request failed."
		}
	}
	if license.IsTransport(err) {
		return "The licensing server could not be reached."
	}
	return "The license request failed."
}

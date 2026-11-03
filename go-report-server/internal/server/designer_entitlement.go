package server

import "net/http"

var errDesignerNotLicensed = &httpError{
	Status:  http.StatusForbidden,
	Message: "Report Designer Web is not licensed for this installation.",
}

// designerEntitled is the single Web Designer entitlement check.
// A Report Server Professional token does not satisfy it.
func (s *Server) designerEntitled() bool {
	return s != nil && s.designerLicense != nil && s.designerLicense.DesignerEntitled()
}

// designerOffered is true only when the package is wired up and the entitlement is valid.
// Entry points use designerConfigured instead, so an installed but unlicensed
// Designer stays discoverable.
func (s *Server) designerOffered() bool {
	return s.designerConfigured() && s.designerEntitled()
}

func (s *Server) designerCreateURL() string {
	if s == nil || !s.designerConfigured() {
		return ""
	}
	return s.goURL("/reports/designer/new")
}

func (s *Server) designerImportURL() string {
	if s == nil || !s.designerConfigured() {
		return ""
	}
	return s.goURL("/reports/designer/import")
}

// allowDesignerRequest rejects a configured Designer that has no valid
// report-designer-web entitlement. Authentication is a separate gate and
// must already have succeeded. An unconfigured Designer keeps the existing
// unavailable path so a missing package does not become a licensed route.
// HTML administrators get the trial and activation page. Other HTML users
// are told to contact an administrator. API and JSON callers stay forbidden.
func (s *Server) allowDesignerRequest(w http.ResponseWriter, r *http.Request, asJSON bool) bool {
	if s == nil || !s.designerConfigured() || s.designerEntitled() {
		return true
	}
	if asJSON || wantsJSONResponse(r) {
		s.writeAuthzJSON(w, errDesignerNotLicensed)
		return false
	}
	s.renderDesignerLicenseGate(w, r)
	return false
}

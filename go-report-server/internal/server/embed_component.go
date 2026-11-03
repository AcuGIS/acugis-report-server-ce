package server

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

// embedComponentIDPattern is the Designer public id format.
// It is case-sensitive: values are not lowercased or otherwise rewritten.
var embedComponentIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type trustedEmbedComponentKey struct{}

func validEmbedComponentID(id string) bool {
	return embedComponentIDPattern.MatchString(id)
}

func withTrustedEmbedComponent(ctx context.Context, embedID string) context.Context {
	return context.WithValue(ctx, trustedEmbedComponentKey{}, embedID)
}

func trustedEmbedComponentID(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, _ := ctx.Value(trustedEmbedComponentKey{}).(string)
	if !validEmbedComponentID(id) {
		return "", false
	}
	return id, true
}

// handleEmbedReportComponent serves one filled component of an authorized report.
// The Embed ID is taken from the path. Query and token values named _componentId
// are removed by the shared content preparation and by rewriteEmbedExecute;
// handleDevReport then seals this path id and sets the Tomcat marker.
func (s *Server) handleEmbedReportComponent(w http.ResponseWriter, r *http.Request) {
	embedID := chi.URLParam(r, "embedId")
	exec, _, ok := s.prepareEmbedReportContentRequest(w, r)
	if !ok {
		return
	}
	if !validEmbedComponentID(embedID) {
		s.writeEmbedContentError(w, &httpError{Status: http.StatusBadRequest, Message: "INVALID_PARAMETER: malformed embed id"})
		return
	}
	exec = exec.WithContext(withTrustedEmbedComponent(exec.Context(), embedID))
	q := exec.URL.Query()
	q.Del("_componentId")
	q.Set("_componentId", embedID)
	exec.URL.RawQuery = q.Encode()

	bridge := newContentBridgeWriter(w)
	s.handleDevReport(bridge, exec)
	bridge.finishComponent(s.effectiveEmbedOrigins())
}

// sealEmbedComponentQuery forces the trusted path id onto the query that is
// about to be sent to Tomcat. Bound parameters and client values cannot remain.
func sealEmbedComponentQuery(raw, componentID string) string {
	q, err := url.ParseQuery(raw)
	if err != nil {
		q = url.Values{}
	}
	q.Del("_componentId")
	if componentID != "" {
		q.Set("_componentId", componentID)
	}
	return q.Encode()
}

// stripEmbedComponentQuery removes a client _componentId without re-encoding
// a query that does not carry that key.
func stripEmbedComponentQuery(raw string) string {
	if !strings.Contains(raw, "_componentId") {
		return raw
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	if _, ok := q["_componentId"]; !ok {
		return raw
	}
	q.Del("_componentId")
	return q.Encode()
}

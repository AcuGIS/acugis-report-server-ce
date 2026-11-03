package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

// handleEmbedReportContent serves the Jasper HTML report as the iframe
// document. It does not render viewer.html. Authentication is the existing
// emb_ token flow, which sets the HttpOnly jrs_embed_token cookie before the
// HTML is written so same-origin resources such as /report-image can load.
//
// The request path stays /embed/report/{key}/content. That keeps the embed
// route frame-ancestors policy (the external allow-list, no X-Frame-Options)
// and does not enter the /reports/execute framing path. Report identity and
// HTML format are pinned before handleDevReport, which runs the existing
// Jasper HTML pipeline and bound-parameter enforcement.
func (s *Server) handleEmbedReportContent(w http.ResponseWriter, r *http.Request) {
	exec, _, ok := s.prepareEmbedReportContentRequest(w, r)
	if !ok {
		return
	}
	bridge := newContentBridgeWriter(w)
	s.handleDevReport(bridge, exec)
	bridge.finish(s.effectiveEmbedOrigins())
}

// prepareEmbedReportContentRequest authenticates the emb_ token and builds the
// execution request shared by /content and /component. It pins the report
// identity and HTML format, resolves the catalog datasource, strips client
// datasource controls and _componentId, and applies blank-text normalization.
// Component mode adds its selector after this returns. rewriteEmbedExecute
// inside handleDevReport remains the authority for token-bound parameters.
func (s *Server) prepareEmbedReportContentRequest(w http.ResponseWriter, r *http.Request) (*http.Request, string, bool) {
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	_, user, err := s.requireEmbedToken(w, r, embedtoken.ContentReport, key)
	if err != nil {
		s.writeEmbedContentError(w, err)
		return nil, "", false
	}
	if !jasper.IsValidReportKey(key) {
		s.writeEmbedContentError(w, errReportInvalid)
		return nil, "", false
	}

	exec := r.Clone(auth.WithUser(r.Context(), user))
	q := exec.URL.Query()
	// The browser token stays on this route. /reports/execute is not taught
	// to accept et, and the token is not forwarded to Jasper.
	q.Del("et")
	// Datasource selection is resolved from the catalog, the same way the
	// Report Viewer renders it into buildUrl(). A content URL cannot choose
	// the JDBC resource, and the name is not placed on the emb_ token or in
	// the content bridge.
	q.Del("_dataSource")
	q.Del("datasource")
	q.Del("_componentId")
	q.Set("_repName", key)
	q.Set("_repFormat", "html")
	if ds := s.catalogDatasource(key); ds != "" {
		q.Set("_dataSource", ds)
	}
	// The Report Viewer form submits a present empty string for a blank text
	// parameter. Content mode has no form, so absent text parameters are
	// normalized here. Token-bound names are left untouched; rewriteEmbedExecute
	// inside handleDevReport remains the authority for those values.
	// Numeric, date, boolean, and other non-text parameters stay absent so
	// Jasper can apply its own defaults. Ordinary /reports/execute is unchanged.
	s.applyContentBlankTextParameters(key, q, embedParameters(exec))
	exec.URL.RawQuery = q.Encode()
	return exec, key, true
}

// applyContentBlankTextParameters submits declared text parameters that neither
// the request nor the embed token provided as present empty strings.
// A value already on the query is preserved, including an explicit empty string.
// A token-bound name is not written here, so a browser value cannot be blanked
// into a hole that bypasses rewriteEmbedExecute, and the bound value stays
// authoritative when that rewrite runs.
func (s *Server) applyContentBlankTextParameters(reportKey string, q url.Values, bound map[string][]string) {
	if s == nil || q == nil || s.deps.Reports == nil {
		return
	}
	def, err := s.deps.Reports.Get(reportKey)
	if err != nil || def == nil {
		return
	}
	for _, p := range def.Parameters() {
		name := strings.TrimSpace(stringify(p["name"]))
		if name == "" || isContentControlParameter(name) || !isTextReportParameter(p) {
			continue
		}
		if _, present := q[name]; present {
			continue
		}
		if _, boundPresent := bound[name]; boundPresent {
			continue
		}
		q.Set(name, "")
	}
}

// isTextReportParameter matches the Report Viewer text classification.
// Catalog parameter metadata uses type; a missing type is a string, the same
// default the viewer form uses. number, integer, date, boolean, select, and
// every other declared type stay non-text.
func isTextReportParameter(p map[string]any) bool {
	if p == nil {
		return false
	}
	typ := strings.ToLower(strings.TrimSpace(stringify(p["type"])))
	if typ == "" {
		typ = "string"
	}
	switch typ {
	case "string", "text", "java.lang.string":
		return true
	default:
		return false
	}
}

// isContentControlParameter keeps blank-text normalization off the values
// content mode already pins or strips. A declared string parameter with one
// of these names cannot put a browser-controlled or empty value back on the
// execution query.
func isContentControlParameter(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "et", "_datasource", "datasource", "_repname", "_repformat", "_componentid", "report":
		return true
	default:
		return false
	}
}

// catalogDatasource returns the report's configured datasource name.
// An empty result leaves _dataSource unset so collection and DuckDB
// preparers keep their existing behavior.
func (s *Server) catalogDatasource(reportKey string) string {
	if s == nil || s.deps.Reports == nil {
		return ""
	}
	def, err := s.deps.Reports.Get(reportKey)
	if err != nil || def == nil {
		return ""
	}
	return strings.TrimSpace(def.Datasource())
}

func (s *Server) writeEmbedContentError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "internal server error"
	var he *httpError
	if errors.As(err, &he) {
		status = he.Status
		msg = he.Message
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Del("Content-Length")
	w.WriteHeader(status)
	_, _ = w.Write(contentEmbedErrorHTML(msg, s.effectiveEmbedOrigins()))
}

// contentBridgeWriter buffers the Jasper response so a small host bridge can
// be added to HTML, and so execution errors can post an error event, without
// a second renderer.
type contentBridgeWriter struct {
	w      http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func newContentBridgeWriter(w http.ResponseWriter) *contentBridgeWriter {
	return &contentBridgeWriter{w: w}
}

func (b *contentBridgeWriter) Header() http.Header { return b.w.Header() }

func (b *contentBridgeWriter) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *contentBridgeWriter) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.buf.Write(p)
}

func (b *contentBridgeWriter) Flush() {}

func (b *contentBridgeWriter) finish(origins []string) {
	b.finishKind(origins, false)
}

// finishComponent injects the same host bridge as content mode, except the
// resize payload is the rendered component document. The Jasper HTML exporter
// wraps .jrPage in a width=100% centering table, so a document scroll
// measurement would report the iframe viewport once the iframe is larger
// than the page.
func (b *contentBridgeWriter) finishComponent(origins []string) {
	b.finishKind(origins, true)
}

func (b *contentBridgeWriter) finishKind(origins []string, component bool) {
	status := b.status
	if status == 0 {
		status = http.StatusOK
	}
	body := b.buf.Bytes()
	ct := strings.ToLower(b.Header().Get("Content-Type"))
	if status >= 200 && status < 300 && strings.Contains(ct, "text/html") {
		script := contentEmbedBridgeScript(origins, contentLoadScript)
		if component {
			script = componentEmbedBridgeScript(origins, contentLoadScript)
		}
		body = injectContentBridge(body, script)
		b.Header().Del("Content-Length")
	} else if status >= 400 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(status)
		}
		body = contentEmbedErrorHTML(msg, origins)
		b.Header().Set("Content-Type", "text/html; charset=utf-8")
		b.Header().Del("Content-Length")
	}
	b.w.WriteHeader(status)
	_, _ = b.w.Write(body)
}

func injectContentBridge(html []byte, script string) []byte {
	lower := bytes.ToLower(html)
	if i := bytes.LastIndex(lower, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(html)+len(script))
		out = append(out, html[:i]...)
		out = append(out, script...)
		out = append(out, html[i:]...)
		return out
	}
	out := make([]byte, 0, len(html)+len(script))
	out = append(out, html...)
	out = append(out, script...)
	return out
}

const contentLoadScript = `post("load",{});` +
	`if(typeof ResizeObserver!=="undefined"&&document.body){` +
	`new ResizeObserver(function(){publishSize();}).observe(document.body);}`

func contentEmbedErrorHTML(full string, origins []string) []byte {
	full = strings.TrimSpace(full)
	if full == "" {
		full = "internal server error"
	}
	code, text := splitEmbedError(full)
	codeJS, err := json.Marshal(code)
	if err != nil {
		codeJS = []byte(`"SERVER_ERROR"`)
	}
	textJS, err := json.Marshal(text)
	if err != nil {
		textJS = []byte(`"internal server error"`)
	}
	script := contentEmbedBridgeScript(origins, "post(\"error\",{code:"+string(codeJS)+",message:"+string(textJS)+"});")
	page := "<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body><pre>" +
		html.EscapeString(full) + "</pre>" + script + "</body></html>"
	return []byte(page)
}

func splitEmbedError(full string) (code, text string) {
	full = strings.TrimSpace(full)
	if i := strings.Index(full, ":"); i > 0 {
		code = strings.TrimSpace(full[:i])
		text = strings.TrimSpace(full[i+1:])
		switch code {
		case "AUTH_REQUIRED", "ACCESS_DENIED", "CONTENT_NOT_FOUND", "INVALID_PARAMETER",
			"SERVER_ERROR", "EMBED_ORIGIN_DENIED", "TOKEN_EXPIRED", "TOKEN_INVALID",
			"COMPONENT_NOT_FOUND", "COMPONENT_NOT_PRINTED", "COMPONENT_AMBIGUOUS",
			"COMPONENT_UNSUPPORTED":
			if text == "" {
				text = full
			}
			return code, text
		}
	}
	if full == "" {
		full = "internal server error"
	}
	return "SERVER_ERROR", full
}

const contentMeasureJS = `function measure(){var b=document.body,d=document.documentElement;return {width:window.innerWidth||(d?d.clientWidth:0),height:Math.max(b?b.scrollHeight:0,d?d.scrollHeight:0)};}`

const contentPublishJS = `function publishSize(){var box=measure();if(box.height>0)post("resize",box);}`

// componentMeasureJS reports the CSS pixels the iframe must have so the
// exported component document does not scroll.
//
// .jrPage getBoundingClientRect() is the rendered page, including the line
// box under an inline chart image. Style width and data-jr-height are the
// design box and are smaller than that rendered page.
//
// The exporter's outer table is width=100%, and its side cells contain
// &nbsp;. Those cells' used width tracks the viewport, so the measurement
// uses a Range around each spacer's contents. Horizontal body margin on the
// right is not added when that centering row is present: Chrome's scroll
// width is the left margin plus the page plus the spacer glyphs. Vertical
// body margins are both included. No padding constant is added.
//
// Math.ceil(n-1e-6) snaps binary float dust back to an integer and still
// rounds a real fraction up, because a fractional CSS pixel leaves a
// scrollbar. The deployed text field and chart sums are integers, so this
// does not add a pixel to them.
const componentMeasureJS = `function measure(){` +
	`var page=document.querySelector(".jrPage");` +
	`if(!page)return {width:0,height:0};` +
	`var rect=page.getBoundingClientRect();` +
	`var view=document.defaultView||window;` +
	`function box(el){var z={ml:0,mr:0,mt:0,mb:0,bl:0,br:0,bt:0,bb:0,pl:0,pr:0,pt:0,pb:0};if(!el)return z;var s=view.getComputedStyle(el);function n(p){var v=parseFloat(s[p]);return v>0?v:0;}z.ml=n("marginLeft");z.mr=n("marginRight");z.mt=n("marginTop");z.mb=n("marginBottom");z.bl=n("borderLeftWidth");z.br=n("borderRightWidth");z.bt=n("borderTopWidth");z.bb=n("borderBottomWidth");z.pl=n("paddingLeft");z.pr=n("paddingRight");z.pt=n("paddingTop");z.pb=n("paddingBottom");return z;}` +
	`var b=box(document.body),h=box(document.documentElement);` +
	`var width=rect.width+h.ml+h.bl+h.pl+b.ml+b.bl+b.pl+h.pr+h.br+b.pr+b.br;` +
	`var height=rect.height+h.mt+h.mb+h.bt+h.bb+h.pt+h.pb+b.mt+b.mb+b.bt+b.bb+b.pt+b.pb;` +
	`var center=page.parentNode;` +
	`while(center&&center.nodeType===1&&center.tagName!=="TD"&&center.tagName!=="TH")center=center.parentNode;` +
	`var centering=!!(center&&(center.tagName==="TD"||center.tagName==="TH")&&center.parentNode);` +
	`if(centering){var row=center.parentNode;for(var i=0;i<row.children.length;i++){var cell=row.children[i];if(cell===center)continue;var range=document.createRange();range.selectNodeContents(cell);width+=range.getBoundingClientRect().width;}}else{width+=h.mr+b.mr;}` +
	`if(!(width>0||height>0))return {width:0,height:0};` +
	`return {width:width>0?Math.ceil(width-1e-6):0,height:height>0?Math.ceil(height-1e-6):0};` +
	`}`

const componentPublishJS = `function publishSize(){var box=measure();if(box.width>0||box.height>0)post("resize",box);}`

// contentEmbedBridgeScript posts load, error, and resize to an allowed parent.
// It does not use a wildcard target and does not handle export, selection, or
// filter events. Report parameters are not reapplied here; the first render
// already went through the execute pipeline.
func contentEmbedBridgeScript(origins []string, deliverJS string) string {
	return embedBridgeScript(origins, deliverJS, contentMeasureJS, contentPublishJS)
}

func componentEmbedBridgeScript(origins []string, deliverJS string) string {
	return embedBridgeScript(origins, deliverJS, componentMeasureJS, componentPublishJS)
}

func embedBridgeScript(origins []string, deliverJS, measureJS, publishJS string) string {
	raw, err := json.Marshal(origins)
	if err != nil || len(raw) == 0 {
		raw = []byte("[]")
	}
	return `<script>(function(){` +
		`var allowed=` + string(raw) + `;` +
		`var parentOrigin="";` +
		`try{if(document.referrer)parentOrigin=new URL(document.referrer).origin;}catch(e){}` +
		`function originAllowed(origin){if(!origin)return false;for(var i=0;i<allowed.length;i++){if(allowed[i]===origin)return true;}return false;}` +
		`function target(){if(parentOrigin&&originAllowed(parentOrigin))return parentOrigin;if(allowed.length===1)return allowed[0];return "";}` +
		`function post(type,payload){var t=target();if(!t||!window.parent||window.parent===window)return;window.parent.postMessage({source:"acugis-report",version:1,type:type,payload:payload||{}},t);}` +
		measureJS +
		publishJS +
		`var sent=false;` +
		`function deliver(){if(!target())return;if(!sent){sent=true;` + deliverJS + `}publishSize();}` +
		`window.addEventListener("message",function(event){if(!originAllowed(event.origin)||event.source!==window.parent)return;var data=event.data;if(!data||data.source!=="acugis-report"||data.version!==1||typeof data.type!=="string")return;if(data.type==="configure"&&data.payload&&originAllowed(data.payload.hostOrigin)){parentOrigin=data.payload.hostOrigin;deliver();}else if(data.type==="resize"){publishSize();}});` +
		`window.addEventListener("load",function(){publishSize();});` +
		`deliver();` +
		`try{var u=new URL(location.href);if(u.searchParams.has("et")){u.searchParams.delete("et");history.replaceState({},"",u.pathname+u.search+u.hash);}}catch(e){}` +
		`})();</script>`
}

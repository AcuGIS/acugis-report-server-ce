package server

import (
	_ "embed"
	"html/template"
)

//go:embed viewer.html
var viewHTML string

type viewPageData struct {
	materialChromeData
	IsMap                  bool
	ReportJS               template.JS
	DatasourceJS           template.JS
	ReportEndpointJS       template.JS
	IsMapJS                template.JS
	CloseURL               string
	EmbedMode              bool
	EmbedAllowedOriginsJS  template.JS
	EmbedBoundParametersJS template.JS
	EmbedToolbar           bool
	EmbedParameters        bool
	EmbedExport            bool
	EmbedZoom              bool
	EmbedClose             bool
	EmbedAutoResize        bool
	ReportsBaseURL         string
	MapReportCSP           string
}

var viewTemplate = template.Must(template.New("view").Parse(viewHTML))

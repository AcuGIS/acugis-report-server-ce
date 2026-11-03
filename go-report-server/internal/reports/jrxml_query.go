package reports

import (
	"encoding/xml"
	"io"
	"os"
	"strings"
)

// CollectionQueryInfo is the collection-fill query extracted from JRXML.
type CollectionQueryInfo struct {
	Query          string
	Language       string // cassandra | collection | cql | sql | ""
	Provider       string // acugis.datasource.provider when set
	Connection     string // acugis.datasource.connection when set
	Resource       string // acugis.datasource.resource logical path
	Delimiter      string // acugis.datasource.delimiter for CSV
	JSONSource     string // acugis.datasource.json.source file|url
	JSONURL        string // acugis.datasource.json.url
	JSONCollection string // acugis.datasource.json.collection
	ExcelSheet     string // acugis.datasource.excel.sheet
	XMLCollection  string // acugis.datasource.xml.collection
}

// IsCassandraCollection reports whether the JRXML is a Designer Cassandra/collection dataset.
func (i CollectionQueryInfo) IsCassandraCollection() bool {
	lang := strings.ToLower(strings.TrimSpace(i.Language))
	if lang == "cassandra" || lang == "cql" {
		return true
	}
	prov := strings.ToLower(strings.TrimSpace(i.Provider))
	if prov == "cassandra" {
		return true
	}
	return lang == "collection" && prov == "cassandra"
}

// IsBigQueryCollection reports whether the JRXML is a Designer BigQuery/collection dataset.
func (i CollectionQueryInfo) IsBigQueryCollection() bool {
	lang := strings.ToLower(strings.TrimSpace(i.Language))
	prov := strings.ToLower(strings.TrimSpace(i.Provider))
	if prov == "bigquery" {
		return true
	}
	// Legacy mistaken language="bigquery" (Jasper has no such executer).
	if lang == "bigquery" {
		return true
	}
	return lang == "collection" && prov == "bigquery"
}

// IsDuckDB reports whether the JRXML is a DuckDB report (JDBC fill via REPORT_CONNECTION).
// Name retained historically as collection detection; DuckDB no longer uses collection fill.
func (i CollectionQueryInfo) IsDuckDBCollection() bool {
	return i.IsDuckDB()
}

// IsDuckDB reports whether the JRXML targets a DuckDB datasource / query language.
func (i CollectionQueryInfo) IsDuckDB() bool {
	lang := strings.ToLower(strings.TrimSpace(i.Language))
	prov := strings.ToLower(strings.TrimSpace(i.Provider))
	if prov == "duckdb" {
		return true
	}
	if lang == "duckdb" {
		return true
	}
	return lang == "collection" && prov == "duckdb"
}

// IsArcGISCollection reports whether the JRXML is a Designer ArcGIS Feature Server dataset.
// Designer writes language="collection" plus acugis.datasource.provider=arcgis.
// A collection query with no provider is the legacy ArcGIS form.
func (i CollectionQueryInfo) IsArcGISCollection() bool {
	lang := strings.ToLower(strings.TrimSpace(i.Language))
	prov := strings.ToLower(strings.TrimSpace(i.Provider))
	switch prov {
	case "arcgis", "arcgis_feature_server", "feature_server":
		return true
	}
	return lang == "collection" && prov == ""
}

// IsGeoServerCollection reports whether the JRXML is a Designer GeoServer WFS dataset.
// Designer writes language="collection" plus acugis.datasource.provider=geoserver.
func (i CollectionQueryInfo) IsGeoServerCollection() bool {
	lang := strings.ToLower(strings.TrimSpace(i.Language))
	prov := strings.ToLower(strings.TrimSpace(i.Provider))
	if prov == "geoserver" {
		return true
	}
	return lang == "collection" && prov == "geoserver"
}

// IsCSVCollection reports whether the JRXML is a report-owned CSV dataset.
// Provider csv is enough even when the query language was not recorded, so the
// report cannot fall through to JDBC.
func (i CollectionQueryInfo) IsCSVCollection() bool {
	return strings.EqualFold(strings.TrimSpace(i.Provider), "csv")
}

// IsJSONCollection reports whether the JRXML is a JSON document dataset.
// Provider json is enough even when the query language was not recorded, so the
// report cannot fall through to JDBC.
func (i CollectionQueryInfo) IsJSONCollection() bool {
	return strings.EqualFold(strings.TrimSpace(i.Provider), "json")
}

// IsExcelCollection reports whether the JRXML is an Excel workbook dataset.
// Provider excel is enough even when the query language was not recorded, so the
// report cannot fall through to JDBC.
func (i CollectionQueryInfo) IsExcelCollection() bool {
	return strings.EqualFold(strings.TrimSpace(i.Provider), "excel")
}

// IsXMLCollection reports whether the JRXML is an XML document dataset.
// Provider xml is enough even when the query language was not recorded, so the
// report cannot fall through to JDBC.
func (i CollectionQueryInfo) IsXMLCollection() bool {
	return strings.EqualFold(strings.TrimSpace(i.Provider), "xml")
}

// JSONURLMode reports whether the JSON document is fetched at execution time.
func (i CollectionQueryInfo) JSONURLMode() bool {
	if !i.IsJSONCollection() {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(i.JSONSource), "url") {
		return true
	}
	return strings.TrimSpace(i.JSONURL) != "" && strings.TrimSpace(i.Resource) == ""
}

// IsCollectionFill reports whether the JRXML expects Go collection fill (not JDBC).
// DuckDB uses direct JDBC REPORT_CONNECTION and is intentionally excluded.
func (i CollectionQueryInfo) IsCollectionFill() bool {
	return i.IsCassandraCollection() || i.IsBigQueryCollection() || i.IsArcGISCollection() || i.IsGeoServerCollection() || i.IsCSVCollection() || i.IsJSONCollection() || i.IsExcelCollection() || i.IsXMLCollection()
}

// ExtractCollectionQueryInfo reads query language + text + provider from JRXML.
func ExtractCollectionQueryInfo(jrxml []byte) CollectionQueryInfo {
	var info CollectionQueryInfo
	if len(jrxml) == 0 {
		return info
	}
	dec := xml.NewDecoder(strings.NewReader(string(jrxml)))
	var fallbackLang, fallbackText string
	foundCollection := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "property":
			var name, value string
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "name":
					name = a.Value
				case "value":
					value = a.Value
				}
			}
			switch name {
			case "acugis.collectionQuery":
				if strings.TrimSpace(value) != "" && info.Query == "" {
					info.Query = strings.TrimSpace(value)
				}
			case "acugis.datasource.provider":
				if v := strings.TrimSpace(value); v != "" {
					info.Provider = v
				}
			case "acugis.datasource.connection":
				if v := strings.TrimSpace(value); v != "" {
					info.Connection = v
				}
			case "acugis.datasource.resource":
				if v := strings.TrimSpace(value); v != "" {
					info.Resource = v
				}
			case "acugis.datasource.delimiter":
				if v := strings.TrimSpace(value); v != "" {
					info.Delimiter = v
				}
			case "acugis.datasource.json.source":
				if v := strings.TrimSpace(value); v != "" {
					info.JSONSource = v
				}
			case "acugis.datasource.json.url":
				if v := strings.TrimSpace(value); v != "" {
					info.JSONURL = v
				}
			case "acugis.datasource.json.collection":
				if v := strings.TrimSpace(value); v != "" {
					info.JSONCollection = v
				}
			case "acugis.datasource.excel.sheet":
				if v := strings.TrimSpace(value); v != "" {
					info.ExcelSheet = v
				}
			case "acugis.datasource.xml.collection":
				if v := strings.TrimSpace(value); v != "" {
					info.XMLCollection = v
				}
			}
		case "queryString", "query":
			// Digester JRXML uses <queryString>; JasperReports 7 (Designer) uses <query>.
			lang := "SQL"
			for _, a := range se.Attr {
				if a.Name.Local == "language" {
					lang = a.Value
				}
			}
			var cdata struct {
				Text string `xml:",chardata"`
			}
			if err := dec.DecodeElement(&cdata, &se); err != nil {
				continue
			}
			text := strings.TrimSpace(cdata.Text)
			if text == "" {
				continue
			}
			l := strings.ToLower(strings.TrimSpace(lang))
			if l == "cassandra" || l == "collection" || l == "cql" || l == "bigquery" || l == "duckdb" {
				if !foundCollection {
					info.Language = l
					info.Query = text
					foundCollection = true
				}
				continue
			}
			if fallbackText == "" {
				fallbackLang, fallbackText = lang, text
			}
		}
	}
	if !foundCollection && info.Query == "" && fallbackText != "" {
		info.Language = fallbackLang
		info.Query = fallbackText
	}
	return info
}

// ExtractCollectionQueryFromJRXML reads CQL/collection query text from JRXML.
// Designer embeds Cassandra queries as JR7 <query language="cassandra"> (or classic
// Digester <queryString language="cassandra|collection">) and may also set
// property acugis.collectionQuery.
func ExtractCollectionQueryFromJRXML(jrxml []byte) string {
	return ExtractCollectionQueryInfo(jrxml).Query
}

// LoadCollectionQueryFromJRXMLFile reads a JRXML file and extracts the collection query.
func LoadCollectionQueryFromJRXMLFile(path string) string {
	return LoadCollectionQueryInfoFromJRXMLFile(path).Query
}

// LoadCollectionQueryInfoFromJRXMLFile reads JRXML collection metadata from disk.
func LoadCollectionQueryInfoFromJRXMLFile(path string) CollectionQueryInfo {
	path = strings.TrimSpace(path)
	if path == "" {
		return CollectionQueryInfo{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CollectionQueryInfo{}
	}
	return ExtractCollectionQueryInfo(data)
}

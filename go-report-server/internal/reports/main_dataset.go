package reports

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

// DatasetRoleMain is the only dataset role profiled in Dataset Intelligence V2.
// Named subdatasets can be added later as additional roles; they are not extracted here.
const DatasetRoleMain = "main"

// MainField is one field declared on the report main dataset.
type MainField struct {
	Name  string
	Class string
}

// MainParameter is one parameter declared on the report main dataset.
type MainParameter struct {
	Name         string
	Class        string
	NestedType   string
	ForPrompting bool
	DefaultExpr  string
}

// MainDataset is the report main dataset definition taken from JRXML.
// Query text is read for profiling and display. It is not an editable copy.
type MainDataset struct {
	Query      string
	Language   string
	Provider   string
	Fields     []MainField
	Parameters []MainParameter
}

// HasQuery reports whether the main dataset has an executable query text.
func (m MainDataset) HasQuery() bool {
	return strings.TrimSpace(m.Query) != ""
}

var reDefaultExpr = regexp.MustCompile(`(?is)<(?:[\w.-]+:)?defaultValueExpression\b[^>]*>(?:\s*<!\[CDATA\[(.*?)\]\]>|([^<]*))</(?:[\w.-]+:)?defaultValueExpression>`)

// ExtractMainDataset reads the jasperReport main dataset only.
// It does not descend into subDataset, datasetRun, or subreport, and it does
// not use the recursive query/parameter helpers that can mix those in.
func ExtractMainDataset(jrxml []byte) (MainDataset, error) {
	var md MainDataset
	if len(bytes.TrimSpace(jrxml)) == 0 {
		return md, nil
	}
	dec := xml.NewDecoder(bytes.NewReader(jrxml))
	var stack []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return md, err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			local := el.Name.Local
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if parent == "jasperReport" && isNestedDatasetElement(local) {
				if err := skipElement(dec); err != nil {
					return md, err
				}
				continue
			}
			if parent == "jasperReport" {
				switch local {
				case "query", "queryString":
					text, lang, err := decodeQueryElement(dec, el)
					if err != nil {
						return md, err
					}
					if md.Query == "" && strings.TrimSpace(text) != "" {
						md.Query = strings.TrimSpace(text)
						md.Language = strings.TrimSpace(lang)
					}
					continue
				case "parameter":
					p, err := decodeMainParameter(dec, el)
					if err != nil {
						return md, err
					}
					if p.Name != "" {
						md.Parameters = append(md.Parameters, p)
					}
					continue
				case "field":
					name := strings.TrimSpace(attrValue(el, "name"))
					if name != "" {
						class := strings.TrimSpace(attrValue(el, "class"))
						if class == "" {
							class = "java.lang.String"
						}
						md.Fields = append(md.Fields, MainField{Name: name, Class: class})
					}
					if err := skipElement(dec); err != nil {
						return md, err
					}
					continue
				case "property":
					if attrValue(el, "name") == "acugis.datasource.provider" {
						if v := strings.TrimSpace(attrValue(el, "value")); v != "" {
							md.Provider = v
						}
					}
					if err := skipElement(dec); err != nil {
						return md, err
					}
					continue
				}
			}
			stack = append(stack, local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if md.Query != "" && strings.TrimSpace(md.Language) == "" {
		md.Language = "SQL"
	}
	return md, nil
}

func isNestedDatasetElement(local string) bool {
	switch local {
	case "subDataset", "datasetRun", "subreport":
		return true
	default:
		return false
	}
}

func attrValue(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// skipElement consumes the remainder of an element whose start token was already read.
func skipElement(dec *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

func decodeQueryElement(dec *xml.Decoder, start xml.StartElement) (string, string, error) {
	var box struct {
		Text string `xml:",chardata"`
	}
	if err := dec.DecodeElement(&box, &start); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(box.Text), attrValue(start, "language"), nil
}

func decodeMainParameter(dec *xml.Decoder, start xml.StartElement) (MainParameter, error) {
	var box struct {
		Inner string `xml:",innerxml"`
	}
	if err := dec.DecodeElement(&box, &start); err != nil {
		return MainParameter{}, err
	}
	name := strings.TrimSpace(attrValue(start, "name"))
	class := strings.TrimSpace(attrValue(start, "class"))
	if class == "" {
		class = "java.lang.String"
	}
	prompt := true
	if v := strings.TrimSpace(attrValue(start, "isForPrompting")); strings.EqualFold(v, "false") {
		prompt = false
	}
	return MainParameter{
		Name:         name,
		Class:        class,
		NestedType:   strings.TrimSpace(attrValue(start, "nestedType")),
		ForPrompting: prompt,
		DefaultExpr:  defaultExprText(box.Inner),
	}, nil
}

func defaultExprText(inner string) string {
	m := reDefaultExpr.FindStringSubmatch(inner)
	if len(m) < 3 {
		return ""
	}
	text := m[1]
	if strings.TrimSpace(text) == "" {
		text = m[2]
	}
	return strings.TrimSpace(text)
}

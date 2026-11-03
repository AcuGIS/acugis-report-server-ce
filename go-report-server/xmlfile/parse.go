package xmlfile

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type qname struct {
	Space string
	Local string
}

type piece struct {
	text  string
	child *element
}

type element struct {
	name   qname
	attrs  []xmlAttr
	pieces []piece
}

type xmlAttr struct {
	name  qname
	value string
}

func (e *element) children() []*element {
	if e == nil {
		return nil
	}
	out := make([]*element, 0, len(e.pieces))
	for _, p := range e.pieces {
		if p.child != nil {
			out = append(out, p.child)
		}
	}
	return out
}

func (e *element) directText() string {
	var b strings.Builder
	for _, p := range e.pieces {
		if p.child == nil {
			b.WriteString(p.text)
		}
	}
	return b.String()
}

func (e *element) allText() string {
	var b strings.Builder
	var walk func(*element)
	walk = func(n *element) {
		if n == nil {
			return
		}
		for _, p := range n.pieces {
			if p.child != nil {
				walk(p.child)
				continue
			}
			b.WriteString(p.text)
		}
	}
	walk(e)
	return b.String()
}

func (e *element) mixed() bool {
	if e == nil || len(e.children()) == 0 {
		return false
	}
	for _, p := range e.pieces {
		if p.child == nil && strings.TrimSpace(p.text) != "" {
			return true
		}
	}
	return false
}

func (e *element) scalarEmpty() bool {
	return e.directText() == ""
}

// Parse reads one XML document and discovers record collections.
// The reader is capped at MaxBytes. External entities, DTDs, and entity
// declarations are rejected and are never fetched.
func Parse(r io.Reader) (*Document, error) {
	if r == nil {
		return nil, fmt.Errorf("XML document is not valid")
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("XML document is not valid")
	}
	data, err = normalize(data)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("XML document is not valid")
	}
	root, err := parseTree(data)
	if err != nil {
		return nil, err
	}
	return discover(root)
}

func parseTree(data []byte) (*element, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if isUTF8Label(charset) || strings.TrimSpace(charset) == "" {
			return input, nil
		}
		return nil, fmt.Errorf("XML encoding %q is not supported", charset)
	}
	var stack []*element
	var root *element
	elements := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalidXML(err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return nil, fmt.Errorf("XML documents with a DTD or entity declaration are not supported")
		case xml.StartElement:
			elements++
			if elements > maxElements {
				return nil, fmt.Errorf("XML document has too many elements")
			}
			if len(stack) >= maxDocumentDepth {
				return nil, fmt.Errorf("XML document is nested too deeply")
			}
			if err := checkName(t.Name.Local); err != nil {
				return nil, err
			}
			n := &element{name: qname{Space: t.Name.Space, Local: t.Name.Local}}
			for _, a := range t.Attr {
				if isNamespaceAttr(a.Name) {
					continue
				}
				if err := checkName(a.Name.Local); err != nil {
					return nil, err
				}
				if len(a.Value) > MaxValueBytes {
					return nil, fmt.Errorf("XML value exceeds the %d byte limit", MaxValueBytes)
				}
				n.attrs = append(n.attrs, xmlAttr{name: qname{Space: a.Name.Space, Local: a.Name.Local}, value: a.Value})
				if len(n.attrs) > MaxFields {
					return nil, fmt.Errorf("XML document has too many fields")
				}
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("XML document is not valid")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.pieces = append(parent.pieces, piece{child: n})
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("XML document is not valid")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(t)) != "" {
					return nil, fmt.Errorf("XML document is not valid")
				}
				continue
			}
			if len(t) > MaxValueBytes {
				return nil, fmt.Errorf("XML value exceeds the %d byte limit", MaxValueBytes)
			}
			cur := stack[len(stack)-1]
			cur.pieces = append(cur.pieces, piece{text: string(t)})
			if len(cur.directText()) > MaxValueBytes {
				return nil, fmt.Errorf("XML value exceeds the %d byte limit", MaxValueBytes)
			}
		case xml.Comment, xml.ProcInst:
			continue
		default:
			continue
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, fmt.Errorf("XML document is not valid")
	}
	return root, nil
}

func invalidXML(err error) error {
	if err == nil {
		return fmt.Errorf("XML document is not valid")
	}
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "encoding") && strings.Contains(strings.ToLower(msg), "not supported") {
		return err
	}
	return fmt.Errorf("XML document is not valid")
}

func checkName(local string) error {
	if local == "" || len(local) > maxNameLen {
		return fmt.Errorf("XML document has a field name that is too long")
	}
	if strings.ContainsAny(local, ".@{}") {
		return fmt.Errorf("XML element name %q cannot be used as a field", local)
	}
	return nil
}

func isNamespaceAttr(name xml.Name) bool {
	if name.Space == "xmlns" || name.Space == "http://www.w3.org/2000/xmlns/" {
		return true
	}
	return name.Local == "xmlns" && name.Space == ""
}

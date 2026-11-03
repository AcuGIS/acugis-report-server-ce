package xmlfile

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/acugis/acugis-report-server-go/collectionfile"
)

func discover(root *element) (*Document, error) {
	if root == nil {
		return nil, fmt.Errorf("XML document has no record collection")
	}
	doc := &Document{nodes: map[string][]*element{}}
	rootPath := "/" + segmentFor(nil, root.name)
	if err := walkRepeating(root, rootPath, doc); err != nil {
		return nil, err
	}
	if err := addSingletons(root, rootPath, doc, false); err != nil {
		return nil, err
	}
	if len(doc.collections) == 0 {
		return nil, fmt.Errorf("XML document has no record collection")
	}
	sort.SliceStable(doc.collections, func(i, j int) bool {
		return strings.Count(doc.collections[i].Path, "/") < strings.Count(doc.collections[j].Path, "/")
	})
	return doc, nil
}

func walkRepeating(el *element, path string, doc *Document) error {
	groups := groupChildren(el)
	names := childNames(el)
	for _, g := range groups {
		if len(g.elems) < 2 || !allRecordLike(g.elems) {
			continue
		}
		if len(g.elems) > MaxRecords {
			return fmt.Errorf("XML document has too many records")
		}
		next := path + "/" + segmentFor(names, g.name)
		if _, exists := doc.nodes[next]; exists {
			return fmt.Errorf("XML document has ambiguous record collection %q", next)
		}
		if len(doc.collections) >= MaxCollections {
			return fmt.Errorf("XML document has too many record collections")
		}
		doc.nodes[next] = g.elems
		doc.collections = append(doc.collections, CollectionInfo{Path: next, Rows: len(g.elems)})
	}
	for _, c := range el.children() {
		next := path + "/" + segmentFor(names, c.name)
		if err := walkRepeating(c, next, doc); err != nil {
			return err
		}
	}
	return nil
}

func addSingletons(el *element, path string, doc *Document, under bool) error {
	if under || collectionPath(doc, path) {
		return nil
	}
	if recordShaped(el) {
		if _, exists := doc.nodes[path]; exists {
			return fmt.Errorf("XML document has ambiguous record collection %q", path)
		}
		if len(doc.collections) >= MaxCollections {
			return fmt.Errorf("XML document has too many record collections")
		}
		doc.nodes[path] = []*element{el}
		doc.collections = append(doc.collections, CollectionInfo{Path: path, Rows: 1})
		return nil
	}
	names := childNames(el)
	for _, c := range el.children() {
		next := path + "/" + segmentFor(names, c.name)
		childUnder := collectionPath(doc, next)
		if err := addSingletons(c, next, doc, childUnder); err != nil {
			return err
		}
	}
	return nil
}

func collectionPath(doc *Document, path string) bool {
	_, ok := doc.nodes[path]
	return ok
}

type childGroup struct {
	name  qname
	elems []*element
}

func groupChildren(el *element) []childGroup {
	var groups []childGroup
	index := map[string]int{}
	for _, c := range el.children() {
		key := c.name.Space + "\x00" + c.name.Local
		if i, ok := index[key]; ok {
			groups[i].elems = append(groups[i].elems, c)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, childGroup{name: c.name, elems: []*element{c}})
	}
	return groups
}

func childNames(el *element) []qname {
	kids := el.children()
	out := make([]qname, len(kids))
	for i, c := range kids {
		out[i] = c.name
	}
	return out
}

func segmentFor(siblings []qname, name qname) string {
	if name.Local == "" {
		return "_"
	}
	spaces := map[string]struct{}{}
	for _, n := range siblings {
		if n.Local == name.Local {
			spaces[n.Space] = struct{}{}
		}
	}
	if len(spaces) <= 1 {
		return name.Local
	}
	return "{" + name.Space + "}" + name.Local
}

func allRecordLike(elems []*element) bool {
	if len(elems) == 0 {
		return false
	}
	for _, el := range elems {
		if !recordLike(el) {
			return false
		}
	}
	return true
}

func recordLike(el *element) bool {
	if el == nil {
		return false
	}
	return len(el.attrs) > 0 || len(el.children()) > 0
}

func recordShaped(el *element) bool {
	if el == nil {
		return false
	}
	if len(el.attrs) > 0 {
		return true
	}
	for _, c := range el.children() {
		if len(c.children()) == 0 {
			return true
		}
	}
	return false
}

// Select chooses the record collection for a query.
// A FROM table that matches a discovered collection wins. Otherwise the
// configured path is used. An empty configuration uses the first collection.
func (d *Document) Select(configured, sql string) (string, error) {
	if d == nil || len(d.collections) == 0 {
		return "", fmt.Errorf("XML document has no record collection")
	}
	if name := tableFromSQL(sql); name != "" {
		if _, ok := d.nodes[name]; ok {
			return name, nil
		}
	}
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if _, ok := d.nodes[configured]; ok {
			return configured, nil
		}
		return "", fmt.Errorf("XML document has no record collection %q", configured)
	}
	return d.collections[0].Path, nil
}

// Table materializes one record collection. An empty path uses the first collection.
func (d *Document) Table(path string) (*collectionfile.Table, error) {
	if d == nil {
		return nil, fmt.Errorf("XML document has no record collection")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		if len(d.collections) == 0 {
			return nil, fmt.Errorf("XML document has no record collection")
		}
		path = d.collections[0].Path
	}
	rows, ok := d.nodes[path]
	if !ok {
		return nil, fmt.Errorf("XML document has no record collection %q", path)
	}
	if len(rows) > MaxRecords {
		return nil, fmt.Errorf("XML document has too many records")
	}
	return buildTable(rows)
}

var xmlFromRE = regexp.MustCompile(`(?is)\bfrom\s+(?:"(?:[^"]|"")*"\s*\.\s*)?(?:"((?:[^"]|"")*)"|([A-Za-z_][A-Za-z0-9_]*))`)

func tableFromSQL(sql string) string {
	m := xmlFromRE.FindStringSubmatch(sql)
	if m == nil {
		return ""
	}
	name := m[1]
	if name == "" {
		name = m[2]
	}
	return strings.ReplaceAll(strings.TrimSpace(name), `""`, `"`)
}

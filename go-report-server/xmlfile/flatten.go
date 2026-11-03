package xmlfile

import (
	"fmt"
	"strings"

	"github.com/acugis/acugis-report-server-go/collectionfile"
)

type multi struct {
	parts []collectionfile.Sample
}

func buildTable(records []*element) (*collectionfile.Table, error) {
	type col struct {
		name    string
		samples []collectionfile.Sample
	}
	var cols []*col
	index := map[string]int{}
	for row, rec := range records {
		flat, order, err := flattenRecord(rec)
		if err != nil {
			return nil, err
		}
		for _, name := range order {
			if _, ok := index[name]; ok {
				continue
			}
			if len(cols) >= MaxFields {
				return nil, fmt.Errorf("XML document has too many fields")
			}
			missing := make([]collectionfile.Sample, row)
			for i := range missing {
				missing[i] = collectionfile.Sample{Null: true}
			}
			index[name] = len(cols)
			cols = append(cols, &col{name: name, samples: missing})
		}
		for _, c := range cols {
			if sample, ok := flat[c.name]; ok {
				c.samples = append(c.samples, sample)
			} else {
				c.samples = append(c.samples, collectionfile.Sample{Null: true})
			}
		}
	}
	table := &collectionfile.Table{
		Columns: make([]collectionfile.Column, len(cols)),
		Rows:    make([]map[string]any, len(records)),
	}
	for i := range table.Rows {
		table.Rows[i] = map[string]any{}
	}
	for i, c := range cols {
		kind := collectionfile.ColumnKind(c.samples)
		table.Columns[i] = collectionfile.Column{Name: c.name, Kind: kind}
		for row, sample := range c.samples {
			table.Rows[row][c.name] = collectionfile.Finalize(kind, sample)
		}
	}
	return table, nil
}

func flattenRecord(el *element) (map[string]collectionfile.Sample, []string, error) {
	out := map[string]*multi{}
	var order []string
	if err := flatten(el, "", 0, out, &order); err != nil {
		return nil, nil, err
	}
	flat := make(map[string]collectionfile.Sample, len(order))
	for _, name := range order {
		flat[name] = collapse(out[name].parts)
	}
	return flat, order, nil
}

func flatten(el *element, prefix string, depth int, out map[string]*multi, order *[]string) error {
	if err := addAttrs(el, prefix, out, order); err != nil {
		return err
	}
	if prefix == "" {
		direct := el.directText()
		if strings.TrimSpace(direct) != "" {
			if err := addPart(out, order, "#text", collectionfile.Sample{Raw: direct, Text: true}); err != nil {
				return err
			}
		}
	}
	names := childNames(el)
	for _, c := range el.children() {
		seg := segmentFor(names, c.name)
		name := joinField(prefix, seg)
		if len(name) > maxNameLen {
			return fmt.Errorf("XML document has a field name that is too long")
		}
		if len(c.children()) == 0 && !c.mixed() {
			if err := addAttrs(c, name, out, order); err != nil {
				return err
			}
			sample := collectionfile.Sample{Null: true}
			if !c.scalarEmpty() {
				sample = collectionfile.Sample{Raw: c.directText()}
			}
			if err := addPart(out, order, name, sample); err != nil {
				return err
			}
			continue
		}
		if c.mixed() {
			if err := addAttrs(c, name, out, order); err != nil {
				return err
			}
			if err := addPart(out, order, name, collectionfile.Sample{Raw: c.allText(), Text: true}); err != nil {
				return err
			}
			continue
		}
		if depth+1 >= MaxFlattenDepth {
			if err := addPart(out, order, name, collectionfile.Sample{Raw: canonical(c), Text: true}); err != nil {
				return err
			}
			continue
		}
		if err := flatten(c, name, depth+1, out, order); err != nil {
			return err
		}
	}
	return nil
}

func addAttrs(el *element, prefix string, out map[string]*multi, order *[]string) error {
	if el == nil {
		return nil
	}
	names := make([]qname, len(el.attrs))
	for i, a := range el.attrs {
		names[i] = a.name
	}
	for _, a := range el.attrs {
		seg := "@" + segmentFor(names, a.name)
		name := joinField(prefix, seg)
		if len(name) > maxNameLen {
			return fmt.Errorf("XML document has a field name that is too long")
		}
		if err := addPart(out, order, name, collectionfile.Sample{Raw: a.value}); err != nil {
			return err
		}
	}
	return nil
}

func joinField(prefix, seg string) string {
	if prefix == "" {
		return seg
	}
	return prefix + "." + seg
}

func addPart(out map[string]*multi, order *[]string, name string, sample collectionfile.Sample) error {
	if len(name) > maxNameLen {
		return fmt.Errorf("XML document has a field name that is too long")
	}
	cur, ok := out[name]
	if !ok {
		if len(out) >= MaxFields {
			return fmt.Errorf("XML document has too many fields")
		}
		cur = &multi{}
		out[name] = cur
		*order = append(*order, name)
	}
	cur.parts = append(cur.parts, sample)
	return nil
}

func collapse(parts []collectionfile.Sample) collectionfile.Sample {
	if len(parts) == 0 {
		return collectionfile.Sample{Null: true}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	allNull := true
	bits := make([]string, len(parts))
	for i, p := range parts {
		if p.Null {
			continue
		}
		allNull = false
		bits[i] = p.Raw
	}
	if allNull {
		return collectionfile.Sample{Null: true}
	}
	return collectionfile.Sample{Raw: strings.Join(bits, RepeatSeparator), Text: true}
}

func canonical(el *element) string {
	if el == nil {
		return `""`
	}
	if el.mixed() {
		return quoteCanon(el.allText())
	}
	if len(el.children()) == 0 && len(el.attrs) == 0 {
		if el.scalarEmpty() {
			return `""`
		}
		return quoteCanon(el.directText())
	}
	var b strings.Builder
	b.WriteByte('{')
	first := true
	emit := func(s string) {
		if !first {
			b.WriteByte(' ')
		}
		first = false
		b.WriteString(s)
	}
	names := make([]qname, len(el.attrs))
	for i, a := range el.attrs {
		names[i] = a.name
	}
	for _, a := range el.attrs {
		emit("@" + segmentFor(names, a.name) + "=" + quoteCanon(a.value))
	}
	type group struct {
		name string
		vals []string
	}
	var groups []group
	idx := map[string]int{}
	childNs := childNames(el)
	for _, c := range el.children() {
		seg := segmentFor(childNs, c.name)
		val := canonical(c)
		if i, ok := idx[seg]; ok {
			groups[i].vals = append(groups[i].vals, val)
			continue
		}
		idx[seg] = len(groups)
		groups = append(groups, group{name: seg, vals: []string{val}})
	}
	for _, g := range groups {
		if len(g.vals) == 1 && !strings.HasPrefix(g.vals[0], "{") && !strings.HasPrefix(g.vals[0], `"`) {
			emit(g.name + "=" + quoteCanon(g.vals[0]))
			continue
		}
		if len(g.vals) == 1 {
			emit(g.name + "=" + g.vals[0])
			continue
		}
		emit(g.name + "=[" + strings.Join(g.vals, " ") + "]")
	}
	b.WriteByte('}')
	return b.String()
}

func quoteCanon(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

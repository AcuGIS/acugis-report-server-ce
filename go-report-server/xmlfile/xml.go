// Package xmlfile reads one XML document into typed row maps for the existing
// collection query evaluator. It is not an XPath or XQuery engine. A stored
// element path selects the repeating record elements. Filtering is done by the
// collection query, for example WHERE "region" = $P{p_region}.
//
// A record collection is a path such as /customers/customer. Paths use local
// names when that local name is unambiguous among its siblings. When two
// namespaces share a local name, the segment is {namespace-uri}local. Matching
// uses the namespace URI and the local name, not the document's prefixes.
//
// Direct scalar child elements become fields. Nested scalar elements use
// dot-qualified names, analogous to JSON: address.city. Attributes use an @
// prefix so they cannot collide with child elements: @id, address.@type.
//
// Repeated child elements inside one record do not become extra rows. Every
// value is kept, in document order, joined with " | ". An empty occurrence
// contributes an empty slot. The joined value is a string. If every occurrence
// is empty, the field is null.
//
// Missing elements and self-closing or empty elements are null. Whitespace-only
// text is preserved exactly and is not trimmed. Text that contains non-whitespace
// is also preserved exactly. Mixed content, such as
// <description>This is <b>important</b> text.</description>, becomes the
// concatenation of character data in document order. Markup is not kept.
// CDATA is ordinary character data. A DTD or entity declaration is rejected.
//
// Type inference follows the CSV rules: integer to Long, decimal to Double,
// true/false to Boolean, YYYY-MM-DD to Date, ISO datetime to Timestamp, and
// everything else to String. Leading zeros stay strings. Empty and missing
// values do not vote. Long and Double widen to Double. Date and Timestamp widen
// to Timestamp. Any other mix becomes String.
//
// Supported encodings are UTF-8, UTF-16 (with a BOM or a UTF-16 signature),
// US-ASCII, ISO-8859-1, ISO-8859-15, and Windows-1252. Any other declared
// encoding is an error. A declaration that does not match the bytes is an error.
package xmlfile

import "github.com/acugis/acugis-report-server-go/collectionfile"

const (
	// MaxBytes is the largest document that will be read.
	MaxBytes = collectionfile.MaxBytes
	// MaxRecords is the most rows one collection may contain.
	// Exceeding it is an error. Rows are not silently truncated.
	MaxRecords = 100000
	// MaxFields is the largest union of fields a collection may expose.
	MaxFields = 512
	// MaxCollections is the most record collections one document may expose.
	MaxCollections = 64
	// MaxFlattenDepth is how many nested elements are expanded into dot fields.
	// Deeper elements are kept as one deterministic string.
	MaxFlattenDepth = 8
	// MaxValueBytes is the largest text or attribute value that will be kept.
	MaxValueBytes = 1 << 20
	// RepeatSeparator joins repeated values inside one record.
	RepeatSeparator = " | "

	maxDocumentDepth = 32
	maxNameLen       = 256
	maxElements      = 2000000
)

// SupportedEncodings lists the encodings Parse accepts.
func SupportedEncodings() []string {
	return []string{
		"UTF-8",
		"UTF-16",
		"US-ASCII",
		"ISO-8859-1",
		"ISO-8859-15",
		"Windows-1252",
	}
}

// CollectionInfo is one repeating record element the designer can select.
type CollectionInfo struct {
	Path string
	Rows int
}

// Document is a parsed XML document and the record collections found in it.
type Document struct {
	collections []CollectionInfo
	nodes       map[string][]*element
}

// Collections returns discovered record collections in document order.
func (d *Document) Collections() []CollectionInfo {
	if d == nil {
		return nil
	}
	out := make([]CollectionInfo, len(d.collections))
	copy(out, d.collections)
	return out
}

package xmlfile

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

var encodingValue = regexp.MustCompile(`(?i)\sencoding\s*=\s*["']([^"']+)["']`)
var encodingStrip = regexp.MustCompile(`(?i)\s+encoding\s*=\s*(?:"[^"]*"|'[^']*')`)

func normalize(data []byte) ([]byte, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("XML file exceeds the %d byte limit", MaxBytes)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	transport, data, err := decodeTransport(data)
	if err != nil {
		return nil, err
	}
	declared := declaredEncoding(data)
	switch transport {
	case "utf-16":
		if declared != "" && !isUTF16Label(declared) {
			return nil, fmt.Errorf("XML encoding %q does not match the UTF-16 document", declared)
		}
		if !utf8.Valid(data) {
			return nil, fmt.Errorf("XML document is not valid")
		}
	default:
		switch {
		case declared == "" || isUTF8Label(declared):
			if !utf8.Valid(data) {
				return nil, fmt.Errorf("XML document is not valid")
			}
		case isASCIILabel(declared):
			if !isASCII(data) {
				return nil, fmt.Errorf("XML encoding %q does not match the document bytes", declared)
			}
		case isUTF16Label(declared):
			return nil, fmt.Errorf("UTF-16 XML requires a byte-order mark or a UTF-16 signature")
		default:
			dec, ok := charsetDecoder(declared)
			if !ok {
				return nil, fmt.Errorf("XML encoding %q is not supported", declared)
			}
			data, err = decodeAll(dec, data)
			if err != nil || !utf8.Valid(data) {
				return nil, fmt.Errorf("XML encoding %q could not be decoded", declared)
			}
		}
	}
	if dtdPresent(data) {
		return nil, fmt.Errorf("XML documents with a DTD or entity declaration are not supported")
	}
	return stripEncoding(data), nil
}

func decodeTransport(data []byte) (string, []byte, error) {
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		out, err := decodeUTF16(data, unicode.BigEndian, unicode.ExpectBOM)
		return "utf-16", out, err
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		out, err := decodeUTF16(data, unicode.LittleEndian, unicode.ExpectBOM)
		return "utf-16", out, err
	}
	if looksUTF16LE(data) {
		out, err := decodeUTF16(data, unicode.LittleEndian, unicode.IgnoreBOM)
		return "utf-16", out, err
	}
	if looksUTF16BE(data) {
		out, err := decodeUTF16(data, unicode.BigEndian, unicode.IgnoreBOM)
		return "utf-16", out, err
	}
	return "utf-8", data, nil
}

func looksUTF16LE(data []byte) bool {
	return len(data) >= 8 && data[0] == '<' && data[1] == 0 && data[2] == '?' && data[3] == 0
}

func looksUTF16BE(data []byte) bool {
	return len(data) >= 8 && data[0] == 0 && data[1] == '<' && data[2] == 0 && data[3] == '?'
}

func decodeUTF16(data []byte, order unicode.Endianness, bom unicode.BOMPolicy) ([]byte, error) {
	out, err := decodeAll(unicode.UTF16(order, bom).NewDecoder(), data)
	if err != nil {
		return nil, fmt.Errorf("XML document is not valid")
	}
	return out, nil
}

func decodeAll(dec transform.Transformer, data []byte) ([]byte, error) {
	return io.ReadAll(transform.NewReader(bytes.NewReader(data), dec))
}

func declaredEncoding(data []byte) string {
	decl, ok := xmlDeclaration(data)
	if !ok {
		return ""
	}
	m := encodingValue.FindStringSubmatch(decl)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func stripEncoding(data []byte) []byte {
	decl, ok := xmlDeclaration(data)
	if !ok {
		return data
	}
	stripped := encodingStrip.ReplaceAllString(decl, "")
	if stripped == decl {
		return data
	}
	s := string(data)
	idx := strings.Index(s, decl)
	if idx < 0 {
		return data
	}
	return []byte(s[:idx] + stripped + s[idx+len(decl):])
}

func xmlDeclaration(data []byte) (string, bool) {
	s := string(data)
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			goto done
		}
	}
done:
	rest := s[i:]
	if len(rest) < 5 || !strings.EqualFold(rest[:5], "<?xml") {
		return "", false
	}
	end := strings.Index(rest, "?>")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

func dtdPresent(data []byte) bool {
	lower := bytes.ToLower(data)
	return bytes.Contains(lower, []byte("<!doctype")) || bytes.Contains(lower, []byte("<!entity"))
}

func charsetDecoder(label string) (transform.Transformer, bool) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1", "l1", "csisolatin1":
		return charmap.ISO8859_1.NewDecoder(), true
	case "iso-8859-15", "iso8859-15", "latin9", "latin-9", "l9":
		return charmap.ISO8859_15.NewDecoder(), true
	case "windows-1252", "cp1252", "cswindows1252":
		return charmap.Windows1252.NewDecoder(), true
	default:
		return nil, false
	}
}

func isUTF8Label(label string) bool {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-8", "utf8":
		return true
	default:
		return false
	}
}

func isUTF16Label(label string) bool {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-16", "utf16", "utf-16le", "utf-16be", "utf16le", "utf16be":
		return true
	default:
		return false
	}
}

func isASCIILabel(label string) bool {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "us-ascii", "ascii", "ansi_x3.4-1968":
		return true
	default:
		return false
	}
}

func isASCII(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 {
			return false
		}
	}
	return true
}

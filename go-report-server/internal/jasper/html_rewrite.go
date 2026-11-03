package jasper

import (
	"bytes"
	"io"
	"net/url"
)

// htmlImageRewriter streams HTML and rewrites Jasper image servlet URLs to the
// Go image proxy (PHP report_image.php and Tomcat 10 acugis_report_image).
//
// Tomcat may bake a context path into the URL, e.g.
//
//	/acugis-report-server/acugis_report_image?image=…
//
// The rewriter strips that prefix so the Go proxy path is not doubled.
type htmlImageRewriter struct {
	src     io.ReadCloser
	markers [][]byte
	repl    []byte
	buf     []byte
	err     error
	eof     bool
	keep    int
}

func newHTMLImageRewriter(src io.ReadCloser, reportName, imageProxyBase string) io.ReadCloser {
	base := imageProxyBase
	if base == "" {
		base = "/dev/report-image"
	}
	repl := base + "?_repName=" + url.QueryEscape(reportName) + "&image="
	markers := [][]byte{
		[]byte("/report_image.php?image="),
		[]byte("/acugis_report_image?image="),
	}
	keep := 0
	for _, n := range markers {
		// Allow a typical context-path prefix before the servlet marker.
		if nlen := len(n) + 64; nlen-1 > keep {
			keep = nlen - 1
		}
	}
	return &htmlImageRewriter{
		src:     src,
		markers: markers,
		repl:    []byte(repl),
		buf:     make([]byte, 0, 4096),
		keep:    keep,
	}
}

// findImageURL locates a Jasper image servlet URL, including an optional
// single context-path segment before the servlet name.
func (r *htmlImageRewriter) findImageURL() (start, end int) {
	bestStart, bestEnd := -1, -1
	for _, marker := range r.markers {
		i := bytes.Index(r.buf, marker)
		if i < 0 {
			continue
		}
		s := i
		// Expand left over "/context" so "/ctx/acugis_report_image?image=" is
		// replaced as a whole (not leaving "/ctx" prefixed to the Go proxy).
		if i > 0 && r.buf[i] == '/' {
			j := i - 1
			for j >= 0 && isURLPathByte(r.buf[j]) {
				j--
			}
			// j is at the char before the path; path starts at j+1.
			cand := j + 1
			if cand < i && r.buf[cand] == '/' {
				s = cand
			}
		}
		e := i + len(marker)
		if bestStart < 0 || s < bestStart {
			bestStart, bestEnd = s, e
		}
	}
	return bestStart, bestEnd
}

func isURLPathByte(b byte) bool {
	// Path segment chars that may appear in a Tomcat context path.
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '/' || b == '-' || b == '_' || b == '.' || b == '~'
}

func (r *htmlImageRewriter) Read(p []byte) (int, error) {
	if r.err != nil && len(r.buf) == 0 {
		return 0, r.err
	}
	for {
		if start, end := r.findImageURL(); start >= 0 {
			out := append([]byte{}, r.buf[:start]...)
			out = append(out, r.repl...)
			r.buf = r.buf[end:]
			n := copy(p, out)
			if n < len(out) {
				r.buf = append(out[n:], r.buf...)
			}
			return n, nil
		}
		if r.eof {
			if len(r.buf) == 0 {
				if r.err != nil {
					return 0, r.err
				}
				return 0, io.EOF
			}
			n := copy(p, r.buf)
			r.buf = r.buf[n:]
			return n, nil
		}
		if len(r.buf) > r.keep && len(p) > 0 {
			emit := len(r.buf) - r.keep
			if emit > len(p) {
				emit = len(p)
			}
			n := copy(p, r.buf[:emit])
			r.buf = r.buf[n:]
			return n, nil
		}
		tmp := make([]byte, 32*1024)
		n, err := r.src.Read(tmp)
		if n > 0 {
			r.buf = append(r.buf, tmp[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				r.eof = true
				r.err = io.EOF
			} else {
				r.err = err
				r.eof = true
			}
		}
	}
}

func (r *htmlImageRewriter) Close() error {
	if r.src == nil {
		return nil
	}
	return r.src.Close()
}

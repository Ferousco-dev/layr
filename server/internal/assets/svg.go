package assets

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
)

// maxSVGBytes bounds the in-memory document the sanitizer will handle.
const maxSVGBytes = 20 << 20

var ErrInvalidSVG = errors.New("asset is not a valid svg document")

// SVG sanitizing removes active content and external references; inlining still needs a CSP.

// dangerousElements can run code or load outside content.
var dangerousElements = map[string]bool{
	"script": true, "foreignobject": true, "iframe": true, "embed": true, "object": true,
	"audio": true, "video": true, "canvas": true, "applet": true, "meta": true, "link": true, "base": true,
	"animate": true, "set": true, "animatemotion": true, "animatetransform": true, "handler": true, "listener": true,
}

var (
	safeDataURI     = regexp.MustCompile(`(?i)^data:image/(png|jpeg|jpg|gif|webp);base64,[a-z0-9+/=\s]*$`)
	externalInStyle = regexp.MustCompile(`(?i)(@import|expression\s*\(|javascript:|behavior\s*:|-moz-binding|url\s*\(\s*['"]?\s*[^#'")\s])`)
)

// SanitizeReport counts what was removed, for warnings.
type SanitizeReport struct {
	Elements   int
	Attributes int
}

func (r SanitizeReport) Changed() bool { return r.Elements+r.Attributes > 0 }

// sanitizeSVG rewrites the document token by token, dropping unsafe elements and attributes.
func sanitizeSVG(doc []byte) ([]byte, SanitizeReport, error) {
	var report SanitizeReport
	if len(doc) > maxSVGBytes {
		return nil, report, ErrTooLarge
	}
	dec := xml.NewDecoder(bytes.NewReader(doc))
	dec.Strict = true
	dec.CharsetReader = func(charset string, in io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "us-ascii") {
			return in, nil
		}
		return nil, ErrInvalidSVG
	}

	var out bytes.Buffer
	depth, skip, sawRoot := 0, 0, false
	for {
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, report, ErrInvalidSVG
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if skip > 0 {
				skip++
				continue
			}
			if !sawRoot {
				if !strings.EqualFold(t.Name.Local, "svg") {
					return nil, report, ErrInvalidSVG
				}
				sawRoot = true
			}
			if dangerousElements[strings.ToLower(t.Name.Local)] {
				skip, report.Elements = 1, report.Elements+1
				continue
			}
			depth++
			writeStart(&out, t, &report)
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			depth--
			if depth < 0 {
				return nil, report, ErrInvalidSVG
			}
			out.WriteString("</" + qualified(t.Name) + ">")
		case xml.CharData:
			if skip == 0 {
				_ = xml.EscapeText(&out, t)
			}
		case xml.Comment, xml.Directive, xml.ProcInst:
			// Doctypes carry entity tricks, and comments and instructions add nothing to the drawing.
		}
	}
	if !sawRoot || depth != 0 || skip != 0 {
		return nil, report, ErrInvalidSVG
	}
	return out.Bytes(), report, nil
}

func qualified(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func writeStart(out *bytes.Buffer, t xml.StartElement, report *SanitizeReport) {
	out.WriteString("<" + qualified(t.Name))
	for _, a := range t.Attr {
		if !attributeAllowed(a) {
			report.Attributes++
			continue
		}
		out.WriteString(" " + qualified(a.Name) + `="`)
		_ = xml.EscapeText(out, []byte(a.Value))
		out.WriteString(`"`)
	}
	out.WriteString(">")
}

// attributeAllowed drops event handlers, external references and scriptable styles.
func attributeAllowed(a xml.Attr) bool {
	name := strings.ToLower(a.Name.Local)
	value := strings.TrimSpace(a.Value)
	switch {
	case strings.HasPrefix(name, "on"):
		return false
	case name == "href" || name == "src" || name == "data":
		return value == "" || strings.HasPrefix(value, "#") || safeDataURI.MatchString(value)
	case name == "style" || name == "content":
		return !externalInStyle.MatchString(value)
	}
	// Presentation attributes such as fill="url(http://...)" can also fetch remote content.
	if strings.Contains(strings.ToLower(value), "javascript:") {
		return false
	}
	if lower := strings.ToLower(value); strings.Contains(lower, "url(") && externalInStyle.MatchString(value) {
		return false
	}
	return true
}

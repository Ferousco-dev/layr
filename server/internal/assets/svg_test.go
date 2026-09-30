package assets

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

// figmaSVG is representative of Figma's export: prefixes, gradients, clip paths and an embedded image.
const figmaSVG = `<svg width="48" height="48" viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">
<g clip-path="url(#clip0_1_2)">
<path fill-rule="evenodd" clip-rule="evenodd" d="M24 4L44 44H4L24 4Z" fill="url(#paint0_linear_1_2)"/>
<rect x="8" y="8" width="8" height="8" fill="#FF0000" fill-opacity="0.5" style="mix-blend-mode:multiply"/>
<use xlink:href="#shape" transform="translate(2 2)"/>
<image id="img" width="4" height="4" xlink:href="data:image/png;base64,iVBORw0KGgo="/>
</g>
<defs>
<linearGradient id="paint0_linear_1_2" x1="24" y1="4" x2="24" y2="44" gradientUnits="userSpaceOnUse"><stop stop-color="#0057FF"/><stop offset="1" stop-color="#00C2FF"/></linearGradient>
<clipPath id="clip0_1_2"><rect width="48" height="48" fill="white"/></clipPath>
<path id="shape" d="M0 0H1V1H0Z"/>
</defs>
</svg>`

func TestSanitizeKeepsAFigmaExportIntact(t *testing.T) {
	out, report, err := sanitizeSVG([]byte(figmaSVG))
	if err != nil {
		t.Fatal(err)
	}

	if report.Changed() {
		t.Fatalf("a clean export was modified: %+v", report)
	}
	text := string(out)
	for _, want := range []string{
		`viewBox="0 0 48 48"`, `xmlns:xlink="http://www.w3.org/1999/xlink"`, `xlink:href="#shape"`, `xlink:href="data:image/png;base64,iVBORw0KGgo="`,
		`d="M24 4L44 44H4L24 4Z"`, `fill="url(#paint0_linear_1_2)"`, `clip-path="url(#clip0_1_2)"`, `style="mix-blend-mode:multiply"`,
		`<stop stop-color="#0057FF"></stop>`, `fill-opacity="0.5"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("lost %q in:\n%s", want, text)
		}
	}
	assertWellFormed(t, out)
}

func TestSanitizeRemovesActiveContent(t *testing.T) {
	cases := []struct {
		name, doc string
		gone      []string
		keep      []string
	}{
		{"script element", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script><path d="M0 0"/></svg>`, []string{"script", "alert"}, []string{`d="M0 0"`}},
		{"nested script", `<svg xmlns="http://www.w3.org/2000/svg"><g><script><![CDATA[fetch('//evil')]]></script></g><path d="M1 1"/></svg>`, []string{"script", "fetch", "evil"}, []string{`d="M1 1"`}},
		{"onload", `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><path d="M2 2"/></svg>`, []string{"onload", "alert"}, []string{`d="M2 2"`}},
		{"onclick", `<svg xmlns="http://www.w3.org/2000/svg"><path onclick="x()" onmouseover="y()" d="M3 3"/></svg>`, []string{"onclick", "onmouseover"}, []string{`d="M3 3"`}},
		{"javascript href", `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><a xlink:href="javascript:alert(1)"><path d="M4 4"/></a></svg>`, []string{"javascript"}, []string{`d="M4 4"`}},
		{"external image", `<svg xmlns="http://www.w3.org/2000/svg"><image href="https://evil.example/track.png" width="1" height="1"/></svg>`, []string{"evil.example"}, nil},
		{"external use", `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="https://evil.example/x.svg#a"/></svg>`, []string{"evil.example"}, nil},
		{"data html uri", `<svg xmlns="http://www.w3.org/2000/svg"><image href="data:text/html;base64,PHNjcmlwdD4="/></svg>`, []string{"text/html"}, nil},
		{"data svg uri", `<svg xmlns="http://www.w3.org/2000/svg"><image href="data:image/svg+xml;base64,PHN2Zz4="/></svg>`, []string{"data:image/svg"}, nil},
		{"foreignObject", `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><iframe src="https://evil.example"></iframe><p>hi</p></foreignObject><path d="M5 5"/></svg>`, []string{"foreignObject", "iframe", "evil.example", "<p>"}, []string{`d="M5 5"`}},
		{"style import", `<svg xmlns="http://www.w3.org/2000/svg"><path style="fill:url(https://evil.example/x)" d="M6 6"/></svg>`, []string{"evil.example"}, []string{`d="M6 6"`}},
		{"style expression", `<svg xmlns="http://www.w3.org/2000/svg"><path style="width:expression(alert(1))" d="M7 7"/></svg>`, []string{"expression"}, []string{`d="M7 7"`}},
		{"animate href", `<svg xmlns="http://www.w3.org/2000/svg"><a href="#x"><set attributeName="href" to="javascript:alert(1)"/></a></svg>`, []string{"javascript", "<set"}, nil},
		{"doctype entity", `<!DOCTYPE svg [<!ENTITY x "boom">]><svg xmlns="http://www.w3.org/2000/svg"><path d="M8 8"/></svg>`, []string{"ENTITY", "DOCTYPE"}, []string{`d="M8 8"`}},
		{"upper case script", `<svg xmlns="http://www.w3.org/2000/svg"><SCRIPT>alert(1)</SCRIPT></svg>`, []string{"SCRIPT", "alert"}, nil},
		{"fill url external", `<svg xmlns="http://www.w3.org/2000/svg"><path fill="url(https://evil.example/f)" d="M9 9"/></svg>`, []string{"evil.example"}, []string{`d="M9 9"`}},
	}
	for _, tc := range cases {
		out, report, err := sanitizeSVG([]byte(tc.doc))
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		text := string(out)
		for _, gone := range tc.gone {
			if strings.Contains(strings.ToLower(text), strings.ToLower(gone)) {
				t.Errorf("%s: %q survived in %s", tc.name, gone, text)
			}
		}
		for _, keep := range tc.keep {
			if !strings.Contains(text, keep) {
				t.Errorf("%s: lost %q in %s", tc.name, keep, text)
			}
		}
		if tc.name != "doctype entity" && !report.Changed() {
			t.Errorf("%s: nothing reported as removed", tc.name)
		}
		assertWellFormed(t, out)
	}
}

func TestSanitizeRejectsWhatIsNotAnSVG(t *testing.T) {
	for name, doc := range map[string]string{
		"html root":  `<html><body>hi</body></html>`,
		"empty":      ``,
		"unbalanced": `<svg xmlns="http://www.w3.org/2000/svg"><g></svg>`,
		"extra end":  `<svg xmlns="http://www.w3.org/2000/svg"></svg></g>`,
		"garbage":    `<svg`,
		"latin1":     `<?xml version="1.0" encoding="ISO-8859-1"?><svg xmlns="http://www.w3.org/2000/svg"/>`,
	} {
		if _, _, err := sanitizeSVG([]byte(doc)); !errors.Is(err, ErrInvalidSVG) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, _, err := sanitizeSVG(make([]byte, maxSVGBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversized: %v", err)
	}
}

func TestSanitizeIsIdempotent(t *testing.T) {
	once, _, _ := sanitizeSVG([]byte(figmaSVG))
	twice, report, err := sanitizeSVG(once)
	if err != nil || string(once) != string(twice) || report.Changed() {
		t.Fatalf("second pass changed output (err %v, report %+v)", err, report)
	}
}

func assertWellFormed(t *testing.T, doc []byte) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(doc)))
	for {
		if _, err := dec.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			t.Fatalf("sanitized output is not well formed: %v\n%s", err, doc)
		}
	}
}

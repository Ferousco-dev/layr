package assets

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func pngBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gifBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := gif.Encode(&b, image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// webpVP8X builds the smallest valid extended WebP header.
func webpVP8X(w, h int) []byte {
	b := make([]byte, 30)
	copy(b[0:], "RIFF")
	binary.LittleEndian.PutUint32(b[4:], 22)
	copy(b[8:], "WEBPVP8X")
	binary.LittleEndian.PutUint32(b[16:], 10)
	w, h = w-1, h-1
	b[24], b[25], b[26] = byte(w), byte(w>>8), byte(w>>16)
	b[27], b[28], b[29] = byte(h), byte(h>>8), byte(h>>16)
	return b
}

const svgSample = `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M0 0L24 24" stroke="black"/></svg>`

func TestSniffRecognisesFormatsByBytesAndReadsDimensions(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Format
		w, h int
	}{
		{"png", pngBytes(t, 4, 2), formatPNG, 4, 2},
		{"jpeg", jpegBytes(t, 8, 6), formatJPEG, 8, 6},
		{"gif", gifBytes(t, 5, 3), formatGIF, 5, 3},
		{"webp", webpVP8X(640, 480), formatWebP, 640, 480},
	}
	for _, tc := range cases {
		f, ok := sniff(tc.data)
		w, h, dimsOK := dimensions(f, tc.data)
		if !ok || f != tc.want || !dimsOK || w != tc.w || h != tc.h {
			t.Errorf("%s: format %v ok %v dims %dx%d ok %v", tc.name, f, ok, w, h, dimsOK)
		}
	}
}

func TestSniffRejectsWhatIsNotAnImage(t *testing.T) {
	for name, data := range map[string][]byte{
		"html": []byte("<html><script>alert(1)</script></html>"), "php": []byte("<?php system($_GET['c']); ?>"),
		"empty": {}, "text": []byte("hello"), "truncated png": []byte("\x89PNG\r\n\x1a\n"), "zip": []byte("PK\x03\x04"),
	} {
		if f, ok := sniff(data); ok {
			if _, _, dims := dimensions(f, data); dims {
				t.Errorf("%s accepted as %v", name, f)
			}
		}
		if looksLikeSVG(data) {
			t.Errorf("%s accepted as svg", name)
		}
	}
}

func TestLooksLikeSVGAcceptsRealPrologs(t *testing.T) {
	for name, doc := range map[string]string{
		"plain":      svgSample,
		"xml decl":   `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + svgSample,
		"comment":    `<!-- Generator: Figma -->` + svgSample,
		"doctype":    `<?xml version="1.0"?><!DOCTYPE svg PUBLIC "x" "y">` + svgSample,
		"prefixed":   `<svg:svg xmlns:svg="http://www.w3.org/2000/svg"></svg:svg>`,
		"whitespace": "\n\n  " + svgSample,
		"upper case": `<SVG xmlns="http://www.w3.org/2000/svg"></SVG>`,
	} {
		if !looksLikeSVG([]byte(doc)) {
			t.Errorf("%s not recognised", name)
		}
	}
	for _, doc := range []string{`<html><svg></svg></html>`, `<div><svg/></div>`, `svg`} {
		if looksLikeSVG([]byte(doc)) {
			t.Errorf("%q accepted", doc)
		}
	}
}

func TestDeclaredContentTypePolicy(t *testing.T) {
	for _, ok := range []string{"", "image/png", "IMAGE/JPEG", "image/svg+xml; charset=utf-8", "application/octet-stream", "binary/octet-stream", "text/xml", "application/xml; charset=utf-8", "text/plain; charset=utf-8"} {
		if err := checkDeclaredType(ok); err != nil {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"text/html", "application/json", "application/x-php", "text/html; charset=utf-8", "application/javascript"} {
		if err := checkDeclaredType(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSVGSizeOnlyReportsWhatIsDeclared(t *testing.T) {
	if w, h, ok := svgSize([]byte(svgSample)); !ok || w != 24 || h != 24 {
		t.Fatalf("size = %v %v %v", w, h, ok)
	}
	if w, h, ok := svgSize([]byte(`<svg viewBox="0 0 100 50" xmlns="http://www.w3.org/2000/svg"></svg>`)); !ok || w != 100 || h != 50 {
		t.Fatalf("viewBox size = %v %v %v", w, h, ok)
	}
	if _, _, ok := svgSize([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="5" height="5"/></svg>`)); ok {
		t.Fatal("dimensions invented from a child element")
	}
	if _, _, ok := svgSize([]byte(`<svg width="100%" height="100%"></svg>`)); ok {
		t.Fatal("percentage treated as pixels")
	}
}

package assets

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif" // registers the decoder for image.DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"strconv"
	"strings"
)

// Format is a stored asset's true format, decided from its bytes.
type Format struct {
	Name      string
	Extension string
	MediaType string
}

var (
	formatPNG  = Format{"png", "png", "image/png"}
	formatJPEG = Format{"jpeg", "jpg", "image/jpeg"}
	formatGIF  = Format{"gif", "gif", "image/gif"}
	formatWebP = Format{"webp", "webp", "image/webp"}
	formatSVG  = Format{"svg", "svg", "image/svg+xml"}
)

var (
	ErrUnsupportedContent = errors.New("asset content is not a supported image")
	ErrContentType        = errors.New("asset content type is not an image")
)

// sniff identifies raster formats by magic bytes; SVG is recognised separately by looksLikeSVG.
func sniff(head []byte) (Format, bool) {
	switch {
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return formatPNG, true
	case bytes.HasPrefix(head, []byte{0xff, 0xd8, 0xff}):
		return formatJPEG, true
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return formatGIF, true
	case len(head) >= 12 && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
		return formatWebP, true
	}
	return Format{}, false
}

var svgRoot = regexp.MustCompile(`(?is)^\s*(<\?xml[^>]*\?>\s*)?(<!--.*?-->\s*)*(<!DOCTYPE[^>]*>\s*)?(<!--.*?-->\s*)*<(\w+:)?svg[\s>/]`)

// looksLikeSVG checks the document opens with an svg root element.
func looksLikeSVG(head []byte) bool {
	if len(head) > 4096 {
		head = head[:4096]
	}
	return svgRoot.Match(head)
}

// checkDeclaredType rejects non-image types; missing, generic, XML or plain-text types defer to the bytes.
func checkDeclaredType(contentType string) error {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch {
	case ct == "", ct == "application/octet-stream", ct == "binary/octet-stream", ct == "text/xml", ct == "application/xml", ct == "text/plain", strings.HasPrefix(ct, "image/"):
		return nil
	}
	return ErrContentType
}

// dimensions reads pixel size from the file header; ok is false when it cannot be determined.
func dimensions(f Format, head []byte) (w, h int, ok bool) {
	if f == formatWebP {
		return webpSize(head)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(head))
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// webpSize parses the VP8, VP8L and VP8X headers.
func webpSize(b []byte) (int, int, bool) {
	if len(b) < 30 {
		return 0, 0, false
	}
	switch string(b[12:16]) {
	case "VP8 ":
		if len(b) < 30 || b[23] != 0x9d || b[24] != 0x01 || b[25] != 0x2a {
			return 0, 0, false
		}
		return int(binary.LittleEndian.Uint16(b[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(b[28:30]) & 0x3fff), true
	case "VP8L":
		if b[20] != 0x2f {
			return 0, 0, false
		}
		bits := binary.LittleEndian.Uint32(b[21:25])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, true
	case "VP8X":
		w := int(b[24]) | int(b[25])<<8 | int(b[26])<<16
		h := int(b[27]) | int(b[28])<<8 | int(b[29])<<16
		return w + 1, h + 1, true
	}
	return 0, 0, false
}

var (
	viewBoxPattern = regexp.MustCompile(`(?i)\sviewBox\s*=\s*["']\s*([-\d.eE]+)[\s,]+([-\d.eE]+)[\s,]+([-\d.eE]+)[\s,]+([-\d.eE]+)\s*["']`)
	sizePattern    = func(attr string) *regexp.Regexp {
		return regexp.MustCompile(`(?i)\s` + attr + `\s*=\s*["']\s*([\d.]+)\s*(px)?\s*["']`)
	}
	widthAttr, heightAttr = sizePattern("width"), sizePattern("height")
)

// svgSize returns declared dimensions from the root tag; ok is false when they are absent.
func svgSize(doc []byte) (float64, float64, bool) {
	end := bytes.IndexByte(doc, '>')
	if i := bytes.Index(bytes.ToLower(doc), []byte("<svg")); i >= 0 {
		if e := bytes.IndexByte(doc[i:], '>'); e >= 0 {
			end = i + e
		}
	}
	if end < 0 {
		return 0, 0, false
	}
	tag := doc[:end]
	if w, h := widthAttr.FindSubmatch(tag), heightAttr.FindSubmatch(tag); w != nil && h != nil {
		wf, e1 := strconv.ParseFloat(string(w[1]), 64)
		hf, e2 := strconv.ParseFloat(string(h[1]), 64)
		if e1 == nil && e2 == nil && wf > 0 && hf > 0 {
			return wf, hf, true
		}
	}
	if m := viewBoxPattern.FindSubmatch(tag); m != nil {
		wf, e1 := strconv.ParseFloat(string(m[3]), 64)
		hf, e2 := strconv.ParseFloat(string(m[4]), 64)
		if e1 == nil && e2 == nil && wf > 0 && hf > 0 {
			return wf, hf, true
		}
	}
	return 0, 0, false
}

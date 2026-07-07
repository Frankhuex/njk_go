package uimage

import (
	"bytes"
	"image/gif"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func NormalizedExt(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err == nil && parsed.Path != "" {
		return strings.ToLower(path.Ext(parsed.Path))
	}
	return strings.ToLower(filepath.Ext(sourceURL))
}

func FallbackExt(sourceURL string, data []byte) string {
	contentType := strings.ToLower(http.DetectContentType(data))
	switch {
	case strings.Contains(contentType, "image/jpeg"), strings.Contains(contentType, "image/jpg"):
		return ".jpg"
	case strings.Contains(contentType, "image/png"):
		return ".png"
	}
	if ext := NormalizedExt(sourceURL); ext != "" {
		return ext
	}
	return ".png"
}

func FileSegmentNameFromData(index int, sourceURL string, data []byte) string {
	ext := FallbackExt(sourceURL, data)
	if _, err := gif.DecodeAll(bytes.NewReader(data)); err == nil {
		ext = ".gif"
	}
	return "image_" + strconvItoa(index+1) + ext
}

func FallbackFileSegmentName(index int, sourceURL string) string {
	ext := NormalizedExt(sourceURL)
	if ext == "" {
		ext = ".png"
	}
	return "image_" + strconvItoa(index+1) + ext
}

func strconvItoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := [20]byte{}
	pos := len(buf)
	for value > 0 {
		pos--
		buf[pos] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}


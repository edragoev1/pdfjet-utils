// main_test.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
)

// pngOf returns a PNG of w by h pixels.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// jpegOf returns a JPEG of w by h pixels, with the Exif orientation, 1 to
// 8, as a phone writes it, or none for 0.
func jpegOf(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	if orientation == 0 {
		return data
	}
	// APP1, Exif, a TIFF header of Motorola order and one IFD entry,
	// Orientation (0x0112), a SHORT
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1,
		0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, byte(orientation), 0, 0, 0, 0, 0, 0}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	app1 := append([]byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}, payload...)
	return append(append([]byte{0xFF, 0xD8}, app1...), data[2:]...)
}

const square = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100" height="100"><rect x="10" y="10" width="80" height="80" fill="#1f4e8c"/></svg>`

func TestPictureOf(t *testing.T) {
	cases := map[string][]byte{
		"jpeg": jpegOf(t, 4, 4, 0),
		"png":  pngOf(t, 4, 4),
		"bmp":  []byte("BM\x00\x00"),
		"svg":  []byte("<?xml version=\"1.0\"?>\n" + square),
	}
	for want, data := range cases {
		if p, err := pictureOf("x", data); err != nil || p.kind != want {
			t.Errorf("%s: %q %v", want, p.kind, err)
		}
	}
	refused := map[string][]byte{
		"HEIC": []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"),
		"WebP": []byte("RIFF\x00\x00\x00\x00WEBPVP8 "),
		"GIF":  []byte("GIF89a"),
		"PDF":  []byte("%PDF-1.7"),
		"not":  []byte("hello"),
	}
	for words, data := range refused {
		if _, err := pictureOf("x", data); err == nil || !strings.Contains(err.Error(), words) {
			t.Errorf("%s: %v", words, err)
		}
	}
}

// The page sizes of the PDF, in its order
func pageSizesOf(pdf []byte) []string {
	var sizes []string
	for _, m := range regexp.MustCompile(`/MediaBox \[0(?:\.0)? 0(?:\.0)? ([0-9.]+) ([0-9.]+)\]`).FindAllSubmatch(pdf, -1) {
		w, _ := strconv.ParseFloat(string(m[1]), 64)
		h, _ := strconv.ParseFloat(string(m[2]), 64)
		sizes = append(sizes, fmt.Sprintf("%.0fx%.0f", w, h))
	}
	return sizes
}

func TestRender(t *testing.T) {
	pictures := []picture{
		{name: "tall.png", data: pngOf(t, 40, 80), kind: "png"},
		{name: "wide.jpg", data: jpegOf(t, 80, 40, 0), kind: "jpeg"},
		{name: "logo.svg", data: []byte(square), kind: "svg"},
		// Stored wide, seen tall: the phone held upright (orientation 6)
		{name: "phone.jpg", data: jpegOf(t, 80, 40, 6), kind: "jpeg"},
	}
	pdf, err := render(pictures, "Scans", options{page: "letter", margin: 36, language: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(pageSizesOf(pdf), " ")
	if got != "612x792 792x612 612x792 612x792" {
		t.Errorf("pages %s", got)
	}
	for _, want := range []string{"pdfaid:part>3", "pdfuaid:part>1", "/S /Figure"} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Errorf("no %s", want)
		}
	}
	if _, err := render(nil, "x", options{page: "letter"}); err == nil {
		t.Error("no images made a PDF")
	}
	// A JPEG that does not hold is an error, not a panic
	bad := []picture{{name: "cut.jpg", data: jpegOf(t, 8, 8, 0)[:40], kind: "jpeg"}}
	if _, err := render(bad, "x", options{page: "letter"}); err == nil || !strings.Contains(err.Error(), "cut.jpg") {
		t.Errorf("a cut JPEG: %v", err)
	}
}

func TestJpegIsCMYK(t *testing.T) {
	if jpegIsCMYK(jpegOf(t, 4, 4, 6)) {
		t.Error("RGB taken as CMYK")
	}
	cmyk, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "Projects/pdfjet/resources/images/cmyk.jpg"))
	if err != nil {
		t.Skip("no CMYK JPEG here")
	}
	if !jpegIsCMYK(cmyk) {
		t.Error("CMYK not seen")
	}
	pdf, err := render([]picture{{name: "c.jpg", data: cmyk, kind: "jpeg"}}, "x", options{page: "a4", margin: 36})
	if err != nil || bytes.Contains(pdf, []byte("pdfaid:part")) || !bytes.Contains(pdf, []byte("pdfuaid:part>1")) {
		t.Errorf("a CMYK photo's PDF: %v", err)
	}
}

// A form of files, as the page posts it
func multipartOf(t *testing.T, fields map[string]string, files map[string][]byte, order []string) (string, []byte) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	for _, name := range order {
		part, _ := w.CreateFormFile("images", name)
		_, _ = part.Write(files[name])
	}
	_ = w.Close()
	return w.FormDataContentType(), b.Bytes()
}

func postFiles(t *testing.T, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	env := map[string]string{"REQUEST_METHOD": "POST", "CONTENT_TYPE": contentType, "CONTENT_LENGTH": strconv.Itoa(len(body))}
	var out bytes.Buffer
	cgiform.Write(&out, handleWeb(func(k string) string { return env[k] }, bytes.NewReader(body)))
	head, rest, _ := bytes.Cut(out.Bytes(), []byte("\r\n\r\n"))
	recorder := httptest.NewRecorder()
	status := 200
	for _, line := range strings.Split(string(head), "\r\n") {
		name, value, _ := strings.Cut(line, ": ")
		if name == "Status" {
			status, _ = strconv.Atoi(value[:3])
			continue
		}
		recorder.Header().Set(name, value)
	}
	recorder.WriteHeader(status)
	recorder.Write(rest)
	return recorder
}

func TestWeb(t *testing.T) {
	files := map[string][]byte{"b.png": pngOf(t, 80, 40), "a.svg": []byte(square)}
	ct, body := multipartOf(t, map[string]string{"page": "a4", "title": "My scans"}, files, []string{"b.png", "a.svg"})
	got := postFiles(t, ct, body)
	if got.Code != 200 || !bytes.HasPrefix(got.Body.Bytes(), []byte("%PDF-")) ||
		!strings.Contains(got.Header().Get("Content-Disposition"), "My scans.pdf") {
		t.Fatalf("%d %q %.200s", got.Code, got.Header().Get("Content-Disposition"), got.Body.String())
	}
	// In the order sent: the wide PNG first, on A4 turned sideways
	if sizes := pageSizesOf(got.Body.Bytes()); len(sizes) != 2 || sizes[0] != "842x595" {
		t.Errorf("pages %v", sizes)
	}
	cases := []struct {
		files  map[string][]byte
		fields map[string]string
		status int
		words  string
	}{
		{map[string][]byte{}, nil, 400, "no images"},
		{map[string][]byte{"p.heic": []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00")}, nil, 422, "HEIC"},
		{map[string][]byte{"a.png": pngOf(t, 4, 4)}, map[string]string{"page": "b5"}, 400, "page size"},
	}
	for _, c := range cases {
		var order []string
		for name := range c.files {
			order = append(order, name)
		}
		ct, body := multipartOf(t, c.fields, c.files, order)
		got := postFiles(t, ct, body)
		if got.Code != c.status || !strings.Contains(got.Body.String(), c.words) {
			t.Errorf("%v: %d %.300s", order, got.Code, got.Body.String())
		}
	}
	many := map[string][]byte{}
	var order []string
	for i := 0; i <= maxCount; i++ {
		name := fmt.Sprintf("%d.png", i)
		many[name] = pngOf(t, 2, 2)
		order = append(order, name)
	}
	ct, body = multipartOf(t, nil, many, order)
	if got := postFiles(t, ct, body); got.Code != 413 || !strings.Contains(got.Body.String(), "too many") {
		t.Errorf("too many: %d", got.Code)
	}
	env := map[string]string{"REQUEST_METHOD": "POST", "CONTENT_TYPE": ct, "CONTENT_LENGTH": strconv.Itoa(maxRequest + 1)}
	if a := handleWeb(func(k string) string { return env[k] }, bytes.NewReader(nil)); a.Status != 413 {
		t.Errorf("too large: %d", a.Status)
	}
}

// The program itself run as CGI, as the web server runs img2pdf.cgi
func TestRunAsCGI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the program")
	}
	program := filepath.Join(t.TempDir(), "img2pdf.cgi")
	build := exec.Command("go", "build", "-o", program, ".")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	handler := &cgi.Handler{Path: program, Root: "/img2pdf.cgi"}
	ct, body := multipartOf(t, nil, map[string][]byte{"x.png": pngOf(t, 8, 8)}, []string{"x.png"})
	request := httptest.NewRequest(http.MethodPost, "/img2pdf.cgi", bytes.NewReader(body))
	request.Header.Set("Content-Type", ct)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 || !bytes.HasPrefix(recorder.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("%d %.300s", recorder.Code, recorder.Body.String())
	}
}

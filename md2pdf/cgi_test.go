// cgi_test.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// post runs handleWeb, through writeAnswer and Go's own reading of CGI's
// answer, as the server would, on the fields as a form
func post(t *testing.T, handler http.Handler, fields url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/md2pdf.cgi", strings.NewReader(fields.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// direct is handleWeb as an http.Handler, its answer read back as the
// server reads a CGI program's
var direct = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	env := map[string]string{
		"REQUEST_METHOD": r.Method,
		"CONTENT_TYPE":   r.Header.Get("Content-Type"),
		"CONTENT_LENGTH": strconv.Itoa(len(body)),
	}
	var out bytes.Buffer
	writeAnswer(&out, handleWeb(func(k string) string { return env[k] }, bytes.NewReader(body)))
	head, rest, _ := bytes.Cut(out.Bytes(), []byte("\r\n\r\n"))
	status := http.StatusOK
	for _, line := range strings.Split(string(head), "\r\n") {
		name, value, _ := strings.Cut(line, ": ")
		if name == "Status" {
			status, _ = strconv.Atoi(value[:3])
			continue
		}
		w.Header().Set(name, value)
	}
	w.WriteHeader(status)
	_, _ = w.Write(rest)
})

func TestWebPDF(t *testing.T) {
	sample, err := os.ReadFile("../docs/sample.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, fields := range []url.Values{
		{"markdown": {string(sample)}},
		{"markdown": {string(sample)}, "page": {"a4"}, "font": {"serif"}, "lang": {"fr-CA"}},
	} {
		got := post(t, direct, fields)
		if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "application/pdf" ||
			!bytes.HasPrefix(got.Body.Bytes(), []byte("%PDF-")) {
			t.Fatalf("%v: %d %s %.200s", fields["page"], got.Code, got.Header().Get("Content-Type"), got.Body.String())
		}
		if cd := got.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, ".pdf") {
			t.Errorf("Content-Disposition %q", cd)
		}
		if got.Header().Get("Cache-Control") != "no-store" {
			t.Error("the PDF may be cached")
		}
	}
}

// An image is never read on the web, whatever its source: its text is drawn
// in its place, though the file is there beside the program
func TestWebReadsNoImage(t *testing.T) {
	text := "# Images\n\n![the sample](../docs/sample.png)\n\n![the sample](sample.png)\n\n![x](/etc/passwd)\n"
	dir := t.TempDir()
	png, err := os.ReadFile("../docs/sample.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	got := post(t, direct, url.Values{"markdown": {text}})
	if got.Code != http.StatusOK {
		t.Fatalf("%d %.200s", got.Code, got.Body.String())
	}
	if bytes.Contains(got.Body.Bytes(), []byte("/Subtype /Image")) {
		t.Error("an image was read")
	}
}

func TestWebRefusals(t *testing.T) {
	cases := []struct {
		fields url.Values
		status int
		words  string
	}{
		{url.Values{}, http.StatusBadRequest, "no text"},
		{url.Values{"markdown": {"  \n "}}, http.StatusBadRequest, "no text"},
		{url.Values{"markdown": {strings.Repeat("x", maxMarkdown+1)}}, http.StatusRequestEntityTooLarge, "256 KB"},
		{url.Values{"markdown": {strings.Repeat("é", maxRequest)}}, http.StatusRequestEntityTooLarge, "256 KB"},
		{url.Values{"markdown": {"# Hi"}, "page": {"b5"}}, http.StatusBadRequest, "page size"},
		{url.Values{"markdown": {"# Hi\xff"}}, http.StatusBadRequest, "UTF-8"},
	}
	for _, c := range cases {
		got := post(t, direct, c.fields)
		if got.Code != c.status || !strings.Contains(got.Body.String(), c.words) ||
			!strings.HasPrefix(got.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%.40v: %d %.300s", c.fields, got.Code, got.Body.String())
		}
	}
	// A message is escaped as HTML
	if got := post(t, direct, url.Values{"markdown": {"x"}, "page": {"<b>"}}); strings.Contains(got.Body.String(), "<b>") {
		t.Error("the page is not escaped")
	}
	request := httptest.NewRequest(http.MethodGet, "/md2pdf.cgi", nil)
	recorder := httptest.NewRecorder()
	direct(recorder, request)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != formPage {
		t.Errorf("GET: %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestFileName(t *testing.T) {
	for title, want := range map[string]string{
		"Release notes":         "Release notes",
		`a/b\c:d*e?f"g<h>i|j`:   "abcdefghij",
		"Ünïcode — Кирилица 漢字": "Ünïcode  Кирилица 漢字",
		"../..":                 "Document",
		"":                      "Document",
	} {
		if got := fileName(title); got != want {
			t.Errorf("fileName(%q) = %q, want %q", title, got, want)
		}
	}
}

// The program itself run as CGI, as the web server runs md2pdf.cgi: Go's
// cgi.Handler starts it with the variables of CGI, the request on its input
func TestRunAsCGI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the program")
	}
	program := filepath.Join(t.TempDir(), "md2pdf.cgi")
	build := exec.Command("go", "build", "-o", program, ".")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	handler := &cgi.Handler{Path: program, Root: "/md2pdf.cgi"}
	got := post(t, handler, url.Values{"markdown": {"# Hello\n\nA *PDF*, from the web."}, "name": {"hello.md"}})
	if got.Code != http.StatusOK || !bytes.HasPrefix(got.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("%d %.300s", got.Code, got.Body.String())
	}
	if cd := got.Header().Get("Content-Disposition"); !strings.Contains(cd, "Hello.pdf") {
		t.Errorf("Content-Disposition %q", cd)
	}
	if got := post(t, handler, url.Values{}); got.Code != http.StatusBadRequest {
		t.Errorf("no text: %d", got.Code)
	}
}

// main_test.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

package main

import (
	"bytes"
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

	"github.com/edragoev1/pdfjet/v9/src/errorcorrectionlevel"
)

func TestLinkOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://pdfjet.com":   "https://pdfjet.com",
		" pdfjet.com/forms ":   "https://pdfjet.com/forms",
		"www.example.com/menu": "https://www.example.com/menu",
		"http://example.com?x": "http://example.com?x",
		"mailto:a@example.com": "mailto:a@example.com",
	} {
		if got, err := linkOf(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "just words", "example"} {
		if _, err := linkOf(bad); err == nil {
			t.Errorf("%q taken", bad)
		}
	}
}

func TestWifiOf(t *testing.T) {
	cases := []struct {
		name, password, security string
		hidden                   bool
		want                     string
	}{
		{"Home", "secret", "WPA", false, "WIFI:T:WPA;S:Home;P:secret;;"},
		{`Café;1`, `p:a,s"s\`, "", true, `WIFI:T:WPA;S:Café\;1;P:p\:a\,s\"s\\;H:true;;`},
		{"Guest", "", "none", false, "WIFI:T:nopass;S:Guest;;"},
		{"Old", "12345", "wep", false, "WIFI:T:WEP;S:Old;P:12345;;"},
	}
	for _, c := range cases {
		if got, err := wifiOf(c.name, c.password, c.security, c.hidden); err != nil || got != c.want {
			t.Errorf("%q: %q %v", c.name, got, err)
		}
	}
	if _, err := wifiOf("", "x", "WPA", false); err == nil {
		t.Error("no name taken")
	}
	if _, err := wifiOf("Home", "", "WPA", false); err == nil {
		t.Error("WPA with no password taken")
	}
}

func TestVcardOf(t *testing.T) {
	got, err := vcardOf(contact{first: "Ana", last: "Ivanova", organization: "PDFjet; Software", phone: "+1 416 555 0100",
		email: "ana@example.com", website: "https://pdfjet.com"})
	want := "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Ivanova;Ana;;;\r\nFN:Ana Ivanova\r\nORG:PDFjet\\; Software\r\n" +
		"TEL;TYPE=CELL:+1 416 555 0100\r\nEMAIL:ana@example.com\r\nURL:https://pdfjet.com\r\nEND:VCARD"
	if err != nil || got != want {
		t.Errorf("%q %v", got, err)
	}
	if _, err := vcardOf(contact{phone: "1"}); err == nil {
		t.Error("no name taken")
	}
}

func TestEmailAndPhone(t *testing.T) {
	if got, _ := emailOf("a@example.com", "Hello there & more"); got != "mailto:a@example.com?subject=Hello%20there%20%26%20more" {
		t.Errorf("email %q", got)
	}
	if _, err := emailOf("not an address", ""); err == nil {
		t.Error("bad address taken")
	}
	if got, _ := phoneOf("+1 (416) 555-0100"); got != "tel:+14165550100" {
		t.Errorf("phone %q", got)
	}
	if _, err := phoneOf("12"); err == nil {
		t.Error("short number taken")
	}
}

func TestRender(t *testing.T) {
	for _, opts := range []options{
		{data: "https://pdfjet.com", caption: "pdfjet.com", level: errorcorrectionlevel.M, page: "letter"},
		{data: "https://pdfjet.com", sheet: true, size: 1, level: errorcorrectionlevel.H, page: "a4"},
		{data: "Привет, мир", caption: strings.Repeat("long words ", 30), level: errorcorrectionlevel.Q, page: "letter"},
	} {
		pdf, err := render(opts)
		if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("/S /Figure")) {
			t.Errorf("%+v: %v", opts.size, err)
		}
	}
	for _, bad := range []options{
		{data: "", level: errorcorrectionlevel.M},
		{data: strings.Repeat("x", 3000), level: errorcorrectionlevel.H},
		{data: "x", size: 0.2, level: errorcorrectionlevel.M},
		{data: "x", page: "legal", level: errorcorrectionlevel.M},
	} {
		if _, err := render(bad); err == nil {
			t.Errorf("%d bytes at %v made a code", len(bad.data), bad.size)
		}
	}
	if got, _ := render(options{data: strings.Repeat("x", 3000), level: errorcorrectionlevel.H}); got != nil {
		t.Error("too long")
	}
}

func post(t *testing.T, fields url.Values) (int, string, []byte) {
	t.Helper()
	body := fields.Encode()
	env := map[string]string{"REQUEST_METHOD": "POST", "CONTENT_TYPE": "application/x-www-form-urlencoded", "CONTENT_LENGTH": strconv.Itoa(len(body))}
	a := handleWeb(func(k string) string { return env[k] }, strings.NewReader(body))
	disposition := ""
	for _, h := range a.Headers {
		if h[0] == "Content-Disposition" {
			disposition = h[1]
		}
	}
	return a.Status, disposition, a.Body
}

func TestWeb(t *testing.T) {
	for _, fields := range []url.Values{
		{"kind": {"link"}, "link": {"pdfjet.com"}, "caption": {"pdfjet.com"}},
		{"kind": {"text"}, "text": {"Hello"}, "layout": {"sheet"}, "size": {"1.5"}, "page": {"a4"}},
		{"kind": {"wifi"}, "ssid": {"Home"}, "password": {"secret"}, "security": {"WPA"}, "level": {"H"}},
		{"kind": {"contact"}, "first": {"Ana"}, "phone": {"+1 416 555 0100"}},
		{"kind": {"email"}, "address": {"a@example.com"}},
		{"kind": {"phone"}, "phone": {"+1 416 555 0100"}},
	} {
		status, disposition, body := post(t, fields)
		if status != 200 || !bytes.HasPrefix(body, []byte("%PDF-")) || !strings.Contains(disposition, "QR code.pdf") {
			t.Errorf("%v: %d %.200s", fields, status, body)
		}
	}
	for _, c := range []struct {
		fields url.Values
		words  string
	}{
		{url.Values{"kind": {"link"}, "link": {"no link"}}, "not a link"},
		{url.Values{"kind": {"wifi"}, "ssid": {"Home"}}, "no password"},
		{url.Values{"kind": {"text"}, "text": {strings.Repeat("x", 3000)}, "level": {"H"}}, "too long"},
		{url.Values{"kind": {"link"}, "link": {"pdfjet.com"}, "size": {"20"}}, "size"},
		{url.Values{"kind": {"nope"}}, "kind"},
	} {
		if status, _, body := post(t, c.fields); status != 400 || !strings.Contains(string(body), c.words) {
			t.Errorf("%v: %d %.300s", c.fields, status, body)
		}
	}
}

// The program itself run as CGI, as the web server runs qr2pdf.cgi
func TestRunAsCGI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the program")
	}
	program := filepath.Join(t.TempDir(), "qr2pdf.cgi")
	build := exec.Command("go", "build", "-o", program, ".")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	handler := &cgi.Handler{Path: program, Root: "/qr2pdf.cgi"}
	request := httptest.NewRequest(http.MethodPost, "/qr2pdf.cgi", strings.NewReader("kind=link&link=pdfjet.com"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 || !bytes.HasPrefix(recorder.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("%d %.300s", recorder.Code, recorder.Body.String())
	}
}

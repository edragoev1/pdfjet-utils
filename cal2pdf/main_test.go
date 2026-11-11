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
	"time"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
)

func TestWeeks(t *testing.T) {
	// January 2027 starts on a Friday and has 31 days
	sunday := weeks(2027, time.January, 0)
	if len(sunday) != 6 || sunday[0] != [7]int{0, 0, 0, 0, 0, 1, 2} || sunday[5] != [7]int{31} {
		t.Errorf("from Sunday: %v", sunday)
	}
	monday := weeks(2027, time.January, 1)
	if len(monday) != 5 || monday[0] != [7]int{0, 0, 0, 0, 1, 2, 3} || monday[4] != [7]int{25, 26, 27, 28, 29, 30, 31} {
		t.Errorf("from Monday: %v", monday)
	}
	// February 2027 is four weeks from Monday; 2028 is a leap year
	if got := weeks(2027, time.February, 1); len(got) != 4 {
		t.Errorf("February 2027: %v", got)
	}
	if got := weeks(2028, time.February, 0); got[len(got)-1][2] != 29 {
		t.Errorf("February 2028: %v", got)
	}
}

func TestNames(t *testing.T) {
	for tag, n := range languageNames {
		for _, name := range append(n.months[:], n.days[:]...) {
			if strings.TrimSpace(name) == "" {
				t.Errorf("%s has an empty name", tag)
			}
		}
		if n.firstDay != 0 && n.firstDay != 1 && n.firstDay != 6 {
			t.Errorf("%s starts its weeks on %d", tag, n.firstDay)
		}
	}
	if languageNames["en-US"].firstDay != 0 || languageNames["en-GB"].firstDay != 1 {
		t.Error("the US weeks start on Sunday, the UK's on Monday")
	}
	if languageNames["el-GR"].months[0] != "Ιανουάριος" || languageNames["bg-BG"].months[0] != "Януари" {
		t.Error("a heading's month in the nominative")
	}
}

func TestRender(t *testing.T) {
	for _, opts := range []options{
		{year: 2027, page: "letter", language: "en-US", firstDay: -1},
		{year: 2027, page: "a4", language: "fr-FR", firstDay: 0, yearPage: true},
		{year: 2028, page: "letter", language: "sr-RS", firstDay: 1},
	} {
		pdf, err := render(opts)
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		pages := bytes.Count(pdf, []byte("/Type /Page\n")) + bytes.Count(pdf, []byte("/Type /Page>>")) + bytes.Count(pdf, []byte("/Type /Page "))
		want := 12
		if opts.yearPage {
			want = 1
		}
		if pages != want || !bytes.Contains(pdf, []byte("/S /Table")) {
			t.Errorf("%+v: %d pages", opts, pages)
		}
	}
	for _, bad := range []options{
		{year: 2027, page: "legal", language: "en-US"},
		{year: 2027, page: "letter", language: "xx"},
		{year: 1800, page: "letter", language: "en-US"},
	} {
		if _, err := render(bad); err == nil {
			t.Errorf("%+v made a calendar", bad)
		}
	}
}

func post(t *testing.T, fields url.Values) cgiform.Answer {
	t.Helper()
	body := fields.Encode()
	env := map[string]string{"REQUEST_METHOD": "POST", "CONTENT_TYPE": "application/x-www-form-urlencoded", "CONTENT_LENGTH": strconv.Itoa(len(body))}
	return handleWeb(func(k string) string { return env[k] }, strings.NewReader(body))
}

func TestWeb(t *testing.T) {
	got := post(t, url.Values{"year": {"2027"}, "layout": {"year"}, "page": {"a4"}, "lang": {"de-DE"}, "week": {"monday"}})
	if got.Status != 200 || !bytes.HasPrefix(got.Body, []byte("%PDF-")) {
		t.Fatalf("%d %.200s", got.Status, got.Body)
	}
	var disposition string
	for _, h := range got.Headers {
		if h[0] == "Content-Disposition" {
			disposition = h[1]
		}
	}
	if !strings.Contains(disposition, "Calendar 2027 (year).pdf") {
		t.Errorf("named %q", disposition)
	}
	for _, c := range []struct {
		fields url.Values
		words  string
	}{
		{url.Values{"year": {"x"}}, "The year"},
		{url.Values{"year": {"3000"}}, "The year"},
		{url.Values{"year": {"2027"}, "lang": {"xx"}}, "language"},
		{url.Values{"year": {"2027"}, "page": {"b5"}}, "page size"},
	} {
		if got := post(t, c.fields); got.Status != 400 || !strings.Contains(string(got.Body), c.words) {
			t.Errorf("%v: %d %.200s", c.fields, got.Status, got.Body)
		}
	}
}

// The program itself run as CGI, as the web server runs cal2pdf.cgi
func TestRunAsCGI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the program")
	}
	program := filepath.Join(t.TempDir(), "cal2pdf.cgi")
	build := exec.Command("go", "build", "-o", program, ".")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	handler := &cgi.Handler{Path: program, Root: "/cal2pdf.cgi"}
	request := httptest.NewRequest(http.MethodPost, "/cal2pdf.cgi", strings.NewReader("year=2027&lang=en-US"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 || !bytes.HasPrefix(recorder.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("%d %.300s", recorder.Code, recorder.Body.String())
	}
}

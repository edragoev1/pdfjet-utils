// main_test.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

package main

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
	pdfjet "github.com/edragoev1/pdfjet/v9/src"
	"github.com/edragoev1/pdfjet/v9/src/letter"
)

func TestReadRows(t *testing.T) {
	cases := []struct {
		name, data string
		want       [][]string
	}{
		{"commas and quotes", "a,\"b, c\",d\n1,\"2\"\"x\",3\n", [][]string{{"a", "b, c", "d"}, {"1", "2\"x", "3"}}},
		{"semicolons, as Excel writes them in Europe", "Name;Price\nTea;8,50\n", [][]string{{"Name", "Price"}, {"Tea", "8,50"}}},
		{"tabs", "a\tb\n1\t2\n", [][]string{{"a", "b"}, {"1", "2"}}},
		{"a byte order mark, CRLF", "\uFEFFa,b\r\n1,2\r\n", [][]string{{"a", "b"}, {"1", "2"}}},
		{"short rows filled, empty lines left out", "a,b,c\n1\n\n,,\n1,2,3,4\n", [][]string{{"a", "b", "c", ""}, {"1", "", "", ""}, {"1", "2", "3", "4"}}},
		{"a line break in quotes", "a,b\n\"one\ntwo\",2\n", [][]string{{"a", "b"}, {"one\ntwo", "2"}}},
	}
	for _, c := range cases {
		got, err := readRows([]byte(c.data), 0)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, %v", c.name, got, err)
		}
	}
	// A delimiter given wins over the header line's
	got, _ := readRows([]byte("a;b,c\n1;2,3\n"), ',')
	if !reflect.DeepEqual(got, [][]string{{"a;b", "c"}, {"1;2", "3"}}) {
		t.Errorf("given: %q", got)
	}
}

func TestDelimiterOf(t *testing.T) {
	for text, want := range map[string]rune{"tab": '\t', `\t`: '\t', ";": ';', "|": '|'} {
		if got, ok := delimiterOf(text); !ok || got != want {
			t.Errorf("%q: %q %v", text, got, ok)
		}
	}
	for _, text := range []string{"", "ab", `"`, "\n"} {
		if _, ok := delimiterOf(text); ok {
			t.Errorf("%q taken", text)
		}
	}
}

func options9() options {
	return options{pageSize: letter.Landscape(), header: true, size: 9, language: "en-US"}
}

func TestRenderIsATaggedTable(t *testing.T) {
	pdf, err := render([]byte("Name,Price\nTea,8.00\nCoffee,12.50\n"), "Prices", options9())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"%PDF-", "/S /Table", "/S /TH", "/S /TD"} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Errorf("no %s", want)
		}
	}
	if _, err := render([]byte("\n\n"), "x", options9()); err == nil {
		t.Error("no rows made a PDF")
	}
}

func TestFitSize(t *testing.T) {
	pdf := pdfjet.NewPDF(bufio.NewWriter(io.Discard))
	bold, _ := font(pdf, "IBMPlexSans-Bold.ttf", 9)
	regular, _ := font(pdf, "IBMPlexSans-Regular.ttf", 9)
	narrow := fitSize([]string{"a", "b"}, [][]string{{"1", "2"}}, bold, regular, options9())
	if narrow != 9 {
		t.Errorf("a narrow table at %v", narrow)
	}
	wide := make([]string, 12)
	for i := range wide {
		wide[i] = strings.Repeat("wide field ", 3)
	}
	size := fitSize(wide, nil, bold, regular, options9())
	if size >= 9 || size < minSize {
		t.Errorf("a wide table at %v", size)
	}
	huge := make([]string, 60)
	for i := range huge {
		huge[i] = strings.Repeat("x", 30)
	}
	if size := fitSize(huge, nil, bold, regular, options9()); size != minSize || minSize < 8 {
		t.Errorf("a huge table at %v", size)
	}
}

func TestColumns(t *testing.T) {
	if got, err := columnsOf(" 1, 3,5 "); err != nil || !reflect.DeepEqual(got, []int{0, 2, 4}) {
		t.Errorf("%v %v", got, err)
	}
	for _, bad := range []string{"", "0", "a", "1,-2"} {
		if _, err := columnsOf(bad); err == nil {
			t.Errorf("%q taken", bad)
		}
	}
	opts := options9()
	opts.columns = []int{2, 0}
	pdf, err := render([]byte("a,b,c\n1,2,3\n"), "x", opts)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal(err)
	}
	opts.columns = []int{5}
	if _, err := render([]byte("a,b\n1,2\n"), "x", opts); err == nil || !strings.Contains(err.Error(), "no column 6") {
		t.Errorf("a column past the table: %v", err)
	}
	got := post(t, url.Values{"csv": {"a,b,c\n1,2,3\n"}, "columns": {"1,3"}})
	if got.Code != 200 {
		t.Errorf("columns on the web: %d %.200s", got.Code, got.Body.String())
	}
	if got := post(t, url.Values{"csv": {"a,b"}, "columns": {"x"}}); got.Code != 400 {
		t.Errorf("columns x: %d", got.Code)
	}
}

func TestConvert(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sales.csv")
	if err := os.WriteFile(input, []byte("Region,Units\nNorth,120\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := convert(input, filepath.Join(dir, "sales.pdf"), options9()); err != nil {
		t.Fatal(err)
	}
	if err := convert(input, input, options9()); err == nil {
		t.Error("the PDF replaced the CSV")
	}
}

// post runs handleWeb on the fields as a form, its answer read back as the
// server reads a CGI program's
func post(t *testing.T, fields url.Values) *httptest.ResponseRecorder {
	t.Helper()
	body := fields.Encode()
	env := map[string]string{
		"REQUEST_METHOD": "POST",
		"CONTENT_TYPE":   "application/x-www-form-urlencoded",
		"CONTENT_LENGTH": strconv.Itoa(len(body)),
	}
	var out bytes.Buffer
	cgiform.Write(&out, handleWeb(func(k string) string { return env[k] }, strings.NewReader(body)))
	head, rest, _ := bytes.Cut(out.Bytes(), []byte("\r\n\r\n"))
	recorder := httptest.NewRecorder()
	status := http.StatusOK
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
	got := post(t, url.Values{"csv": {"Name;Price\nTea;8,50\n"}, "name": {"prices.csv"}, "page": {"a4"}})
	if got.Code != 200 || !bytes.HasPrefix(got.Body.Bytes(), []byte("%PDF-")) ||
		!strings.Contains(got.Header().Get("Content-Disposition"), "prices.pdf") {
		t.Fatalf("%d %q %.200s", got.Code, got.Header().Get("Content-Disposition"), got.Body.String())
	}
	cases := []struct {
		fields url.Values
		status int
		words  string
	}{
		{url.Values{}, 400, "no data"},
		{url.Values{"csv": {strings.Repeat("x", maxCSV+1)}}, 413, "512 KB"},
		{url.Values{"csv": {strings.Repeat("é", maxRequest)}}, 413, "512 KB"},
		{url.Values{"csv": {"a,b"}, "page": {"b5"}}, 400, "page size"},
		{url.Values{"csv": {"a,b"}, "delimiter": {"ab"}}, 400, "delimiter"},
		{url.Values{"csv": {"a,b\xff"}}, 400, "UTF-8"},
	}
	for _, c := range cases {
		got := post(t, c.fields)
		if got.Code != c.status || !strings.Contains(got.Body.String(), c.words) {
			t.Errorf("%.40v: %d %.200s", c.fields, got.Code, got.Body.String())
		}
	}
	// No header: the columns named
	got = post(t, url.Values{"csv": {"1,2\n3,4\n"}, "noheader": {"1"}})
	if got.Code != 200 {
		t.Errorf("no header: %d", got.Code)
	}
}

// The program itself run as CGI, as the web server runs csv2pdf.cgi
func TestRunAsCGI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the program")
	}
	program := filepath.Join(t.TempDir(), "csv2pdf.cgi")
	build := exec.Command("go", "build", "-o", program, ".")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	handler := &cgi.Handler{Path: program, Root: "/csv2pdf.cgi"}
	request := httptest.NewRequest(http.MethodPost, "/csv2pdf.cgi",
		strings.NewReader(url.Values{"csv": {"a,b\n1,2\n"}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 || !bytes.HasPrefix(recorder.Body.Bytes(), []byte("%PDF-")) ||
		!strings.Contains(recorder.Header().Get("Content-Disposition"), "Table.pdf") {
		t.Fatalf("%d %.300s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/csv2pdf.cgi", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 303 || recorder.Header().Get("Location") != "/csv-to-pdf" {
		t.Errorf("GET: %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

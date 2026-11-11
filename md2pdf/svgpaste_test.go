// svgpaste_test.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

package main

import (
	"bytes"
	"compress/zlib"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/edragoev1/pdfjet/v9/src/pagesize"
)

const blueSquare = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100" height="100">
  <title>Blue square</title>
  <rect x="10" y="10" width="80" height="80" fill="#1f4e8c"/>
</svg>`

// The image's text as PDFjet writes it, in UTF-16: Blue square
const blueSquareAlt = "/Alt <feff0042006c007500650020007300710075006100720065>"

func TestPastedSVGs(t *testing.T) {
	cases := []struct {
		text, want string
		files      int
	}{
		{"# Logo\n\n" + blueSquare + "\n\nAfter.", "![Blue square](svg-1.svg)", 1},
		{"<?xml version=\"1.0\"?>\n" + strings.Replace(blueSquare, "<title>Blue square</title>", "", 1), "![Image 1](svg-1.svg)", 1},
		{`<svg viewBox="0 0 10 10"><circle r="5"/></svg> and after`, "and after", 1},
		{"```\n" + blueSquare + "\n```", "<title>Blue square</title>", 0},
		{"<svg viewBox=\"0 0 10 10\">\nno end", "no end", 0},
		{"Text <svg> in a line", "Text <svg> in a line", 0},
		{strings.Replace(blueSquare, "Blue square", "A [b] c\\", 1), "![A (b) c/](svg-1.svg)", 1},
	}
	for _, c := range cases {
		got, files := pastedSVGs(c.text)
		if !strings.Contains(got, c.want) || len(files) != c.files {
			t.Errorf("%.40q: %d files, %q", c.text, len(files), got)
		}
	}
	two, files := pastedSVGs(blueSquare + "\n\ntext\n\n" + blueSquare)
	if len(files) != 2 || !strings.Contains(two, "svg-2.svg") {
		t.Errorf("two: %q", two)
	}
}

// The text of the PDF's streams, inflated
func inflated(pdf []byte) string {
	var all strings.Builder
	streams := regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`)
	for _, m := range streams.FindAllSubmatch(pdf, -1) {
		r, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(r)
		all.Write(data)
	}
	return all.String()
}

// On the web, a pasted SVG is drawn, as shapes and with its title for a
// screen reader, and its folder is gone after
func TestWebDrawsPastedSVG(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	got := post(t, direct, url.Values{"markdown": {"# Logo\n\n" + blueSquare + "\n\nThe end.\n"}})
	if got.Code != http.StatusOK {
		t.Fatalf("%d %.300s", got.Code, got.Body.String())
	}
	pdf := got.Body.Bytes()
	if !bytes.Contains(pdf, []byte(blueSquareAlt)) {
		t.Error("the SVG's title is not its text")
	}
	// The square in its blue, #1f4e8c
	if !strings.Contains(inflated(pdf), "0.12 0.31 0.55 rg") {
		t.Error("the SVG is not drawn")
	}
	if left, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "md2pdf-svg-*")); len(left) > 0 {
		t.Errorf("left behind: %v", left)
	}
	// Still no other image: one named beside the pasted one is not read
	got = post(t, direct, url.Values{"markdown": {blueSquare + "\n\n![x](../docs/sample.png)\n\n![y](svg-1.svg/../../x.png)\n"}})
	if got.Code != http.StatusOK || bytes.Contains(got.Body.Bytes(), []byte("/Subtype /Image")) {
		t.Errorf("%d, an image read", got.Code)
	}
}

// On the command line, a pasted SVG and the images beside the Markdown are
// drawn together, and the hidden folder is removed
func TestConvertDrawsPastedSVGAndImages(t *testing.T) {
	dir := t.TempDir()
	png, err := os.ReadFile("../docs/sample.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(dir, "doc.md")
	text := "# Doc\n\n" + blueSquare + "\n\n![A shot](shot.png)\n"
	if err := os.WriteFile(md, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := options{pageSize: mustPage(t), size: 11, language: "en-US"}
	if err := convert(md, filepath.Join(dir, "doc.pdf"), opts); err != nil {
		t.Fatal(err)
	}
	pdf, err := os.ReadFile(filepath.Join(dir, "doc.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/Subtype /Image")) || !bytes.Contains(pdf, []byte(blueSquareAlt)) {
		t.Error("the PNG and the SVG are not both drawn")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".md2pdf-svg-*")); len(left) > 0 {
		t.Errorf("left behind: %v", left)
	}
}

func mustPage(t *testing.T) pagesize.PageSize {
	t.Helper()
	size, ok := pageSizeOf("letter", false)
	if !ok {
		t.Fatal("no letter")
	}
	return size
}

// main_test.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edragoev1/pdfjet/v9/src"
	"github.com/edragoev1/pdfjet/v9/src/letter"
)

func TestConvertWritesAPDF(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "notes.md")
	text := "# Notes\n\nSome **bold**, *italic* and `code`.\n\n- one\n- two\n\n| A | B |\n|---|---|\n| 1 | 2 |\n"
	if err := os.WriteFile(input, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "notes.pdf")
	if err := convert(input, output, options{pageSize: letter.Portrait(), size: 11, language: "en-US"}); err != nil {
		t.Fatal(err)
	}
	pdf, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Errorf("not a PDF")
	}
	if err := convert(input, input, options{pageSize: letter.Portrait(), size: 11}); err == nil {
		t.Errorf("the Markdown file was replaced")
	}
}

// The text fonts kern, "AV" closer than A and V apart; the code font does
// not, its letters one width each.
func TestTheTextIsKernedAndTheCodeIsNot(t *testing.T) {
	pdf := pdfjet.NewPDF(bufio.NewWriter(io.Discard))
	for _, c := range []struct {
		name   string
		kerned bool
	}{
		{"IBMPlexSans-Regular.ttf", true},
		{"IBMPlexSans-Bold.ttf", true},
		{"IBMPlexSans-Italic.ttf", true},
		{"IBMPlexSans-BoldItalic.ttf", true},
		{"IBMPlexMono-Regular.ttf", false},
	} {
		f, err := font(pdf, c.name, 11)
		if err != nil {
			t.Fatal(err)
		}
		apart := f.StringWidth(f.GetSize(), "A") + f.StringWidth(f.GetSize(), "V")
		if together := f.StringWidth(f.GetSize(), "AV"); (together < apart) != c.kerned {
			t.Errorf("%s: AV %v, A and V %v", c.name, together, apart)
		}
	}
}

func TestTitleOf(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"# Notes\n\ntext", "Notes"},
		{"intro\n\n## Two\n# One #\n", "One"},
		{"```\n# not a heading\n```\n# Real", "Real"},
		{"no heading", "file"},
	} {
		if got := titleOf(c.text, "dir/file.md"); got != c.want {
			t.Errorf("%q: %q, want %q", c.text, got, c.want)
		}
	}
}

func TestVersionNamesMd2pdfAndGo(t *testing.T) {
	text := versionText()
	if !strings.HasPrefix(text, "md2pdf ") || !strings.Contains(text, "go1.") {
		t.Errorf("%q", text)
	}
}

func TestPageSizeOf(t *testing.T) {
	for _, c := range []struct {
		name          string
		landscape     bool
		width, height float32
	}{
		{"letter", false, 612, 792},
		{"Letter", true, 792, 612},
		{"legal", false, 612, 1008},
		{"tabloid", true, 1224, 792},
		{"a4", false, 595, 842},
		{"A4", true, 842, 595},
		{"a3", false, 842, 1191},
		{"a5", true, 595, 420},
	} {
		size, ok := pageSizeOf(c.name, c.landscape)
		if !ok || int(size.GetWidth()) != int(c.width) || int(size.GetHeight()) != int(c.height) {
			t.Errorf("%s, landscape %v: %v x %v, %v", c.name, c.landscape, size.GetWidth(), size.GetHeight(), ok)
		}
	}
	if _, ok := pageSizeOf("b4", false); ok {
		t.Error("b4 is a page size")
	}
}

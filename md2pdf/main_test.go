// main_test.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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

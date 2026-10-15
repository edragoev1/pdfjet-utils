// main.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// md2pdf converts Markdown files to PDF with PDFjet's Markdown: headings,
// paragraphs with bold, italic and code, lists, quotes, code blocks, tables
// and images, in IBM Plex Sans and IBM Plex Mono, which are built into the
// program. The PDFs are PDF/UA, tagged for screen readers, with a page number
// at the foot of every page.
//
//	md2pdf notes.md                  writes notes.pdf, next to notes.md
//	md2pdf -o out.pdf notes.md       writes out.pdf
//	md2pdf -page a4 *.md             writes a PDF for each, on A4 pages
//
// Images are read from the directory of the Markdown file, and from nowhere
// else.
package main

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	pdfjet "github.com/edragoev1/pdfjet/v9/src"
	"github.com/edragoev1/pdfjet/v9/src/a4"
	"github.com/edragoev1/pdfjet/v9/src/compliance"
	"github.com/edragoev1/pdfjet/v9/src/letter"
	"github.com/edragoev1/pdfjet/v9/src/pagesize"
)

// The colors, as GitHub shows Markdown: dark blue headings, blue links, and
// green boxes of the tasks that are done.
const (
	headingColor = 0x1F3A5F
	linkColor    = 0x0969DA
	checkColor   = 0x1A7F37
)

//go:embed fonts/*.ttf
var fonts embed.FS

// options are the choices of the command line.
type options struct {
	output   string
	pageSize pagesize.PageSize
	size     float32
	language string
}

func main() {
	output := flag.String("o", "", "the PDF to write, for a single Markdown file (default: the .md file's name with .pdf)")
	page := flag.String("page", "letter", "the page size: letter or a4")
	size := flag.Float64("size", 11, "the size of the text, in points")
	language := flag.String("lang", "en-US", "the language of the text, such as en-US, fr-CA or de-DE")
	showVersion := flag.Bool("version", false, "print the version of md2pdf and of PDFjet, and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: md2pdf [-o file.pdf] [-page letter|a4] [-size 11] [-lang en-US] file.md ...\n       md2pdf --version")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(versionText())
		return
	}
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *output != "" && flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "md2pdf: -o names one PDF, for one Markdown file")
		os.Exit(2)
	}
	opts := options{output: *output, size: float32(*size), language: *language}
	switch strings.ToLower(*page) {
	case "letter":
		opts.pageSize = letter.Portrait()
	case "a4":
		opts.pageSize = a4.Portrait()
	default:
		fmt.Fprintln(os.Stderr, "md2pdf: the page is letter or a4, not "+*page)
		os.Exit(2)
	}
	if opts.size < 6 || opts.size > 24 {
		fmt.Fprintln(os.Stderr, "md2pdf: the size is from 6 to 24 points")
		os.Exit(2)
	}

	failed := false
	for _, input := range flag.Args() {
		pdfPath := opts.output
		if pdfPath == "" {
			pdfPath = strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
		}
		if err := convert(input, pdfPath, opts); err != nil {
			fmt.Fprintln(os.Stderr, "md2pdf: "+input+": "+err.Error())
			failed = true
			continue
		}
		fmt.Println(input + " => " + pdfPath)
	}
	if failed {
		os.Exit(1)
	}
}

// convert writes the PDF of the Markdown file. The PDF is written in memory
// first, so that a failed conversion leaves no half-written file.
func convert(input, pdfPath string, opts options) error {
	text, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	if filepath.Clean(input) == filepath.Clean(pdfPath) {
		return errors.New("the PDF would replace the Markdown file")
	}

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	pdf := pdfjet.NewPDF(writer)
	pdf.SetCompliance(compliance.PDF_UA_1)
	pdf.SetTitle(titleOf(string(text), input))
	pdf.SetLanguage(opts.language)

	regular, err := font(pdf, "IBMPlexSans-Regular.ttf", opts.size)
	if err != nil {
		return err
	}
	bold, err := font(pdf, "IBMPlexSans-Bold.ttf", opts.size)
	if err != nil {
		return err
	}
	italic, err := font(pdf, "IBMPlexSans-Italic.ttf", opts.size)
	if err != nil {
		return err
	}
	boldItalic, err := font(pdf, "IBMPlexSans-BoldItalic.ttf", opts.size)
	if err != nil {
		return err
	}
	code, err := font(pdf, "IBMPlexMono-Regular.ttf", opts.size*0.86)
	if err != nil {
		return err
	}

	markdown := pdfjet.NewMarkdown(regular, bold, italic, boldItalic, code).
		SetHeadingColor(headingColor).
		SetLinkColor(linkColor).
		SetCheckColor(checkColor)
	markdown.SetImageDirectory(filepath.Dir(input))
	pages := make([]*pdfjet.Page, 0)
	markdown.DrawOnPages(pdf, string(text), &pages, opts.pageSize)
	for i, page := range pages {
		number := pdfjet.NewTextLine(regular, strconv.Itoa(i+1))
		number.SetFontSize(9)
		page.AddFooterOffsetBy(number, 36)
	}
	pdf.AddPages(pages)
	if err := pdf.Complete(); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return os.WriteFile(pdfPath, buffer.Bytes(), 0o644)
}

// font returns the built-in font, at the size.
func font(pdf *pdfjet.PDF, name string, size float32) (*pdfjet.Font, error) {
	data, err := fonts.ReadFile("fonts/" + name)
	if err != nil {
		return nil, err
	}
	return pdfjet.NewFont(pdf, bytes.NewReader(data)).SetSize(size), nil
}

// titleOf returns the title of the document: its first heading of #, not in
// a block of code, or the name of the file without .md.
func titleOf(text, input string) string {
	inCode := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inCode = !inCode
		}
		if !inCode && strings.HasPrefix(line, "# ") {
			if title := strings.TrimSpace(strings.Trim(line[2:], "#")); title != "" {
				return title
			}
		}
	}
	return strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
}

// versionText returns the version of md2pdf and of the PDFjet it is built
// on, as Go records them in the program: the tag of a release, such as
// v1.0.5, for go install and for build.sh at a tag, and a pseudo-version
// of the commit otherwise.
func versionText() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "md2pdf (version unknown)"
	}
	version := info.Main.Version
	if version == "" {
		version = "(devel)"
	}
	text := "md2pdf " + version
	for _, module := range info.Deps {
		if module.Path == "github.com/edragoev1/pdfjet/v9" {
			pdfjetVersion := module.Version
			if module.Replace != nil {
				pdfjetVersion = module.Replace.Version
				if pdfjetVersion == "" {
					pdfjetVersion = "(local)"
				}
			}
			text += ", built on PDFjet " + pdfjetVersion
		}
	}
	return text + ", " + info.GoVersion
}

// main.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// md2pdf converts Markdown files to PDF with PDFjet's Markdown: headings,
// paragraphs with bold, italic and code, lists, quotes, code blocks, tables
// and images, in IBM Plex Sans or Serif and Cascadia Mono, which are built into the
// program. The PDFs are PDF/UA, tagged for screen readers, with a page number
// at the foot of every page.
//
//	md2pdf notes.md                  writes notes.pdf, next to notes.md
//	md2pdf -o out.pdf notes.md       writes out.pdf
//	md2pdf -page a4 *.md             writes a PDF for each, on A4 pages
//	md2pdf -landscape wide.md        writes wide.pdf, on letter pages turned sideways
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
	"github.com/edragoev1/pdfjet/v9/src/a3"
	"github.com/edragoev1/pdfjet/v9/src/a4"
	"github.com/edragoev1/pdfjet/v9/src/a5"
	"github.com/edragoev1/pdfjet/v9/src/compliance"
	"github.com/edragoev1/pdfjet/v9/src/legal"
	"github.com/edragoev1/pdfjet/v9/src/letter"
	"github.com/edragoev1/pdfjet/v9/src/pagesize"
	"github.com/edragoev1/pdfjet/v9/src/tabloid"
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
	serif    bool
	language string
}

// The page sizes, by name: PDFjet's, upright and turned sideways.
var pageSizes = map[string][2]func() pagesize.PageSize{
	"letter":  {letter.Portrait, letter.Landscape},
	"legal":   {legal.Portrait, legal.Landscape},
	"tabloid": {tabloid.Portrait, tabloid.Landscape},
	"a3":      {a3.Portrait, a3.Landscape},
	"a4":      {a4.Portrait, a4.Landscape},
	"a5":      {a5.Portrait, a5.Landscape},
}

const pageNames = "letter, legal, tabloid, a3, a4 or a5"

// pageSizeOf returns the page size of a name, in any case, upright or
// turned sideways, and whether there is one of that name.
func pageSizeOf(name string, landscape bool) (pagesize.PageSize, bool) {
	sizes, ok := pageSizes[strings.ToLower(name)]
	if !ok {
		return pagesize.PageSize{}, false
	}
	if landscape {
		return sizes[1](), true
	}
	return sizes[0](), true
}

func main() {
	output := flag.String("o", "", "the PDF to write, for a single Markdown file (default: the .md file's name with .pdf)")
	page := flag.String("page", "letter", "the page size: "+pageNames)
	landscape := flag.Bool("landscape", false, "the pages turned sideways, wider than they are tall")
	size := flag.Float64("size", 11, "the size of the text, in points")
	serif := flag.Bool("serif", false, "the text in IBM Plex Serif, not IBM Plex Sans")
	language := flag.String("lang", "en-US", "the language of the text, such as en-US, fr-CA or de-DE")
	showVersion := flag.Bool("version", false, "print the version of md2pdf and of PDFjet, and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: md2pdf [-o file.pdf] [-page letter|legal|tabloid|a3|a4|a5] [-landscape] [-serif] [-size 11] [-lang en-US] file.md ...\n       md2pdf --version")
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
	opts := options{output: *output, size: float32(*size), serif: *serif, language: *language}
	pageSize, ok := pageSizeOf(*page, *landscape)
	if !ok {
		fmt.Fprintln(os.Stderr, "md2pdf: the page is "+pageNames+", not "+*page)
		os.Exit(2)
	}
	opts.pageSize = pageSize
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

	family := "IBMPlexSans"
	if opts.serif {
		family = "IBMPlexSerif"
	}
	regular, err := font(pdf, family+"-Regular.ttf", opts.size)
	if err != nil {
		return err
	}
	bold, err := font(pdf, family+"-Bold.ttf", opts.size)
	if err != nil {
		return err
	}
	italic, err := font(pdf, family+"-Italic.ttf", opts.size)
	if err != nil {
		return err
	}
	boldItalic, err := font(pdf, family+"-BoldItalic.ttf", opts.size)
	if err != nil {
		return err
	}
	code, err := font(pdf, "CascadiaMono-SemiLight.ttf", opts.size*0.86)
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

// font returns the built-in font, at the size, kerned as the font's own
// kerning says, as PDFjet does since 9.1.5; but the code font, whose
// letters keep one width each, so that the columns of code stay in line.
func font(pdf *pdfjet.PDF, name string, size float32) (*pdfjet.Font, error) {
	data, err := fonts.ReadFile("fonts/" + name)
	if err != nil {
		return nil, err
	}
	f := pdfjet.NewFont(pdf, bytes.NewReader(data)).SetSize(size)
	if !strings.Contains(name, "Mono") {
		f.SetKernPairs(true)
	}
	return f, nil
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

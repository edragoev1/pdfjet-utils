// main.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// csv2pdf converts a CSV file to a PDF table with PDFjet's BigTable: its
// first row the header, on every page, the numbers right-aligned, every other
// row shaded, and "Page 1 of N" under it, in IBM Plex Sans, which is built
// into the program. The PDFs are PDF/UA, the table tagged as a table for
// screen readers. The fields are read as RFC 4180 has them, a delimiter or a
// line break inside quotes kept; the delimiter, a comma, a semicolon, a tab
// or a bar, is the one of the header line, unless it is given.
//
//	csv2pdf data.csv                 writes data.pdf, next to data.csv
//	csv2pdf -o out.pdf data.csv      writes out.pdf
//	csv2pdf -page a4 *.csv           writes a PDF for each, on A4 pages
//	csv2pdf -portrait narrow.csv     upright pages, not turned sideways
//	csv2pdf -columns 1,2,5 wide.csv  only the first, second and fifth columns
//
// Run by a web server, as csv2pdf.cgi on pdfjet.com, it answers the form of
// the page CSV to PDF (cgi.go).
package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
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

//go:embed fonts/*.ttf
var fonts embed.FS

// The colors of the table: the header's, as PDFjet's examples have it
const headerColor = 0xDDE3EA

// options are the choices of the command line and of the form.
type options struct {
	pageSize  pagesize.PageSize
	delimiter rune  // 0 for the one of the header line
	header    bool  // The first row is the header; Column 1, 2... when not
	columns   []int // The columns drawn, counted from 0, in their order; nil for all
	size      float32
	language  string
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

// pageSizeOf returns the page size of a name, in any case, upright or turned
// sideways, and whether there is one of that name.
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
	if cgiform.Running() {
		cgiform.Serve(handleWeb)
		return
	}
	output := flag.String("o", "", "the PDF to write, for a single CSV file (default: the .csv file's name with .pdf)")
	page := flag.String("page", "letter", "the page size: "+pageNames)
	portrait := flag.Bool("portrait", false, "upright pages; a table is on pages turned sideways unless this is given")
	delimiter := flag.String("d", "", "the delimiter, such as , or ; or tab (default: the one of the header line)")
	noHeader := flag.Bool("no-header", false, "the first row is data, not the header: the columns are named Column 1, 2...")
	size := flag.Float64("size", 9, "the size of the text, in points")
	columnList := flag.String("columns", "", "the columns drawn, counted from 1, such as 1,3,5 (default: all)")
	language := flag.String("lang", "en-US", "the language of the text, such as en-US, fr-CA or de-DE")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: csv2pdf [options] file.csv...\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *output != "" && flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "csv2pdf: -o is for one file only")
		os.Exit(2)
	}
	pageSize, ok := pageSizeOf(*page, !*portrait)
	if !ok {
		fmt.Fprintf(os.Stderr, "csv2pdf: the page size %q is not one of %s\n", *page, pageNames)
		os.Exit(2)
	}
	opts := options{pageSize: pageSize, header: !*noHeader, size: float32(*size), language: *language}
	if *delimiter != "" {
		d, ok := delimiterOf(*delimiter)
		if !ok {
			fmt.Fprintf(os.Stderr, "csv2pdf: the delimiter %q is not one character\n", *delimiter)
			os.Exit(2)
		}
		opts.delimiter = d
	}
	if *columnList != "" {
		columns, err := columnsOf(*columnList)
		if err != nil {
			fmt.Fprintln(os.Stderr, "csv2pdf:", err)
			os.Exit(2)
		}
		opts.columns = columns
	}
	failed := false
	for _, input := range flag.Args() {
		pdfPath := *output
		if pdfPath == "" {
			pdfPath = strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
		}
		if err := convert(input, pdfPath, opts); err != nil {
			fmt.Fprintf(os.Stderr, "csv2pdf: %s: %v\n", input, err)
			failed = true
			continue
		}
		fmt.Printf("%s => %s\n", input, pdfPath)
	}
	if failed {
		os.Exit(1)
	}
}

// delimiterOf returns the delimiter a name or a character gives: tab, or \t,
// or one character.
func delimiterOf(text string) (rune, bool) {
	switch strings.ToLower(text) {
	case "tab", `\t`:
		return '\t', true
	}
	r := []rune(text)
	if len(r) != 1 || r[0] == '"' || r[0] == '\n' || r[0] == '\r' {
		return 0, false
	}
	return r[0], true
}

// convert writes the PDF of the CSV file. The PDF is made in memory first,
// so that a failed conversion leaves no half-written file.
func convert(input, pdfPath string, opts options) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	if filepath.Clean(input) == filepath.Clean(pdfPath) {
		return errors.New("the PDF would replace the CSV file")
	}
	title := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	pdf, err := render(data, title, opts)
	if err != nil {
		return err
	}
	return os.WriteFile(pdfPath, pdf, 0o644)
}

// guessDelimiter returns the delimiter of the header line: the one of a
// comma, a semicolon, a tab and a bar that it has most of, outside quotes,
// or a comma.
func guessDelimiter(data []byte) rune {
	line := data
	if i := bytes.IndexAny(line, "\r\n"); i >= 0 {
		line = line[:i]
	}
	counts := map[rune]int{}
	quoted := false
	for _, r := range string(line) {
		if r == '"' {
			quoted = !quoted
		} else if !quoted && strings.ContainsRune(",;\t|", r) {
			counts[r]++
		}
	}
	best, most := ',', 0
	for _, r := range []rune{',', ';', '\t', '|'} {
		if counts[r] > most {
			best, most = r, counts[r]
		}
	}
	return best
}

// readRows returns the rows of the CSV, as RFC 4180 has them, each as long
// as the longest, the short ones filled with empty fields; the byte order
// mark at its start left out.
func readRows(data []byte, delimiter rune) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\uFEFF"))
	if delimiter == 0 {
		delimiter = guessDelimiter(data)
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	var rows [][]string
	width := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		// A line of nothing but delimiters is no row
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		rows = append(rows, row)
		width = max(width, len(row))
	}
	for i, row := range rows {
		for len(row) < width {
			row = append(row, "")
		}
		rows[i] = row
	}
	return rows, nil
}

// render returns the PDF of the CSV, with the title.
func render(data []byte, title string, opts options) ([]byte, error) {
	rows, err := readRows(data, opts.delimiter)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("there are no rows")
	}
	header := rows[0]
	body := rows[1:]
	if !opts.header {
		header = make([]string, len(rows[0]))
		for i := range header {
			header[i] = fmt.Sprintf("Column %d", i+1)
		}
		body = rows
	}

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	pdf := pdfjet.NewPDF(writer)
	pdf.SetCompliance(compliance.PDF_UA_1)
	pdf.SetTitle(title)
	pdf.SetLanguage(opts.language)
	bold, err := font(pdf, "IBMPlexSans-Bold.ttf", opts.size)
	if err != nil {
		return nil, err
	}
	regular, err := font(pdf, "IBMPlexSans-Regular.ttf", opts.size)
	if err != nil {
		return nil, err
	}
	// A table wider than the page is set smaller to fit it, down to a size
	// still read: BigTable cuts the last columns of one still too wide
	columns := opts.columns
	if columns == nil {
		for i := range header {
			columns = append(columns, i)
		}
	}
	for _, c := range columns {
		if c < 0 || c >= len(header) {
			return nil, fmt.Errorf("there is no column %d: the table has %d", c+1, len(header))
		}
	}
	size := fitSize(pick(header, columns), pickRows(body, columns), bold, regular, opts)
	bold.SetSize(size)
	regular.SetSize(size)

	table := pdfjet.NewBigTable(pdf, bold, regular, opts.pageSize)
	table.SetColumns(columns...)
	table.SetTableRows(header, func(yield func([]string) bool) {
		for _, row := range body {
			if !yield(row) {
				return
			}
		}
	})
	table.SetLocation(margin, margin)
	table.SetBottomMargin(margin)
	table.SetPadding(padding)
	table.SetVerticalPadding(2)
	table.SetHeaderColor(headerColor)
	table.SetFooterFontSize(min(8, size))
	table.SetFooterOffset(18)
	if err := table.Complete(); err != nil {
		return nil, err
	}
	if err := pdf.Complete(); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// The least size the text of a table is set at to fit the page, in points:
// no smaller, so that it is read with ease (the owner, 10 November 2026: not
// too small; landscape, and fewer columns); one still too wide has its last
// columns cut, and the page says to choose fewer
const minSize = 8

// The table's margins and the padding of its cells, in points
const (
	margin  = 36
	padding = 4
)

// fitSize returns the size the table's text is set at: the size asked, or
// less, down to minSize, as a table as wide as its widest fields fits the
// page between its margins. The widths grow with the size, so the size that
// fits is the one asked times the room over the width at it.
func fitSize(header []string, body [][]string, bold, regular *pdfjet.Font, opts options) float32 {
	widths := make([]float32, len(header))
	for i, field := range header {
		widths[i] = bold.StringWidth(opts.size, field)
	}
	for _, row := range body {
		for i, field := range row {
			if i < len(widths) {
				widths[i] = max(widths[i], regular.StringWidth(opts.size, field))
			}
		}
	}
	var total float32
	for _, w := range widths {
		total += w
	}
	room := opts.pageSize.GetWidth() - 2*margin - float32(len(widths))*2*padding
	if total <= room || total == 0 {
		return opts.size
	}
	return max(minSize, opts.size*room/total)
}

// pick returns the fields of the columns, in their order.
func pick(row []string, columns []int) []string {
	fields := make([]string, len(columns))
	for i, c := range columns {
		fields[i] = row[c]
	}
	return fields
}

func pickRows(rows [][]string, columns []int) [][]string {
	picked := make([][]string, len(rows))
	for i, row := range rows {
		picked[i] = pick(row, columns)
	}
	return picked
}

// columnsOf returns the columns a list names, counted from 1 and separated
// by commas, as counted from 0: "1,3,5" is 0, 2 and 4.
func columnsOf(list string) ([]int, error) {
	var columns []int
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("%q is not the number of a column", part)
		}
		columns = append(columns, n-1)
	}
	if len(columns) == 0 {
		return nil, errors.New("no column is chosen")
	}
	return columns, nil
}

// font returns the built-in font, at the size, kerned as the font's own
// kerning says.
func font(pdf *pdfjet.PDF, name string, size float32) (*pdfjet.Font, error) {
	data, err := fonts.ReadFile("fonts/" + name)
	if err != nil {
		return nil, err
	}
	return pdfjet.NewFont(pdf, bytes.NewReader(data)).SetSize(size).SetKernPairs(true), nil
}

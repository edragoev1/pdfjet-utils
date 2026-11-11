// main.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// cal2pdf makes a printable calendar of a year: a month a page, on pages
// turned sideways, with room to write in each day, or the whole year on one
// page. The months and the days are named in the language chosen, as CLDR
// names them (names.go), and the weeks start on its first day, or on the one
// chosen. The PDF is PDF/UA, each month a table tagged for screen readers.
//
//	cal2pdf 2027                     writes calendar-2027.pdf, a month a page
//	cal2pdf -year-page 2027          the year on one page
//	cal2pdf -page a4 -lang fr-FR 2027
//	cal2pdf -monday 2027             the weeks from Monday
//
// Run by a web server, as cal2pdf.cgi on pdfjet.com, it answers the form of
// the page Printable Calendar (cgi.go).
package main

import (
	"bufio"
	"bytes"
	"embed"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
	"github.com/edragoev1/pdfjet-utils/internal/version"
	pdfjet "github.com/edragoev1/pdfjet/v9/src"
	"github.com/edragoev1/pdfjet/v9/src/a4"
	"github.com/edragoev1/pdfjet/v9/src/alignment"
	"github.com/edragoev1/pdfjet/v9/src/compliance"
	"github.com/edragoev1/pdfjet/v9/src/letter"
	"github.com/edragoev1/pdfjet/v9/src/pagesize"
)

//go:embed fonts/*.ttf
var fonts embed.FS

// calendarNames are a language's names of the months and the days.
type calendarNames struct {
	label    string
	firstDay int // 0 Sunday, 1 Monday
	months   [12]string
	days     [7]string // From Sunday
	letters  [7]string // From Sunday
}

// options are the choices of the command line and of the form.
type options struct {
	year     int
	yearPage bool   // The year on one page, not a month a page
	page     string // letter or a4
	language string // A key of languageNames
	firstDay int    // 0 Sunday, 1 Monday, -1 the language's
}

// The colors: the header's and the weekend's shading, the lines, and the
// gray of the days of other months and of the footer
const (
	headerColor  = 0xDDE3EA
	weekendColor = 0xF3F5F8
	lineColor    = 0xB8BFCC
	titleColor   = 0x1F3A5F
	footerColor  = 0x6B7280
	margin       = 36
)

// The years a calendar may be of
const (
	firstYear = 1900
	lastYear  = 2200
)

func main() {
	if cgiform.Running() {
		cgiform.Serve(handleWeb)
		return
	}
	output := flag.String("o", "", "the PDF to write (default: calendar-YEAR.pdf)")
	yearPage := flag.Bool("year-page", false, "the whole year on one page, not a month a page")
	page := flag.String("page", "letter", "the page size: letter or a4")
	language := flag.String("lang", "en-US", "the language of the names: "+strings.Join(languageTags(), ", "))
	monday := flag.Bool("monday", false, "the weeks from Monday (default: the language's first day)")
	sunday := flag.Bool("sunday", false, "the weeks from Sunday")
	showVersion := flag.Bool("version", false, "print the version of cal2pdf and of PDFjet, and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: cal2pdf [options] year\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Text("cal2pdf"))
		return
	}
	year := time.Now().Year() + 1
	if flag.NArg() > 0 {
		y, err := strconv.Atoi(flag.Arg(0))
		if err != nil || y < firstYear || y > lastYear {
			fmt.Fprintf(os.Stderr, "cal2pdf: the year is a number from %d to %d\n", firstYear, lastYear)
			os.Exit(2)
		}
		year = y
	}
	opts := options{year: year, yearPage: *yearPage, page: strings.ToLower(*page), language: *language, firstDay: -1}
	if *monday {
		opts.firstDay = 1
	} else if *sunday {
		opts.firstDay = 0
	}
	pdf, err := render(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cal2pdf:", err)
		os.Exit(1)
	}
	path := *output
	if path == "" {
		path = fmt.Sprintf("calendar-%d.pdf", year)
	}
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "cal2pdf:", err)
		os.Exit(1)
	}
	fmt.Println(path)
}

// languageTags returns the languages cal2pdf has, in order.
func languageTags() []string {
	tags := make([]string, 0, len(languageNames))
	for tag := range languageNames {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// pageSizeOf returns the page size, upright or turned sideways.
func pageSizeOf(name string, landscape bool) (pagesize.PageSize, bool) {
	sizes := map[string][2]func() pagesize.PageSize{
		"letter": {letter.Portrait, letter.Landscape},
		"a4":     {a4.Portrait, a4.Landscape},
	}[name]
	if sizes[0] == nil {
		return pagesize.PageSize{}, false
	}
	if landscape {
		return sizes[1](), true
	}
	return sizes[0](), true
}

// weeks returns the weeks of the month, each of seven days, 0 for a day of
// another month, from the first day of the week.
func weeks(year int, month time.Month, firstDay int) [][7]int {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	days := first.AddDate(0, 1, -1).Day()
	column := (int(first.Weekday()) - firstDay + 7) % 7
	var out [][7]int
	var week [7]int
	for day := 1; day <= days; day++ {
		week[column] = day
		column++
		if column == 7 {
			out = append(out, week)
			week, column = [7]int{}, 0
		}
	}
	if column > 0 {
		out = append(out, week)
	}
	return out
}

// render returns the PDF of the calendar.
func render(opts options) ([]byte, error) {
	names, ok := languageNames[opts.language]
	if !ok {
		return nil, fmt.Errorf("the language %q is not one of %s", opts.language, strings.Join(languageTags(), ", "))
	}
	if opts.year < firstYear || opts.year > lastYear {
		return nil, fmt.Errorf("the year is a number from %d to %d", firstYear, lastYear)
	}
	firstDay := opts.firstDay
	if firstDay < 0 {
		firstDay = names.firstDay
	}
	size, ok := pageSizeOf(opts.page, !opts.yearPage)
	if !ok {
		return nil, fmt.Errorf("the page size %q is not letter or a4", opts.page)
	}

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	pdf := pdfjet.NewPDF(writer)
	pdf.SetCompliance(compliance.PDF_UA_1)
	pdf.SetTitle(fmt.Sprintf("%d", opts.year))
	pdf.SetLanguage(opts.language)
	regular, err := font(pdf, "IBMPlexSans-Regular.ttf")
	if err != nil {
		return nil, err
	}
	bold, err := font(pdf, "IBMPlexSans-Bold.ttf")
	if err != nil {
		return nil, err
	}
	c := calendar{pdf: pdf, regular: regular, bold: bold, names: names, firstDay: firstDay, year: opts.year, size: size}
	if opts.yearPage {
		c.yearPage()
	} else {
		for m := time.January; m <= time.December; m++ {
			c.monthPage(m)
		}
	}
	if err := pdf.Complete(); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// calendar draws the pages of a year.
type calendar struct {
	pdf           *pdfjet.PDF
	regular, bold *pdfjet.Font
	names         calendarNames
	firstDay      int
	year          int
	size          pagesize.PageSize
}

// footer says where the calendar came from, small, at the foot of the page,
// so that one printed and passed on tells where to make another
func (c calendar) footer(page *pdfjet.Page) {
	line := pdfjet.NewTextLine(c.regular, "Made at pdfjet.com/printable-calendar")
	line.SetFontSize(7)
	line.SetTextColor(footerColor)
	page.AddFooterOffsetBy(line, 18)
}

// dayName returns the name of the day of the week of the column.
func (c calendar) dayName(column int, letters bool) string {
	day := (column + c.firstDay) % 7
	if letters {
		return c.names.letters[day]
	}
	return c.names.days[day]
}

// weekend tells whether the column is Saturday or Sunday.
func (c calendar) weekend(column int) bool {
	day := (column + c.firstDay) % 7
	return day == 0 || day == 6
}

// monthPage draws the month on a page of its own: its name and the year,
// and a table of its weeks, as tall as the page lets them be, to write in.
func (c calendar) monthPage(month time.Month) {
	page := pdfjet.NewPage(c.pdf, c.size)
	width := page.GetWidth() - 2*margin
	title := pdfjet.NewTextLine(c.bold, fmt.Sprintf("%s %d", c.names.months[month-1], c.year))
	title.SetFontSize(26)
	title.SetTextColor(titleColor)
	title.SetLocation(margin, margin+26)
	title.DrawOn(page)

	top := float32(margin + 44)
	header := float32(26)
	rows := weeks(c.year, month, c.firstDay)
	rowHeight := (page.GetHeight() - margin - top - header) / float32(len(rows))
	column := width / 7

	data := make([][]*pdfjet.Cell, 0, len(rows)+1)
	head := make([]*pdfjet.Cell, 7)
	for i := range head {
		cell := pdfjet.NewCell(c.bold, c.dayName(i, false))
		cell.SetFontSize(11)
		cell.SetPadding(5)
		cell.SetTextAlignment(alignment.Center)
		cell.SetBackgroundColor(headerColor)
		cell.SetBorderColor(lineColor)
		head[i] = cell
	}
	data = append(data, head)
	for _, week := range rows {
		row := make([]*pdfjet.Cell, 7)
		for i, day := range week {
			text := ""
			if day > 0 {
				text = strconv.Itoa(day)
			}
			cell := pdfjet.NewCell(c.regular, text)
			cell.SetFontSize(14)
			cell.SetPadding(6)
			cell.SetTopPadding(5)
			cell.SetBottomPadding(rowHeight - 14*1.3 - 5)
			cell.SetBorderColor(lineColor)
			if c.weekend(i) {
				cell.SetBackgroundColor(weekendColor)
			}
			row[i] = cell
		}
		data = append(data, row)
	}
	table := pdfjet.NewTable()
	table.SetTableData(data, 1)
	for i := 0; i < 7; i++ {
		table.SetColumnWidth(i, column)
	}
	table.SetLocation(margin, top)
	table.DrawOn(page)
	c.footer(page)
}

// yearPage draws the twelve months on one page, three across and four
// down, each a small table under its name.
func (c calendar) yearPage() {
	page := pdfjet.NewPage(c.pdf, c.size)
	title := pdfjet.NewTextLine(c.bold, strconv.Itoa(c.year))
	title.SetFontSize(30)
	title.SetTextColor(titleColor)
	title.SetLocation(margin, margin+30)
	title.DrawOn(page)

	gap := float32(22)
	top := float32(margin + 52)
	monthWidth := (page.GetWidth() - 2*margin - 2*gap) / 3
	monthHeight := (page.GetHeight() - margin - top - 3*gap) / 4
	column := monthWidth / 7
	for m := 0; m < 12; m++ {
		x := margin + float32(m%3)*(monthWidth+gap)
		y := top + float32(m/3)*(monthHeight+gap)
		name := pdfjet.NewTextLine(c.bold, c.names.months[m])
		name.SetFontSize(12)
		name.SetTextColor(titleColor)
		name.SetLocation(x, y+12)
		name.DrawOn(page)

		data := [][]*pdfjet.Cell{}
		head := make([]*pdfjet.Cell, 7)
		for i := range head {
			cell := pdfjet.NewCell(c.bold, c.dayName(i, true))
			cell.SetFontSize(8)
			cell.SetTextAlignment(alignment.Center)
			cell.SetBackgroundColor(headerColor)
			cell.SetBorderColor(lineColor)
			head[i] = cell
		}
		data = append(data, head)
		rows := weeks(c.year, time.Month(m+1), c.firstDay)
		// Six rows of weeks for every month, so that the months line up
		for len(rows) < 6 {
			rows = append(rows, [7]int{})
		}
		rowPadding := (monthHeight-20-16)/6 - 8*1.3
		for _, week := range rows {
			row := make([]*pdfjet.Cell, 7)
			for i, day := range week {
				text := ""
				if day > 0 {
					text = strconv.Itoa(day)
				}
				cell := pdfjet.NewCell(c.regular, text)
				cell.SetFontSize(8)
				cell.SetTextAlignment(alignment.Center)
				cell.SetTopPadding(rowPadding / 2)
				cell.SetBottomPadding(rowPadding / 2)
				cell.SetBorderColor(lineColor)
				if c.weekend(i) {
					cell.SetBackgroundColor(weekendColor)
				}
				row[i] = cell
			}
			data = append(data, row)
		}
		table := pdfjet.NewTable()
		table.SetTableData(data, 1)
		for i := 0; i < 7; i++ {
			table.SetColumnWidth(i, column)
		}
		table.SetLocation(x, y+20)
		table.DrawOn(page)
	}
	c.footer(page)
}

// font returns the built-in font, kerned as its own kerning says.
func font(pdf *pdfjet.PDF, name string) (*pdfjet.Font, error) {
	data, err := fonts.ReadFile("fonts/" + name)
	if err != nil {
		return nil, err
	}
	return pdfjet.NewFont(pdf, bytes.NewReader(data)).SetSize(10).SetKernPairs(true), nil
}

// cgi.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.
//
// csv2pdf on the web: the same program, run by the web server as
// csv2pdf.cgi, makes the PDF of the CSV that pdfjet.com's page CSV to PDF
// (/csv-to-pdf) sends, and answers with it, the same PDF as on the command
// line. The text is read, made into a PDF and sent back; it is kept nowhere
// and written in no log. It is limited in size, and the work in time, as the
// host is shared. CGI is spoken by hand, in internal/cgiform, which md2pdf
// shares.

package main

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
)

const (
	// The CSV's size at most: some ten thousand rows of a short table
	maxCSV = 512 << 10
	// The request's, its fields encoded as a form, which may triple the text
	maxRequest = 3*maxCSV + 4096
	// The time a PDF may take, well past what the largest text needs
	maxTime = 20 * time.Second
)

// The page the form is on, which the errors link back to
var formPage = cgiform.Page{Path: "/csv-to-pdf", Title: "CSV to PDF"}

// A language tag as BCP 47 writes it, simply: en, en-US, zh-Hant-TW
var languageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)

// handleWeb answers the form, its variables read by env and its body from
// in: the PDF of its CSV, or a short page that says what is wrong.
func handleWeb(env func(string) string, in io.Reader) cgiform.Answer {
	if env("REQUEST_METHOD") != "POST" {
		return formPage.Redirect()
	}
	tooLong := formPage.Error(413, "The data is too long: CSV to PDF takes 512 KB at most.")
	form, err := cgiform.ReadForm(env, in, maxRequest)
	if errors.As(err, new(cgiform.ErrTooLarge)) {
		return tooLong
	} else if err != nil {
		return formPage.Error(400, "The form could not be read. Try again from the page.")
	}

	text := form.Get("csv")
	if len(text) > maxCSV {
		return tooLong
	}
	if strings.TrimSpace(text) == "" {
		return formPage.Error(400, "There is no data to make a PDF of: paste your CSV, or choose a .csv file.")
	}
	if !utf8.ValidString(text) {
		return formPage.Error(400, "The data is not UTF-8: save the file as UTF-8 (in Excel, \"CSV UTF-8\") and choose it again.")
	}

	opts := options{size: 9, language: "en-US", header: form.Get("noheader") != "1"}
	page := form.Get("page")
	if page == "" {
		page = "letter"
	}
	size, ok := pageSizeOf(page, form.Get("portrait") != "1")
	if !ok {
		return formPage.Error(400, "That page size is not one of "+pageNames+".")
	}
	opts.pageSize = size
	if d := form.Get("delimiter"); d != "" && d != "auto" {
		r, ok := delimiterOf(d)
		if !ok {
			return formPage.Error(400, "The delimiter is not one character.")
		}
		opts.delimiter = r
	}
	if list := form.Get("columns"); list != "" {
		columns, err := columnsOf(list)
		if err != nil {
			return formPage.Error(400, "The columns chosen could not be read: "+err.Error()+".")
		}
		opts.columns = columns
	}
	if lang := form.Get("lang"); languageTag.MatchString(lang) {
		opts.language = lang
	}
	title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(form.Get("name"), ".csv"), ".txt"))
	if title == "" {
		title = "Table"
	}

	type result struct {
		pdf []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- result{err: fmt.Errorf("%v", p)}
			}
		}()
		pdf, err := render([]byte(text), title, opts)
		done <- result{pdf, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			return formPage.Error(422, "The PDF could not be made: "+res.err.Error())
		}
		return cgiform.PDF(res.pdf, title)
	case <-time.After(maxTime):
		return formPage.Error(503, "The PDF took too long to make. Try fewer rows.")
	}
}

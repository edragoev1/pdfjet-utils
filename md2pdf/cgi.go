// cgi.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.
//
// md2pdf on the web: the same program, run by the web server as
// md2pdf.cgi, makes the PDF of the Markdown that pdfjet.com's page
// Markdown to PDF (/markdown-to-pdf) sends, and answers with it, the same PDF
// as on the command line. The text is read, made into a PDF and sent back;
// it is kept nowhere and written in no log.
//
// CGI is spoken by hand, in internal/cgiform, which csv2pdf shares.
//
// It reads no image: a text sent has no folder of its own, so render is
// given none, and PDFjet draws an image's text in its place. A process is
// started for each request, as CGI does, which a Go program does in a few
// milliseconds; the text is limited in size, and the work in time, as the
// host is shared.

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
	// The Markdown's size at most: a long document, some 50 pages
	maxMarkdown = 256 << 10
	// The request's, its fields encoded as a form, which may triple the text
	maxRequest = 3*maxMarkdown + 4096
	// The time a PDF may take, well past what the largest text needs
	maxTime = 20 * time.Second
)

// The page the form is on, which the errors link back to
var formPage = cgiform.Page{Path: "/markdown-to-pdf", Title: "Markdown to PDF"}

// A language tag as BCP 47 writes it, simply: en, en-US, zh-Hant-TW
var languageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)

func serveCGI() {
	cgiform.Serve(handleWeb)
}

// handleWeb answers the form, its variables read by env and its body from
// in: the PDF of its text, or a short page that says what is wrong.
func handleWeb(env func(string) string, in io.Reader) cgiform.Answer {
	if env("REQUEST_METHOD") != "POST" {
		return formPage.Redirect()
	}
	tooLong := formPage.Error(413, "The text is too long: Markdown to PDF takes 256 KB at most.")
	form, err := cgiform.ReadForm(env, in, maxRequest)
	if errors.As(err, new(cgiform.ErrTooLarge)) {
		return tooLong
	} else if err != nil {
		return formPage.Error(400, "The form could not be read. Try again from the page.")
	}

	text := form.Get("markdown")
	if len(text) > maxMarkdown {
		return tooLong
	}
	if strings.TrimSpace(text) == "" {
		return formPage.Error(400, "There is no text to make a PDF of: paste your Markdown, or choose a .md file.")
	}
	if !utf8.ValidString(text) {
		return formPage.Error(400, "The text is not UTF-8: save the file as UTF-8 and choose it again.")
	}

	opts := options{size: 11, language: "en-US"}
	page := form.Get("page")
	if page == "" {
		page = "letter"
	}
	size, ok := pageSizeOf(page, form.Get("landscape") == "1")
	if !ok {
		return formPage.Error(400, "That page size is not one of "+pageNames+".")
	}
	opts.pageSize = size
	opts.serif = form.Get("font") == "serif"
	if lang := form.Get("lang"); languageTag.MatchString(lang) {
		opts.language = lang
	}
	title := titleOf(text, strings.TrimSuffix(form.Get("name"), ".md"))
	if strings.TrimSpace(title) == "" {
		title = "Document"
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
		pdf, err := render([]byte(text), title, "", opts)
		done <- result{pdf, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			return formPage.Error(422, "The PDF could not be made: "+res.err.Error())
		}
		return cgiform.PDF(res.pdf, title)
	case <-time.After(maxTime):
		return formPage.Error(503, "The PDF took too long to make. Try a shorter text.")
	}
}

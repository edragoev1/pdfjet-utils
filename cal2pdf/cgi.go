// cgi.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.
//
// cal2pdf on the web: the same program, run by the web server as
// cal2pdf.cgi, makes the calendar that pdfjet.com's page Printable Calendar
// (/printable-calendar) asks for, and answers with it. Nothing is sent but
// the choices, and nothing is kept. CGI is spoken by hand, in
// internal/cgiform, which the other programs share.

package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
)

// The page the form is on, which the errors link back to
var formPage = cgiform.Page{Path: "/printable-calendar", Title: "Printable Calendar"}

// handleWeb answers the form, its variables read by env and its body from
// in: the PDF of the calendar, or a short page that says what is wrong.
func handleWeb(env func(string) string, in io.Reader) cgiform.Answer {
	if env("REQUEST_METHOD") != "POST" {
		return formPage.Redirect()
	}
	form, err := cgiform.ReadForm(env, in, 4096)
	if errors.As(err, new(cgiform.ErrTooLarge)) || err != nil {
		return formPage.Error(400, "The form could not be read. Try again from the page.")
	}
	year, err := strconv.Atoi(form.Get("year"))
	if err != nil || year < firstYear || year > lastYear {
		return formPage.Error(400, fmt.Sprintf("The year is a number from %d to %d.", firstYear, lastYear))
	}
	opts := options{year: year, yearPage: form.Get("layout") == "year", page: strings.ToLower(form.Get("page")),
		language: form.Get("lang"), firstDay: -1}
	if opts.page == "" {
		opts.page = "letter"
	}
	if opts.language == "" {
		opts.language = "en-US"
	}
	if _, ok := languageNames[opts.language]; !ok {
		return formPage.Error(400, "That language is not one Printable Calendar has.")
	}
	switch form.Get("week") {
	case "sunday":
		opts.firstDay = 0
	case "monday":
		opts.firstDay = 1
	}
	done := make(chan cgiform.Answer, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- formPage.Error(422, fmt.Sprintf("The calendar could not be made: %v.", p))
			}
		}()
		pdf, err := render(opts)
		if err != nil {
			done <- formPage.Error(400, "The calendar could not be made: "+err.Error()+".")
			return
		}
		name := fmt.Sprintf("Calendar %d", year)
		if opts.yearPage {
			name += " (year)"
		}
		done <- cgiform.PDF(pdf, name)
	}()
	select {
	case answer := <-done:
		return answer
	case <-time.After(20 * time.Second):
		return formPage.Error(503, "The calendar took too long to make. Try again.")
	}
}

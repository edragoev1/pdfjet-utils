// cgi.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.
//
// img2pdf on the web: the same program, run by the web server as
// img2pdf.cgi, makes the PDF of the images that pdfjet.com's page Images to
// PDF (/images-to-pdf) sends, in their order, and answers with it. The images
// are read, made into a PDF and sent back; they are kept nowhere and written
// in no log. They are limited in number and size, and the work in time, as
// the host is shared. CGI is spoken by hand, in internal/cgiform, which
// md2pdf and csv2pdf share.

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
)

const (
	// The images' size together at most, and so the request's, with room
	// for the form's fields and the parts' headers
	maxImages  = 20 << 20
	maxRequest = maxImages + 64<<10
	// The images one PDF may have
	maxCount = 50
	// The time a PDF may take, well past what the largest images need
	maxTime = 30 * time.Second
)

// The page the form is on, which the errors link back to
var formPage = cgiform.Page{Path: "/images-to-pdf", Title: "Images to PDF"}

// A language tag as BCP 47 writes it, simply: en, en-US, zh-Hant-TW
var languageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)

// handleWeb answers the form, its variables read by env and its body from
// in: the PDF of its images, or a short page that says what is wrong.
func handleWeb(env func(string) string, in io.Reader) cgiform.Answer {
	if env("REQUEST_METHOD") != "POST" {
		return formPage.Redirect()
	}
	form, files, err := cgiform.ReadFiles(env, in, maxRequest, "images", maxCount)
	switch {
	case errors.As(err, new(cgiform.ErrTooLarge)):
		return formPage.Error(413, "The images are too large together: Images to PDF takes 20 MB at most. Choose fewer, or smaller ones.")
	case errors.As(err, new(cgiform.ErrTooMany)):
		return formPage.Error(413, fmt.Sprintf("There are too many images: Images to PDF takes %d at most.", maxCount))
	case err != nil:
		return formPage.Error(400, "The form could not be read. Try again from the page.")
	}
	if len(files) == 0 {
		return formPage.Error(400, "There are no images to make a PDF of: choose one or more.")
	}

	var pictures []picture
	for _, f := range files {
		p, err := pictureOf(f.Name, f.Data)
		if err != nil {
			return formPage.Error(422, capitalized(err.Error())+".")
		}
		pictures = append(pictures, p)
	}
	if tooManyPixels(pictures) {
		return formPage.Error(413, fmt.Sprintf("The PNG images are too large together: Images to PDF takes %d million "+
			"pixels of PNG at most (a phone's screenshot is about 3 million). Choose fewer, or save them as JPEG.",
			maxPNGPixels/1_000_000))
	}
	opts := options{page: strings.ToLower(form.Get("page")), margin: 36, language: "en-US"}
	if opts.page == "" {
		opts.page = "letter"
	}
	if _, ok := pageSizes[opts.page]; !ok {
		return formPage.Error(400, "That page size is not one of "+pageNames+".")
	}
	if form.Get("margin") == "0" {
		opts.margin = 0
	}
	if lang := form.Get("lang"); languageTag.MatchString(lang) {
		opts.language = lang
	}
	title := strings.TrimSpace(form.Get("title"))
	if title == "" {
		title = "Images"
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
		pdf, err := render(pictures, title, opts)
		done <- result{pdf, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			return formPage.Error(422, "The PDF could not be made: "+res.err.Error()+".")
		}
		return cgiform.PDF(res.pdf, title)
	case <-time.After(maxTime):
		return formPage.Error(503, "The PDF took too long to make. Try fewer images.")
	}
}

// The most pixels of PNG a form may bring, all its PNGs together: a PNG is
// unpacked to its pixels and packed again, which takes the time, and the
// Worker that answers the form on Cloudflare has 15 seconds of it; a JPEG
// goes into the PDF as it is, and costs next to nothing (21 November 2026)
const maxPNGPixels = 150_000_000

// tooManyPixels tells whether the PNGs have more pixels together than
// maxPNGPixels, as their headers say.
func tooManyPixels(pictures []picture) bool {
	var total uint64
	for _, p := range pictures {
		if p.kind == "png" && len(p.data) >= 24 {
			w := uint64(binary.BigEndian.Uint32(p.data[16:20]))
			h := uint64(binary.BigEndian.Uint32(p.data[20:24]))
			total += w * h
			if total > maxPNGPixels {
				return true
			}
		}
	}
	return false
}

func capitalized(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// cgi.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.
//
// qr2pdf on the web: the same program, run by the web server as
// qr2pdf.cgi, makes the QR code that pdfjet.com's page QR Code Generator
// (/qr-code) asks for, and answers with its PDF. What the code holds is sent
// to make it and kept nowhere. CGI is spoken by hand, in internal/cgiform,
// which the other programs share.

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
var formPage = cgiform.Page{Path: "/qr-code", Title: "QR Code Generator"}

// handleWeb answers the form, its variables read by env and its body from
// in: the PDF of the code, or a short page that says what is wrong.
func handleWeb(env func(string) string, in io.Reader) cgiform.Answer {
	if env("REQUEST_METHOD") != "POST" {
		return formPage.Redirect()
	}
	form, err := cgiform.ReadForm(env, in, 64<<10)
	if errors.As(err, new(cgiform.ErrTooLarge)) {
		return formPage.Error(413, "The text is too long for a QR code.")
	} else if err != nil {
		return formPage.Error(400, "The form could not be read. Try again from the page.")
	}
	var data, alt string
	switch form.Get("kind") {
	case "link", "":
		data, err = linkOf(form.Get("link"))
		alt = "QR code of the link " + data
	case "text":
		data = form.Get("text")
		if strings.TrimSpace(data) == "" {
			err = fmt.Errorf("there is no text")
		}
		alt = "QR code of a text"
	case "wifi":
		data, err = wifiOf(form.Get("ssid"), form.Get("password"), form.Get("security"), form.Get("hidden") == "1")
		alt = "QR code to join the Wi-Fi network " + form.Get("ssid")
	case "contact":
		data, err = vcardOf(contact{first: form.Get("first"), last: form.Get("last"), organization: form.Get("org"),
			title: form.Get("title"), phone: form.Get("phone"), email: form.Get("email"), website: form.Get("website")})
		alt = "QR code of the contact card of " + strings.TrimSpace(form.Get("first")+" "+form.Get("last")+" "+form.Get("org"))
	case "email":
		data, err = emailOf(form.Get("address"), form.Get("subject"))
		alt = "QR code to email " + form.Get("address")
	case "phone":
		data, err = phoneOf(form.Get("phone"))
		alt = "QR code to call " + form.Get("phone")
	default:
		return formPage.Error(400, "That kind of code is not one the page makes.")
	}
	if err != nil {
		return formPage.Error(400, capitalized(err.Error())+".")
	}
	level, ok := levels[strings.ToUpper(form.Get("level"))]
	if !ok {
		level = levels["M"]
	}
	opts := options{data: data, caption: strings.TrimSpace(form.Get("caption")), alt: alt,
		sheet: form.Get("layout") == "sheet", level: level, page: strings.ToLower(form.Get("page"))}
	if s := form.Get("size"); s != "" {
		size, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return formPage.Error(400, "The size is a number of inches.")
		}
		opts.size = float32(size)
	}
	if len([]rune(opts.caption)) > 200 {
		return formPage.Error(400, "The words under the code are too long: 200 characters at most.")
	}
	done := make(chan cgiform.Answer, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- formPage.Error(422, fmt.Sprintf("The code could not be made: %v.", p))
			}
		}()
		pdf, err := render(opts)
		if err != nil {
			done <- formPage.Error(400, capitalized(err.Error())+".")
			return
		}
		done <- cgiform.PDF(pdf, "QR code")
	}()
	select {
	case answer := <-done:
		return answer
	case <-time.After(20 * time.Second):
		return formPage.Error(503, "The code took too long to make. Try again.")
	}
}

func capitalized(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

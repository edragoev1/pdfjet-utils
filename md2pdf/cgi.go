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
// CGI is spoken here by hand, its variables and the request's body in, the
// headers and the answer out: Go's net/http would make the program four
// times larger for everyone who downloads md2pdf, for what a form needs,
// which net/url reads.
//
// It reads no image: a text sent has no folder of its own, so render is
// given none, and PDFjet draws an image's text in its place. A process is
// started for each request, as CGI does, which a Go program does in a few
// milliseconds; the text is limited in size, and the work in time, as the
// host is shared.

package main

import (
	"bufio"
	"fmt"
	"html"
	"io"
	"mime"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// The Markdown's size at most: a long document, some 50 pages
	maxMarkdown = 256 << 10
	// The request's, its fields encoded as a form, which may triple the text
	maxRequest = 3*maxMarkdown + 4096
	// The time a PDF may take, well past what the largest text needs
	maxTime = 20 * time.Second
	// The page the form is on, which the errors link back to
	formPage = "/markdown-to-pdf"
)

// A language tag as BCP 47 writes it, simply: en, en-US, zh-Hant-TW
var languageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)

// webAnswer is what the program answers: a status, headers and a body.
type webAnswer struct {
	status  int
	headers [][2]string
	body    []byte
}

func serveCGI() {
	answer := handleWeb(os.Getenv, os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	writeAnswer(out, answer)
	if err := out.Flush(); err != nil {
		os.Exit(1)
	}
}

// writeAnswer writes the answer as CGI has it: Status, the headers, an
// empty line, the body.
func writeAnswer(w io.Writer, a webAnswer) {
	fmt.Fprintf(w, "Status: %d %s\r\n", a.status, statusText[a.status])
	for _, h := range a.headers {
		fmt.Fprintf(w, "%s: %s\r\n", h[0], h[1])
	}
	fmt.Fprintf(w, "Cache-Control: no-store\r\nX-Content-Type-Options: nosniff\r\n")
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(a.body))
	_, _ = w.Write(a.body)
}

var statusText = map[int]string{
	200: "OK", 303: "See Other", 400: "Bad Request", 413: "Content Too Large",
	422: "Unprocessable Content", 503: "Service Unavailable",
}

// handleWeb answers the form, its variables read by env and its body from
// in: the PDF of its text, or a short page that says what is wrong.
func handleWeb(env func(string) string, in io.Reader) webAnswer {
	if env("REQUEST_METHOD") != "POST" {
		return webAnswer{status: 303, headers: [][2]string{{"Location", formPage}}}
	}
	unread := webError(400, "The form could not be read. Try again from the page.")
	if mediaType, _, _ := mime.ParseMediaType(env("CONTENT_TYPE")); mediaType != "application/x-www-form-urlencoded" {
		return unread
	}
	length, err := strconv.Atoi(env("CONTENT_LENGTH"))
	if err != nil || length < 0 {
		return unread
	}
	if length > maxRequest {
		return webError(413, "The text is too long: Markdown to PDF takes 256 KB at most.")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(in, body); err != nil {
		return unread
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return unread
	}

	text := form.Get("markdown")
	if len(text) > maxMarkdown {
		return webError(413, "The text is too long: Markdown to PDF takes 256 KB at most.")
	}
	if strings.TrimSpace(text) == "" {
		return webError(400, "There is no text to make a PDF of: paste your Markdown, or choose a .md file.")
	}
	if !utf8.ValidString(text) {
		return webError(400, "The text is not UTF-8: save the file as UTF-8 and choose it again.")
	}

	opts := options{size: 11, language: "en-US"}
	page := form.Get("page")
	if page == "" {
		page = "letter"
	}
	size, ok := pageSizeOf(page, form.Get("landscape") == "1")
	if !ok {
		return webError(400, "That page size is not one of "+pageNames+".")
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
			return webError(422, "The PDF could not be made: "+res.err.Error())
		}
		return webAnswer{status: 200, body: res.pdf, headers: [][2]string{
			{"Content-Type", "application/pdf"},
			{"Content-Disposition", mime.FormatMediaType("attachment",
				map[string]string{"filename": fileName(title) + ".pdf"})},
		}}
	case <-time.After(maxTime):
		return webError(503, "The PDF took too long to make. Try a shorter text.")
	}
}

// fileName returns the title as a file's name: its letters and digits, of
// any language, its spaces and a few marks, the rest left out, and no more
// than 80 characters.
func fileName(title string) string {
	var b strings.Builder
	count := 0
	for _, r := range title {
		if count == 80 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" -_.,()'", r) {
			b.WriteRune(r)
			count++
		}
	}
	if name := strings.Trim(b.String(), " ."); name != "" {
		return name
	}
	return "Document"
}

// webError answers with a short page that says what is wrong, in the
// look of pdfjet.com, and links back to the form.
func webError(status int, message string) webAnswer {
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>Markdown to PDF</title>
<link rel="stylesheet" href="/default.css?v=15" type="text/css">
</head>
<body>
<div id="content">
<h1>Markdown to PDF</h1>
<p>%s</p>
<p><a href="%s">Back to Markdown to PDF</a></p>
</div>
</body>
</html>
`, html.EscapeString(message), formPage)
	return webAnswer{status: status, body: []byte(page),
		headers: [][2]string{{"Content-Type", "text/html; charset=utf-8"}}}
}

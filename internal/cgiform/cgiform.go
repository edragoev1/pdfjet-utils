// cgiform.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// Package cgiform is what md2pdf and csv2pdf need to answer a form of
// pdfjet.com as CGI: the form read from the variables of CGI and the
// request's body, and the answer written, a file or a short page that says
// what is wrong. CGI is spoken here by hand: Go's net/http would make each
// program about four times larger for everyone who downloads it, for what a
// form needs, which net/url reads.
package cgiform

import (
	"bufio"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// Answer is what a program answers: a status, headers and a body.
type Answer struct {
	Status  int
	Headers [][2]string
	Body    []byte
}

// Running tells whether the program is run by a web server as CGI.
func Running() bool {
	return os.Getenv("GATEWAY_INTERFACE") != ""
}

// Serve answers the request of CGI with what handle answers, its variables
// read by env and its body from in.
func Serve(handle func(env func(string) string, in io.Reader) Answer) {
	out := bufio.NewWriter(os.Stdout)
	Write(out, handle(os.Getenv, os.Stdin))
	if err := out.Flush(); err != nil {
		os.Exit(1)
	}
}

// Write writes the answer as CGI has it: Status, the headers, an empty line,
// the body. No answer is kept by a cache.
func Write(w io.Writer, a Answer) {
	fmt.Fprintf(w, "Status: %d %s\r\n", a.Status, statusText[a.Status])
	for _, h := range a.Headers {
		fmt.Fprintf(w, "%s: %s\r\n", h[0], h[1])
	}
	fmt.Fprintf(w, "Cache-Control: no-store\r\nX-Content-Type-Options: nosniff\r\n")
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(a.Body))
	_, _ = w.Write(a.Body)
}

var statusText = map[int]string{
	200: "OK", 303: "See Other", 400: "Bad Request", 413: "Content Too Large",
	422: "Unprocessable Content", 503: "Service Unavailable",
}

// Page is a page of pdfjet.com that a form is on: its address and its title,
// which the answers that say what is wrong link back to and show.
type Page struct {
	Path  string
	Title string
}

// Redirect sends a request that is not the form's back to its page.
func (p Page) Redirect() Answer {
	return Answer{Status: 303, Headers: [][2]string{{"Location", p.Path}}}
}

// Error answers with a short page that says what is wrong, in the look of
// pdfjet.com, and links back to the form.
func (p Page) Error(status int, message string) Answer {
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>%[1]s</title>
<link rel="stylesheet" href="/default.css?v=15" type="text/css">
</head>
<body>
<div id="content">
<h1>%[1]s</h1>
<p>%[2]s</p>
<p><a href="%[3]s">Back to %[1]s</a></p>
</div>
</body>
</html>
`, html.EscapeString(p.Title), html.EscapeString(message), p.Path)
	return Answer{Status: status, Body: []byte(page),
		Headers: [][2]string{{"Content-Type", "text/html; charset=utf-8"}}}
}

// ErrTooLarge is the error of a request larger than ReadForm takes.
type ErrTooLarge struct{}

func (ErrTooLarge) Error() string { return "the request is too large" }

// ReadForm reads the form a page posts, application/x-www-form-urlencoded,
// of no more than max bytes: ErrTooLarge for one larger, another error for
// one that is not a form.
func ReadForm(env func(string) string, in io.Reader, max int) (url.Values, error) {
	if mediaType, _, _ := mime.ParseMediaType(env("CONTENT_TYPE")); mediaType != "application/x-www-form-urlencoded" {
		return nil, fmt.Errorf("not a form: %q", mediaType)
	}
	length, err := strconv.Atoi(env("CONTENT_LENGTH"))
	if err != nil || length < 0 {
		return nil, fmt.Errorf("no length")
	}
	if length > max {
		return nil, ErrTooLarge{}
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(in, body); err != nil {
		return nil, err
	}
	return url.ParseQuery(string(body))
}

// PDF answers with the PDF, to be saved as the file of the name.
func PDF(pdf []byte, name string) Answer {
	return Answer{Status: 200, Body: pdf, Headers: [][2]string{
		{"Content-Type", "application/pdf"},
		{"Content-Disposition", mime.FormatMediaType("attachment",
			map[string]string{"filename": FileName(name) + ".pdf"})},
	}}
}

// FileName returns the title as a file's name: its letters and digits, of
// any language, its spaces and a few marks, the rest left out, and no more
// than 80 characters; Document when nothing is left.
func FileName(title string) string {
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

// File is a file a form sends: its name, as the browser gives it, and its
// bytes.
type File struct {
	Name string
	Data []byte
}

// ReadFiles reads the form a page posts with its files,
// multipart/form-data, of no more than max bytes: its fields, and the files
// of the field, in the order they were sent, at most maxFiles. It gives
// ErrTooLarge for a request larger than max, ErrTooMany for more files, and
// another error for one that is not such a form. The files are read into
// memory, as they are as large as the request at most.
func ReadFiles(env func(string) string, in io.Reader, max int, field string, maxFiles int) (url.Values, []File, error) {
	mediaType, params, _ := mime.ParseMediaType(env("CONTENT_TYPE"))
	if mediaType != "multipart/form-data" || params["boundary"] == "" {
		return nil, nil, fmt.Errorf("not a form of files: %q", mediaType)
	}
	length, err := strconv.Atoi(env("CONTENT_LENGTH"))
	if err != nil || length < 0 {
		return nil, nil, fmt.Errorf("no length")
	}
	if length > max {
		return nil, nil, ErrTooLarge{}
	}
	reader := multipart.NewReader(io.LimitReader(in, int64(length)), params["boundary"])
	values := url.Values{}
	var files []File
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return nil, nil, err
		}
		if part.FormName() == field && part.FileName() != "" {
			if len(files) == maxFiles {
				return nil, nil, ErrTooMany{}
			}
			files = append(files, File{Name: part.FileName(), Data: data})
		} else if part.FileName() == "" {
			values.Add(part.FormName(), string(data))
		}
	}
	return values, files, nil
}

// ErrTooMany is the error of a form of more files than ReadFiles takes.
type ErrTooMany struct{}

func (ErrTooMany) Error() string { return "too many files" }

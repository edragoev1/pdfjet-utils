// main.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// qr2pdf makes a QR code ready to print: one on a page, or a sheet of the
// same code, for stickers, cards and table tents, each drawn as shapes, so
// that it is sharp at any size, with its words under it if asked. The code
// is plain: what it holds is in it, a link, a text, a Wi-Fi network or a
// contact card, and it works for as long as that does, with no server in
// between to stop it or count its scans. The PDF is PDF/UA, each code read
// out by its words, or what it holds.
//
//	qr2pdf https://pdfjet.com                writes qr-code.pdf, one code
//	qr2pdf -sheet -size 1.5 https://pdfjet.com   a sheet of codes 1.5 inches wide
//	qr2pdf -caption "Scan for the menu" -level H https://example.com/menu
//
// Run by a web server, as qr2pdf.cgi on pdfjet.com, it answers the form of
// the page QR Code Generator (cgi.go), which also makes the codes of Wi-Fi
// networks and contact cards (payload.go).
package main

import (
	"bufio"
	"bytes"
	"embed"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
	"github.com/edragoev1/pdfjet-utils/internal/footer"
	"github.com/edragoev1/pdfjet-utils/internal/version"
	pdfjet "github.com/edragoev1/pdfjet/v9/src"
	"github.com/edragoev1/pdfjet/v9/src/a4"
	"github.com/edragoev1/pdfjet/v9/src/compliance"
	"github.com/edragoev1/pdfjet/v9/src/errorcorrectionlevel"
	"github.com/edragoev1/pdfjet/v9/src/letter"
	"github.com/edragoev1/pdfjet/v9/src/pagesize"
	"github.com/edragoev1/pdfjet/v9/src/qrcode"
)

//go:embed fonts/*.ttf
var fonts embed.FS

// options are the choices of the command line and of the form.
type options struct {
	data    string  // What the code holds
	caption string  // The words under each code, or ""
	alt     string  // What a screen reader says of a code: the caption, or what it holds
	sheet   bool    // A sheet of codes, not one
	size    float32 // The width of a code, in inches
	level   errorcorrectionlevel.ErrorCorrectionLevel
	page    string // letter or a4
}

const (
	margin = 36
	// The white around a code, in its modules, that a scanner needs to find
	// it: four, as the standard asks
	quietModules = 4
)

// levels are the error correction levels by their letters.
var levels = map[string]errorcorrectionlevel.ErrorCorrectionLevel{
	"L": errorcorrectionlevel.L, "M": errorcorrectionlevel.M,
	"Q": errorcorrectionlevel.Q, "H": errorcorrectionlevel.H,
}

func main() {
	if cgiform.Running() {
		cgiform.Serve(handleWeb)
		return
	}
	output := flag.String("o", "qr-code.pdf", "the PDF to write")
	sheet := flag.Bool("sheet", false, "a sheet of the same code, not one")
	size := flag.Float64("size", 0, "the width of a code in inches (default: 4 for one, 1.5 for a sheet)")
	caption := flag.String("caption", "", "the words under each code")
	level := flag.String("level", "M", "the error correction: L, M, Q or H, the most, for a code printed small or with a logo over it")
	page := flag.String("page", "letter", "the page size: letter or a4")
	showVersion := flag.Bool("version", false, "print the version of qr2pdf and of PDFjet, and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: qr2pdf [options] text\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Text("qr2pdf"))
		return
	}
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	l, ok := levels[strings.ToUpper(*level)]
	if !ok {
		fmt.Fprintln(os.Stderr, "qr2pdf: the level is L, M, Q or H")
		os.Exit(2)
	}
	opts := options{data: flag.Arg(0), caption: *caption, sheet: *sheet, size: float32(*size), level: l, page: strings.ToLower(*page)}
	pdf, err := render(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qr2pdf:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, pdf, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "qr2pdf:", err)
		os.Exit(1)
	}
	fmt.Println(*output)
}

// The least and the most width of a code, in inches: a code smaller than
// about 0.6 inches is hard for a phone to read, and 7 fills a Letter page
const (
	minSize = 0.6
	maxSize = 7.0
)

// newCode returns the code of the data, or an error that says it is too
// long for the level, where PDFjet would panic.
func newCode(data string, level errorcorrectionlevel.ErrorCorrectionLevel) (code *qrcode.QRCode, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("the text is too long for a QR code: shorten it, or choose a lower error correction")
		}
	}()
	return qrcode.NewQRCode(data, level), nil
}

// render returns the PDF of the code, one or a sheet of them.
func render(opts options) ([]byte, error) {
	if opts.data == "" {
		return nil, fmt.Errorf("there is nothing for the code to hold")
	}
	var size pagesize.PageSize
	switch opts.page {
	case "letter", "":
		size = letter.Portrait()
	case "a4":
		size = a4.Portrait()
	default:
		return nil, fmt.Errorf("the page size %q is not letter or a4", opts.page)
	}
	width := opts.size
	if width == 0 {
		width = 4
		if opts.sheet {
			width = 1.5
		}
	}
	if width < minSize || width > maxSize {
		return nil, fmt.Errorf("the size of a code is from %.1f to %.0f inches", minSize, maxSize)
	}
	if _, err := newCode(opts.data, opts.level); err != nil {
		return nil, err
	}
	alt := opts.alt
	if alt == "" {
		alt = opts.caption
	}
	if alt == "" {
		alt = opts.data
	}

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	pdf := pdfjet.NewPDF(writer)
	pdf.SetCompliance(compliance.PDF_UA_1)
	pdf.SetTitle("QR code")
	pdf.SetLanguage("en-US")
	data, err := fonts.ReadFile("fonts/IBMPlexSans-Regular.ttf")
	if err != nil {
		return nil, err
	}
	// The words under one code larger than under each of a sheet of them
	captionSize := float32(9)
	if !opts.sheet {
		captionSize = 14
	}
	font := pdfjet.NewFont(pdf, bytes.NewReader(data)).SetSize(captionSize).SetKernPairs(true)

	page := pdfjet.NewPage(pdf, size)
	codeWidth := width * 72
	captionHeight := float32(0)
	if opts.caption != "" {
		captionHeight = captionSize * 1.8
	}
	// Each code with its white around it and its words under it
	draw := func(x, y float32) error {
		code, err := newCode(opts.data, opts.level)
		if err != nil {
			return err
		}
		modules := float32(len(code.GetModules()))
		code.SetModuleLength(codeWidth / modules)
		code.SetAltDescription(alt)
		code.SetLocation(x, y)
		code.DrawOn(page)
		if opts.caption != "" {
			line := pdfjet.NewTextLine(font, fitted(font, captionSize, opts.caption, codeWidth+codeWidth/modules*2*quietModules))
			lineWidth := font.StringWidth(captionSize, line.GetText())
			line.SetLocation(x+(codeWidth-lineWidth)/2, y+codeWidth+captionSize*1.4)
			line.DrawOn(page)
		}
		return nil
	}
	if !opts.sheet {
		x := (page.GetWidth() - codeWidth) / 2
		y := (page.GetHeight() - codeWidth - captionHeight) / 3
		if err := draw(x, y); err != nil {
			return nil, err
		}
	} else {
		// As many as fit, a quiet zone's width apart at least, the rows and
		// columns spread evenly between the margins
		quiet := codeWidth / 25 * quietModules
		cellW := codeWidth + 2*quiet
		cellH := codeWidth + captionHeight + 2*quiet
		roomW := page.GetWidth() - 2*margin
		roomH := page.GetHeight() - 2*margin
		cols := max(1, int(roomW/cellW))
		rows := max(1, int(roomH/cellH))
		gapX := (roomW - float32(cols)*codeWidth) / float32(cols)
		gapY := (roomH - float32(rows)*(codeWidth+captionHeight)) / float32(rows)
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				x := margin + gapX/2 + float32(c)*(codeWidth+gapX)
				y := margin + gapY/2 + float32(r)*(codeWidth+captionHeight+gapY)
				if err := draw(x, y); err != nil {
					return nil, err
				}
			}
		}
	}
	footer.Draw(page, font, "qr-code", 1, 1)
	if err := pdf.Complete(); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// fitted returns the text, cut short with an ellipsis if it is wider than
// the width at the caption's size.
func fitted(font *pdfjet.Font, captionSize float32, text string, width float32) string {
	if font.StringWidth(captionSize, text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && font.StringWidth(captionSize, string(runes)+"…") > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

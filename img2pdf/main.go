// main.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// img2pdf puts images in one PDF, a page each: photos, scans and drawings,
// JPEG, PNG, BMP and SVG, each fitted to its page and centred, on a page
// turned sideways when the image is wider than tall. A photo is drawn the
// way up its camera says (its Exif orientation), and an SVG as shapes, sharp
// at any size. The PDF is PDF/A-3a and PDF/UA-1 in one, made to be kept and
// tagged for screen readers, each image read out by its file's name.
//
//	img2pdf scan1.jpg scan2.jpg      writes scan1.pdf, both pages in it
//	img2pdf -o receipts.pdf *.jpg    writes receipts.pdf
//	img2pdf -page a4 photo.png       on A4 pages
//	img2pdf -margin 0 photo.jpg      the image to the page's edges
//
// Run by a web server, as img2pdf.cgi on pdfjet.com, it answers the form of
// the page Images to PDF (cgi.go).
package main

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/edragoev1/pdfjet-utils/internal/cgiform"
	"github.com/edragoev1/pdfjet-utils/internal/version"
	pdfjet "github.com/edragoev1/pdfjet/v9/src"
	"github.com/edragoev1/pdfjet/v9/src/a4"
	"github.com/edragoev1/pdfjet/v9/src/compliance"
	"github.com/edragoev1/pdfjet/v9/src/legal"
	"github.com/edragoev1/pdfjet/v9/src/letter"
	"github.com/edragoev1/pdfjet/v9/src/pagesize"
)

//go:embed fonts/*.ttf
var fonts embed.FS

// options are the choices of the command line and of the form.
type options struct {
	page     string  // letter, a4 or legal
	margin   float32 // Around the image, in points
	language string
}

// The page sizes, by name, upright and turned sideways.
var pageSizes = map[string][2]func() pagesize.PageSize{
	"letter": {letter.Portrait, letter.Landscape},
	"a4":     {a4.Portrait, a4.Landscape},
	"legal":  {legal.Portrait, legal.Landscape},
}

const pageNames = "letter, a4 or legal"

// An image as it is read: its name and bytes, and its kind.
type picture struct {
	name string
	data []byte
	kind string // jpeg, png, bmp or svg
}

func main() {
	if cgiform.Running() {
		cgiform.Serve(handleWeb)
		return
	}
	output := flag.String("o", "", "the PDF to write (default: the first image's name with .pdf)")
	page := flag.String("page", "letter", "the page size: "+pageNames)
	margin := flag.Float64("margin", 36, "the margin around each image, in points (72 to the inch)")
	language := flag.String("lang", "en-US", "the language of the document, such as en-US, fr-CA or de-DE")
	showVersion := flag.Bool("version", false, "print the version of img2pdf and of PDFjet, and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: img2pdf [options] image...\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Text("img2pdf"))
		return
	}
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if _, ok := pageSizes[strings.ToLower(*page)]; !ok {
		fmt.Fprintf(os.Stderr, "img2pdf: the page size %q is not one of %s\n", *page, pageNames)
		os.Exit(2)
	}
	if *margin < 0 || *margin > 144 {
		fmt.Fprintln(os.Stderr, "img2pdf: the margin is from 0 to 144 points")
		os.Exit(2)
	}
	var pictures []picture
	for _, path := range flag.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "img2pdf:", err)
			os.Exit(1)
		}
		p, err := pictureOf(filepath.Base(path), data)
		if err != nil {
			fmt.Fprintln(os.Stderr, "img2pdf:", err)
			os.Exit(1)
		}
		pictures = append(pictures, p)
	}
	pdfPath := *output
	if pdfPath == "" {
		first := flag.Arg(0)
		pdfPath = strings.TrimSuffix(first, filepath.Ext(first)) + ".pdf"
	}
	for _, path := range flag.Args() {
		if filepath.Clean(path) == filepath.Clean(pdfPath) {
			fmt.Fprintln(os.Stderr, "img2pdf: the PDF would replace an image")
			os.Exit(1)
		}
	}
	title := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	pdf, err := render(pictures, title, options{page: strings.ToLower(*page), margin: float32(*margin), language: *language})
	if err != nil {
		fmt.Fprintln(os.Stderr, "img2pdf:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(pdfPath, pdf, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "img2pdf:", err)
		os.Exit(1)
	}
	fmt.Printf("%d images => %s\n", len(pictures), pdfPath)
}

// pictureOf returns the image of the file's bytes, of the kind its first
// bytes say, whatever its name; or an error that says plainly what it is
// when img2pdf does not take it.
func pictureOf(name string, data []byte) (picture, error) {
	p := picture{name: name, data: data}
	head := data[:min(len(data), 512)]
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		p.kind = "jpeg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		p.kind = "png"
	case bytes.HasPrefix(data, []byte("BM")):
		p.kind = "bmp"
	case bytes.Contains(bytes.ToLower(head), []byte("<svg")) ||
		(bytes.HasPrefix(bytes.TrimSpace(head), []byte("<")) && bytes.Contains(bytes.ToLower(data), []byte("<svg"))):
		p.kind = "svg"
	case len(data) > 12 && string(data[4:8]) == "ftyp" &&
		strings.Contains("heic heix hevc heim heis mif1 msf1 avif", string(data[8:12])):
		return p, fmt.Errorf("%s is a HEIC or AVIF photo, as an iPhone takes them, which is not supported: save it as JPEG (on an iPhone: Settings, Camera, Formats, Most Compatible)", name)
	case len(data) > 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return p, fmt.Errorf("%s is a WebP image, which is not supported: save it as JPEG or PNG", name)
	case bytes.HasPrefix(data, []byte("GIF8")):
		return p, fmt.Errorf("%s is a GIF image, which is not supported: save it as PNG", name)
	case bytes.HasPrefix(data, []byte("%PDF")):
		return p, fmt.Errorf("%s is a PDF already, not an image", name)
	default:
		return p, fmt.Errorf("%s is not a JPEG, PNG, BMP or SVG image", name)
	}
	return p, nil
}

// render returns the PDF of the pictures, a page each, with the title.
func render(pictures []picture, title string, opts options) (pdfBytes []byte, err error) {
	if len(pictures) == 0 {
		return nil, errors.New("there are no images")
	}
	sizes, ok := pageSizes[opts.page]
	if !ok {
		return nil, fmt.Errorf("the page size %q is not one of %s", opts.page, pageNames)
	}
	// PDFjet panics on an image it cannot read; that is an error here
	current := ""
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%s could not be read: %v", current, p)
		}
	}()

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	pdf := pdfjet.NewPDF(writer)
	// PDF/A keeps its colors in sRGB, which a CMYK photo, as a printer makes
	// them, is not: a PDF of one is PDF/UA alone, accessible all the same
	level := compliance.PDF_A_3A_UA_1
	for _, p := range pictures {
		if p.kind == "jpeg" && jpegIsCMYK(p.data) {
			level = compliance.PDF_UA_1
		}
	}
	pdf.SetCompliance(level)
	pdf.SetTitle(title)
	pdf.SetLanguage(opts.language)
	data, err := fonts.ReadFile("fonts/IBMPlexSans-Regular.ttf")
	if err != nil {
		return nil, err
	}
	// For the text of an SVG, if it has any
	font := pdfjet.NewFont(pdf, bytes.NewReader(data)).SetSize(12)

	for _, p := range pictures {
		current = p.name
		alt := strings.TrimSuffix(p.name, filepath.Ext(p.name))
		var w, h float32
		var draw func(page *pdfjet.Page, x, y, scale float32)
		if p.kind == "svg" {
			svg, err := pdfjet.NewSVGImage(bytes.NewReader(p.data))
			if err != nil {
				return nil, fmt.Errorf("%s could not be read: %v", p.name, err)
			}
			svg.SetFont(font)
			svg.SetAltDescription(alt)
			w, h = svg.GetWidth(), svg.GetHeight()
			draw = func(page *pdfjet.Page, x, y, scale float32) {
				svg.ScaleBy(scale)
				svg.SetLocation(x, y)
				svg.DrawOn(page)
			}
		} else {
			image := pdfjet.NewImageFromBytes(pdf, p.data)
			image.SetAltDescription(alt)
			w, h = image.GetWidth(), image.GetHeight()
			draw = func(page *pdfjet.Page, x, y, scale float32) {
				image.ScaleBy(scale)
				image.SetLocation(x, y)
				image.DrawOn(page)
			}
		}
		if !(w > 0) || !(h > 0) {
			return nil, fmt.Errorf("%s has no size", p.name)
		}
		size := sizes[0]()
		if w > h {
			size = sizes[1]()
		}
		page := pdfjet.NewPage(pdf, size)
		roomW := page.GetWidth() - 2*opts.margin
		roomH := page.GetHeight() - 2*opts.margin
		scale := min(roomW/w, roomH/h)
		x := (page.GetWidth() - w*scale) / 2
		y := (page.GetHeight() - h*scale) / 2
		draw(page, x, y, scale)
	}
	current = ""
	if err := pdf.Complete(); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// jpegIsCMYK tells whether the JPEG's frame has four components, CMYK or
// YCCK, from its start of frame: the markers are read one after another to
// the first SOF.
func jpegIsCMYK(data []byte) bool {
	for i := 2; i+9 < len(data); {
		if data[i] != 0xFF {
			return false
		}
		marker := data[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		length := int(data[i+2])<<8 | int(data[i+3])
		// SOF0 to SOF15, but DHT (C4), JPG (C8) and DAC (CC)
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			return data[i+9] == 4
		}
		if marker == 0xDA || length < 2 {
			return false
		}
		i += 2 + length
	}
	return false
}

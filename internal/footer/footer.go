// footer.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// Package footer draws the foot of every page the programs of pdfjet-utils
// make: on the left a link to the program's page on pdfjet.com, which opens
// it when clicked, so that a PDF passed on tells where to make another, and on
// the right the page's number, "1 of 3" (the owner, 10 November 2026).
package footer

import (
	"strconv"

	pdfjet "github.com/edragoev1/pdfjet/v9/src"
)

const (
	// The margin the foot is set in from the sides, and its baseline's
	// height above the bottom of the page, in points
	Margin = 36
	Height = 18
	// The size of the link and of the page's number, the link a point
	// larger (the owner, 10 November 2026), and their color, a quiet gray
	linkSize = 9
	size     = 8
	color    = 0x6B7280
)

// Draw draws the foot of the page: the link to the tool's page,
// pdfjet.com/tool, and the page's number of the pages.
func Draw(page *pdfjet.Page, font *pdfjet.Font, tool string, number, pages int) {
	y := page.GetHeight() - Height
	link := pdfjet.NewTextLine(font, "Made with pdfjet.com/"+tool)
	link.SetFontSize(linkSize)
	link.SetTextColor(color)
	link.SetURIAction("https://pdfjet.com/" + tool)
	link.SetLocation(Margin, y)
	link.DrawOn(page)

	text := strconv.Itoa(number) + " of " + strconv.Itoa(pages)
	count := pdfjet.NewTextLine(font, text)
	count.SetFontSize(size)
	count.SetTextColor(color)
	count.SetLocation(page.GetWidth()-Margin-font.StringWidth(size, text), y)
	count.DrawOn(page)
}

// svgpaste.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.
//
// An SVG pasted into the Markdown, its code on lines of their own, is drawn
// as an image: a logo or a diagram copied from its file, which a text sent to
// the web has no other way to carry (the owner, 10 November 2026). Each one
// is written to a folder of its own, made for the conversion and removed
// after it, and the Markdown names it there as an image, ![title](svg-1.svg),
// which PDFjet draws as it draws an SVG file, sharp at any size. Nothing but
// what was pasted is in that folder, so nothing else is read.
//
// A pasted SVG starts on a line that starts with <svg, or with the <?xml or
// <!DOCTYPE svg that a file of one starts with, and ends on the line with
// </svg>; one in a block of code is code, and left as it is. Its text is its
// <title>, if it has one, as a screen reader says it.

package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The SVGs one text may carry
const maxPastedSVGs = 50

var svgTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// pastedSVGs returns the text with each SVG pasted in it named as an image,
// and the SVGs' files by name; the text as it is when it has none.
func pastedSVGs(text string) (string, map[string][]byte) {
	lines := strings.Split(text, "\n")
	files := map[string][]byte{}
	var out []string
	fence := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		// A block of code, ``` or ~~~, is left as it is
		if fence == "" && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			fence = trimmed[:3]
		} else if fence != "" && strings.HasPrefix(trimmed, fence) {
			fence = ""
		}
		if fence != "" || !startsSVG(trimmed) || len(files) == maxPastedSVGs {
			out = append(out, line)
			continue
		}
		end := -1
		for j := i; j < len(lines); j++ {
			if strings.Contains(strings.ToLower(lines[j]), "</svg>") {
				end = j
				break
			}
		}
		if end < 0 {
			out = append(out, line)
			continue
		}
		code := strings.Join(lines[i:end+1], "\n")
		// What follows </svg> on its line stays text
		closing := strings.Index(strings.ToLower(code), "</svg>") + len("</svg>")
		after := strings.TrimSpace(code[closing:])
		code = code[:closing]
		name := fmt.Sprintf("svg-%d.svg", len(files)+1)
		files[name] = []byte(code)
		// Alone in its paragraph, as PDFjet draws an image
		out = append(out, "", fmt.Sprintf("![%s](%s)", altOf(code, len(files)), name), "")
		if after != "" {
			out = append(out, after)
		}
		i = end
	}
	if len(files) == 0 {
		return text, nil
	}
	return strings.Join(out, "\n"), files
}

func startsSVG(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "<svg") || strings.HasPrefix(lower, "<?xml") ||
		strings.HasPrefix(lower, "<!doctype svg")
}

// altOf returns the SVG's title, plain and on one line, as the image's text,
// or Image and its number
func altOf(code string, n int) string {
	if m := svgTitle.FindStringSubmatch(code); m != nil {
		title := strings.Join(strings.Fields(html.UnescapeString(m[1])), " ")
		// Not what would end the image's text in the Markdown
		title = strings.NewReplacer("[", "(", "]", ")", "\\", "/").Replace(title)
		if title != "" {
			return title
		}
	}
	return fmt.Sprintf("Image %d", n)
}

// withPastedSVGs runs draw with the text's pasted SVGs written to a folder
// of their own and named in the text from it, inside the folder of the
// images, so that both are read, or in the system's temporary folder when
// there is none, as on the web; the folder is removed after. draw is given
// the text and the folder to read images from, "" for none.
func withPastedSVGs(text, imageDirectory string, draw func(text, imageDirectory string) error) error {
	text, files := pastedSVGs(text)
	if len(files) == 0 {
		return draw(text, imageDirectory)
	}
	parent, prefix := imageDirectory, ".md2pdf-svg-"
	if parent == "" {
		parent, prefix = "", "md2pdf-svg-"
	}
	folder, err := os.MkdirTemp(parent, prefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(folder)
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(folder, name), data, 0o600); err != nil {
			return err
		}
	}
	if imageDirectory == "" {
		return draw(text, folder)
	}
	// Named from the folder of the images, which the pasted ones are in
	sub := filepath.Base(folder)
	for name := range files {
		text = strings.ReplaceAll(text, "]("+name+")", "]("+sub+"/"+name+")")
	}
	return draw(text, imageDirectory)
}

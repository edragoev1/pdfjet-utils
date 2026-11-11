// version.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.

// Package version says the version of a program of pdfjet-utils and of the
// PDFjet it is built on, as Go records them in the program: the tag of a
// release, such as v1.0.5, for go install and for build.sh at a tag, and a
// pseudo-version of the commit otherwise. The release checks that each
// program says the tag's version (.github/workflows/release.yml).
package version

import "runtime/debug"

// Text returns the program's name and version, the PDFjet it is built on,
// and the Go that built it: "md2pdf v1.1.0, built on PDFjet v9.1.7, go1.27.2".
func Text(program string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return program + " (version unknown)"
	}
	version := info.Main.Version
	if version == "" {
		version = "(devel)"
	}
	text := program + " " + version
	for _, module := range info.Deps {
		if module.Path == "github.com/edragoev1/pdfjet/v9" {
			pdfjetVersion := module.Version
			if module.Replace != nil {
				pdfjetVersion = module.Replace.Version
				if pdfjetVersion == "" {
					pdfjetVersion = "(local)"
				}
			}
			text += ", built on PDFjet " + pdfjetVersion
		}
	}
	return text + ", " + info.GoVersion
}

# pdfjet-utils

Small command-line tools built on [PDFjet](https://github.com/edragoev1/pdfjet).
Private.

## md2pdf

Converts Markdown files to PDF with PDFjet's Markdown: headings, paragraphs
with bold, italic and code, links, nested lists, quotes, code blocks, tables
and images. The text is IBM Plex Sans and the code IBM Plex Mono, built into
the program, so it runs from any folder. The PDFs are PDF/UA, tagged for
screen readers (veraPDF passes them), with a page number at the foot of every
page and the first `#` heading as the title.

```
go install ./md2pdf               # once: puts md2pdf in ~/go/bin

md2pdf notes.md                   # writes notes.pdf next to notes.md
md2pdf -o out.pdf notes.md        # writes out.pdf
md2pdf -page a4 *.md              # a PDF for each file, on A4 pages
md2pdf -size 10 -lang fr-CA notes.md
```

Images are read from the folder of the Markdown file, and from nowhere else.
It is PDFjet's Markdown, not all of Markdown: HTML is drawn as the text it is,
and a line break inside a paragraph is a space. A failed conversion writes no
file.

The fonts are under the SIL Open Font License 1.1: `md2pdf/fonts/OFL.txt`.

## Executables for other computers

`./build.sh` builds md2pdf for Linux, Windows and macOS, Intel and ARM, into
`dist/`: one file of about 4.5 MB each, with the fonts inside, which needs
nothing installed. Copy the one for the computer and run it from a terminal
(`md2pdf-windows-amd64.exe notes.md` in Windows' terminal).

The files are not signed: Windows' SmartScreen asks before the first run
("More info", then "Run anyway"), and macOS refuses it until its quarantine
is removed, once: `xattr -d com.apple.quarantine md2pdf-darwin-arm64`.

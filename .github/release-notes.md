md2pdf converts Markdown files to accessible PDFs (PDF/UA) with [PDFjet](https://github.com/edragoev1/pdfjet): headings, bold, italic, strikethrough, code, links, nested lists, task lists, quotes, code blocks, tables and images, SVG pasted into the text among them, in IBM Plex Sans or Serif and Cascadia Mono, built in. csv2pdf converts CSV files to accessible PDF tables, with PDFjet's BigTable: the header on every page, numbers right-aligned, every other row shaded, page numbers. img2pdf puts JPEG, PNG, BMP and SVG images in one PDF, a page each, phone photos the right way up, as PDF/A-3a and PDF/UA. cal2pdf makes a printable calendar of a year, a month a page or the year on one, in 36 languages with CLDR's names. qr2pdf makes QR codes ready to print, one or a sheet, of a link, a text, a Wi-Fi network or a contact card: plain codes that never expire and are not tracked. One file per program and computer, nothing to install. On the web: [Markdown to PDF](https://pdfjet.com/markdown-to-pdf) [CSV to PDF](https://pdfjet.com/csv-to-pdf) [Images to PDF](https://pdfjet.com/images-to-pdf) [Printable Calendar](https://pdfjet.com/printable-calendar) and [QR Code Generator](https://pdfjet.com/qr-code).

| Computer | File |
|---|---|
| Windows, Intel or AMD | `md2pdf-windows-amd64.exe` |
| Windows, ARM | `md2pdf-windows-arm64.exe` |
| Mac, Apple chip (M1 and later) | `md2pdf-darwin-arm64` |
| Mac, Intel | `md2pdf-darwin-amd64` |
| Linux, Intel or AMD | `md2pdf-linux-amd64` |
| Linux, ARM | `md2pdf-linux-arm64` |

csv2pdf, img2pdf, cal2pdf and qr2pdf the same way: `csv2pdf-windows-amd64.exe`, `img2pdf-darwin-arm64`, `img2pdf-linux-amd64` and so on.

```
md2pdf notes.md                  # writes notes.pdf next to notes.md
md2pdf -o out.pdf notes.md
md2pdf -page a4 *.md
md2pdf -landscape wide.md        # also -page legal, tabloid, a3 or a5
md2pdf -size 10 -lang fr-CA notes.md
md2pdf -serif notes.md           # the text in IBM Plex Serif
md2pdf --version
csv2pdf data.csv                 # writes data.pdf, landscape
csv2pdf -page a4 -columns 1,2,5 wide.csv
csv2pdf -portrait -d ";" data.csv
img2pdf -o receipts.pdf *.jpg     # a page each, in this order
img2pdf -page a4 -margin 0 photo.png
cal2pdf 2027                     # calendar-2027.pdf, a month a page
cal2pdf -year-page -lang fr-FR -page a4 2027
qr2pdf -caption "Scan for the menu" https://example.com/menu
qr2pdf -sheet -size 1.5 -level H https://pdfjet.com
```

The files are not signed: Windows asks before the first run ("More info", then "Run anyway"); on a Mac, run `xattr -d com.apple.quarantine md2pdf-darwin-arm64` once (and the same for the others), and `chmod +x` it on a Mac or Linux. `SHA256SUMS` has the checksums. Built by GitHub Actions from the tagged commit, with `build.sh`.

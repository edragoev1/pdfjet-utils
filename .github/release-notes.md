md2pdf converts Markdown files to accessible PDFs (PDF/UA) with [PDFjet](https://github.com/edragoev1/pdfjet): headings, bold, italic, code, links, nested lists, task lists, quotes, code blocks, tables and images, in IBM Plex Sans and Mono, built in. One file per computer, nothing to install.

| Computer | File |
|---|---|
| Windows, Intel or AMD | `md2pdf-windows-amd64.exe` |
| Windows, ARM | `md2pdf-windows-arm64.exe` |
| Mac, Apple chip (M1 and later) | `md2pdf-darwin-arm64` |
| Mac, Intel | `md2pdf-darwin-amd64` |
| Linux, Intel or AMD | `md2pdf-linux-amd64` |
| Linux, ARM | `md2pdf-linux-arm64` |

```
md2pdf notes.md                  # writes notes.pdf next to notes.md
md2pdf -o out.pdf notes.md
md2pdf -page a4 *.md
md2pdf -size 10 -lang fr-CA notes.md
md2pdf --version
```

The files are not signed: Windows asks before the first run ("More info", then "Run anyway"); on a Mac, run `xattr -d com.apple.quarantine md2pdf-darwin-arm64` once, and `chmod +x` it on a Mac or Linux. `SHA256SUMS` has the checksums. Built by GitHub Actions from the tagged commit, with `build.sh`.

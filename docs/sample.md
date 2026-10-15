# Release checklist

md2pdf turns this file into an **accessible PDF** in milliseconds, with
[PDFjet](https://github.com/edragoev1/pdfjet) doing the drawing.

## Before the release

- [x] Write the changelog
- [x] Run the tests
  - [x] Linux and macOS
  - [x] Windows
- [ ] Tag the version
- [ ] Tell the [users](https://github.com/edragoev1/pdfjet-utils).

## How it works

> One Go file around PDFjet's `Markdown`, with the fonts built in.

```go
markdown := pdfjet.NewMarkdown(regular, bold, italic, boldItalic, code)
markdown.DrawOnPages(pdf, text, &pages, letter.Portrait())
```

| Feature        | Tagged as         |
|----------------|-------------------|
| Headings       | H1 to H6          |
| Lists and tasks | L, LI, Lbl, LBody |
| Code           | Code              |
| Tables         | Table, TR, TH, TD |

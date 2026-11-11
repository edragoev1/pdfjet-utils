#!/bin/bash
# Copyright (c) 2026 PDFjet Software
# Licensed under the MIT License. See LICENSE file in the project root.
#
# Builds md2pdf for Linux, Windows and macOS into dist/: one file each, with
# the fonts inside, which needs nothing else installed. Go cross-compiles, so
# all of them build here, from the PDFjet that go.mod requires, not a local
# go.work, and with the version of the git tag, which md2pdf --version prints.
#
#   ./build.sh
set -euo pipefail
cd "$(dirname "$0")"
rm -rf dist && mkdir dist
for target in linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/arm64 darwin/amd64; do
    os=${target%/*} arch=${target#*/}
    name=md2pdf-$os-$arch
    [ "$os" = windows ] && name=$name.exe
    # pdfjet_nofonts: without the fonts PDFjet carries, which md2pdf never
    # uses, as it carries its own (md2pdf/fonts); with them, each file was
    # about 15 MB, without them about 6
    GOWORK=off CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -tags pdfjet_nofonts -trimpath -ldflags="-s -w" -o "dist/$name" ./md2pdf
done
ls -lh dist | awk 'NR>1 {print $5, $9}'

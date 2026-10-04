#!/usr/bin/env bash
#
# Builds the decide CLI for every platform we publish, into dist/.
#
# Usage: build-release.sh <version>
#
# The version is stamped into the binary so "decide version" reports the tag it
# came from. Binaries are static (CGO_ENABLED=0) and reproducible: -trimpath
# removes local paths, and -s -w drop the symbol table and DWARF data.

set -euo pipefail

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
	echo "usage: $(basename "$0") <version>" >&2
	exit 2
fi

# Run from the repository root regardless of the caller's working directory.
cd "$(dirname "$0")/.."

rm -rf dist
mkdir -p dist

# os/arch pairs that get a release artifact. Linux arm and windows 386 are here
# because Go still builds them cleanly and they cost nothing to ship.
TARGETS=(
	linux/amd64
	linux/arm64
	linux/386
	linux/arm
	darwin/amd64
	darwin/arm64
	windows/amd64
	windows/arm64
	windows/386
)

for target in "${TARGETS[@]}"; do
	os="${target%/*}"
	arch="${target#*/}"

	binary=decide
	if [ "$os" = "windows" ]; then
		binary=decide.exe
	fi

	mkdir -p "dist/$os-$arch"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
		go build -trimpath \
			-ldflags "-s -w -X main.version=$VERSION" \
			-o "dist/$os-$arch/$binary" \
			./cmd/decide

	# Normalize mtime so identical inputs produce identical archives.
	# gzip, not xz: -J is tar's xz flag and would mislabel a .gz archive.
	# GNU tar only: --sort/--mtime/--owner are not portable to BSD tar (macOS).
	if tar --sort=name --version >/dev/null 2>&1; then
		tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
			-czf "dist/decide_${VERSION#v}_${os}_${arch}.tar.gz" \
			-C "dist/$os-$arch" "$binary"
	else
		tar -czf "dist/decide_${VERSION#v}_${os}_${arch}.tar.gz" \
			-C "dist/$os-$arch" "$binary"
	fi
done

# zip is present on GitHub runners for Windows, but not everywhere else.
if command -v zip >/dev/null 2>&1; then
	for arch in amd64 arm64 386; do
		(cd "dist/windows-$arch" && zip -q -X "../decide_${VERSION#v}_windows_$arch.zip" decide.exe)
	done
else
	echo "zip is unavailable, leaving the Windows binaries unpacked" >&2
fi

rm -rf dist/*-*/

echo "Artifacts:"
ls -1 dist
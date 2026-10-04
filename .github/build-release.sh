#!/usr/bin/env bash
#
# Builds the decide CLI and the decide-playground server for every platform we
# publish, into dist/.
#
# Usage: build-release.sh <version>
#
# The version is stamped into the binary so "decide version" reports the tag it
# came from. Binaries are static (CGO_ENABLED=0) and reproducible: -trimpath
# removes local paths, and -s -w drop the symbol table and DWARF data.
#
# decide-playground embeds the frontend from web/dist, so it needs the
# frontend built first. Build it with `make web` (or set SKIP_WEB=1 to reuse
# whatever is already in web/dist); the script checks that the bundle is real
# and refuses to publish a binary that embeds the placeholder page.

set -euo pipefail

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
	echo "usage: $(basename "$0") <version>" >&2
	exit 2
fi

# Run from the repository root regardless of the caller's working directory.
cd "$(dirname "$0")/.."

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

# Build the frontend into web/dist. A published playground binary must embed the
# real single-page app, not the committed placeholder.
if [ "${SKIP_WEB:-0}" = "1" ]; then
	echo "SKIP_WEB=1: reusing the existing web/dist"
else
	echo "Building the playground frontend"
	(cd web && npm ci --no-audit --no-fund && npm run build)
fi

if ! grep -q '/assets/' web/dist/index.html 2>/dev/null; then
	echo "web/dist/index.html does not reference a built bundle." >&2
	echo "Run 'make web' before building a release." >&2
	exit 1
fi

rm -rf dist
mkdir -p dist

for target in "${TARGETS[@]}"; do
	os="${target%/*}"
	arch="${target#*/}"

	suffix=""
	if [ "$os" = "windows" ]; then
		suffix=".exe"
	fi

	mkdir -p "dist/$os-$arch"
	for command in decide decide-playground; do
		CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
			go build -trimpath \
				-ldflags "-s -w -X main.version=$VERSION" \
				-o "dist/$os-$arch/$command$suffix" \
				"./cmd/$command"
	done

	# Normalize mtime so identical inputs produce identical archives.
	# gzip, not xz: -J is tar's xz flag and would mislabel a .gz archive.
	# GNU tar only: --sort/--mtime/--owner are not portable to BSD tar (macOS).
	if tar --sort=name --version >/dev/null 2>&1; then
		tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
			-czf "dist/decide_${VERSION#v}_${os}_${arch}.tar.gz" \
			-C "dist/$os-$arch" decide$suffix decide-playground$suffix
		tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
			-czf "dist/decide-playground_${VERSION#v}_${os}_${arch}.tar.gz" \
			-C "dist/$os-$arch" decide-playground$suffix
	else
		tar -czf "dist/decide_${VERSION#v}_${os}_${arch}.tar.gz" \
			-C "dist/$os-$arch" decide$suffix decide-playground$suffix
		tar -czf "dist/decide-playground_${VERSION#v}_${os}_${arch}.tar.gz" \
			-C "dist/$os-$arch" decide-playground$suffix
	fi
done

# zip is present on GitHub runners for Windows, but not everywhere else.
if command -v zip >/dev/null 2>&1; then
	for arch in amd64 arm64 386; do
		(cd "dist/windows-$arch" && zip -q -X "../decide_${VERSION#v}_windows_$arch.zip" decide.exe decide-playground.exe)
		(cd "dist/windows-$arch" && zip -q -X "../decide-playground_${VERSION#v}_windows_$arch.zip" decide-playground.exe)
	done
else
	echo "zip is unavailable, leaving the Windows binaries unpacked" >&2
fi

rm -rf dist/*-*/

echo "Artifacts:"
ls -1 dist

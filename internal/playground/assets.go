package playground

import (
	"io/fs"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// assetHandler serves the built frontend from an [fs.FS] rooted at the bundle.
//
// It is written by hand rather than delegated to the static middleware so each
// caching rule is explicit: hashed assets are immutable, the shell is not
// cached at all, and only a real file falls through to the shell.
type assetHandler struct {
	assets fs.FS
}

func newAssetHandler(assets fs.FS) *assetHandler {
	return &assetHandler{assets: assets}
}

// handle serves one static path. It reports false when the path is not a file,
// which lets the caller fall back to the app shell.
func (h *assetHandler) handle(c fiber.Ctx) (bool, error) {
	name := strings.TrimPrefix(c.Path(), "/")
	if name == "" {
		return false, nil
	}

	// A crafted path such as /../../go.mod must never escape the bundle.
	if !fs.ValidPath(name) {
		return false, nil
	}

	data, err := fs.ReadFile(h.assets, name)
	if err != nil {
		return false, nil
	}

	c.Set(fiber.HeaderContentType, contentType(name))
	if strings.HasPrefix(name, strings.Trim(assetPrefix, "/")) {
		// Vite fingerprints these filenames, so a changed file is a changed
		// URL and the old one can be cached forever.
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	} else {
		c.Set(fiber.HeaderCacheControl, "no-store")
	}
	return true, c.Send(data)
}

// contentType maps an extension to a media type. The bundler only emits a
// handful, so a small table beats pulling in a dependency for it.
func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return fiber.MIMETextHTMLCharsetUTF8
	case strings.HasSuffix(name, ".json"), strings.HasSuffix(name, ".map"):
		return fiber.MIMEApplicationJSON
	case strings.HasSuffix(name, ".txt"):
		return fiber.MIMETextPlainCharsetUTF8
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".gif"):
		return "image/gif"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".woff"):
		return "font/woff"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".ttf"):
		return "font/ttf"
	case strings.HasSuffix(name, ".wasm"):
		return "application/wasm"
	default:
		return fiber.MIMEOctetStream
	}
}

// serveIndex sends the app shell. It is always no-store, so a rebuilt binary
// replaces the old interface immediately.
func serveIndex(c fiber.Ctx, assets fs.FS) error {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return &fiber.Error{Code: fiber.StatusNotFound, Message: "index.html is missing from the embedded assets"}
	}

	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Send(data)
}

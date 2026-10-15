package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

// staticModTime is a stable timestamp used for Last-Modified. Files in an
// embed.FS carry a zero mod time, which tells clients "do not cache"; a fixed
// stamp lets them revalidate cheaply with If-None-Match / If-Modified-Since.
var staticModTime = time.Now().UTC().Truncate(time.Second)

// staticAsset is a precomputed, ready-to-serve copy of one embedded file.
type staticAsset struct {
	data  []byte
	gz    []byte // gzip of data; nil when the type is not worth compressing
	etag  string
	ctype string
}

// staticIndex maps a cleaned /static-relative path (e.g. "niburu/ships/202.gif")
// to its asset. Built once at startup so serving is a map lookup plus a write.
var staticIndex = buildStaticIndex()

func buildStaticIndex() map[string]*staticAsset {
	idx := make(map[string]*staticAsset)
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return idx
	}
	_ = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(sub, p)
		if err != nil {
			return nil
		}
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		a := &staticAsset{data: data, ctype: ct, etag: assetETag(data)}
		if compressible(ct) {
			var buf bytes.Buffer
			zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			if err == nil {
				_, _ = zw.Write(data)
				if zw.Close() == nil {
					a.gz = buf.Bytes()
				}
			}
		}
		idx[path.Clean(p)] = a
		return nil
	})
	return idx
}

// assetETag is a strong validator derived from the file contents.
func assetETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// compressible reports whether gzip is worth applying: text and svg/json only,
// never already-compressed images (gif/jpg/png).
func compressible(ct string) bool {
	if strings.HasPrefix(ct, "text/") {
		return true
	}
	switch ct {
	case "application/javascript", "application/json", "application/xml",
		"image/svg+xml":
		return true
	}
	return false
}

// Static returns the file handler for the embedded skin assets. It is mounted
// outside the per-IP rate limiter (a page pulls hundreds of sprites) and sends
// ETag/Cache-Control/gzip so repeat loads are 304s.
func (h *Handler) Static() http.Handler {
	return http.StripPrefix("/static/", http.HandlerFunc(serveStaticAsset))
}

func serveStaticAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	a, ok := staticIndex[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	head := w.Header()
	head.Set("ETag", a.etag)
	head.Set("Cache-Control", "public, max-age=3600")
	head.Set("Last-Modified", staticModTime.Format(http.TimeFormat))

	if etagMatches(r.Header.Get("If-None-Match"), a.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	head.Set("Content-Type", a.ctype)
	body := a.data
	if a.gz != nil {
		head.Set("Vary", "Accept-Encoding")
		if acceptsGzip(r) {
			head.Set("Content-Encoding", "gzip")
			body = a.gz
		}
	}
	head.Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(body)
}

func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		if strings.TrimSpace(part) == etag {
			return true
		}
	}
	return false
}

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

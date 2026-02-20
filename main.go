package main

import (
	"compress/gzip"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed src/*
var assets embed.FS

func main() {
	staticFiles, err := fs.Sub(assets, "src")
	if err != nil {
		log.Fatal(err)
	}

	handler := loggingMiddleware(gzipMiddleware(http.FileServer(http.FS(staticFiles))))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

// compressibleTypes lists MIME type prefixes that benefit from compression.
// Binary formats like images, video, and woff2 are already compressed.
var compressibleTypes = []string{
	"text/",
	"application/json",
	"application/javascript",
	"application/xml",
	"application/xhtml+xml",
	"image/svg+xml",
}

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
		return w
	},
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		grw := &gzipResponseWriter{ResponseWriter: w}
		next.ServeHTTP(grw, r)

		// If we buffered the body (compressible type), compress and flush it now.
		if grw.body != nil {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Del("Content-Length")
			w.WriteHeader(grw.code)

			gz := gzipWriterPool.Get().(*gzip.Writer)
			gz.Reset(w)
			gz.Write(grw.body)
			gz.Close()
			gzipWriterPool.Put(gz)
		}
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	body        []byte
	code        int
	wroteHeader bool
	passthrough bool
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.code = code

	ct := w.Header().Get("Content-Type")
	if shouldCompress(ct) {
		// Buffer the response so we can compress it.
		return
	}

	// Not compressible — write through directly.
	w.passthrough = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.passthrough {
		return w.ResponseWriter.Write(b)
	}
	w.body = append(w.body, b...)
	return len(b), nil
}

func shouldCompress(contentType string) bool {
	ct := strings.ToLower(contentType)
	for _, prefix := range compressibleTypes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

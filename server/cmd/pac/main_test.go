package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidFileTypeRejectsRenamedContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.pdf")
	if err := os.WriteFile(path, []byte("not a pdf"), 0600); err != nil {
		t.Fatal(err)
	}
	if validFileType(path, ".pdf") {
		t.Fatal("renamed text must not pass PDF detection")
	}
}

func TestValidFileTypeAcceptsPDFSignature(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\n% demo"), 0600); err != nil {
		t.Fatal(err)
	}
	if !validFileType(path, ".pdf") {
		t.Fatal("PDF signature should be accepted")
	}
}

func TestSecurityHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

package opds

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestRequestOrigin_directIP(t *testing.T) {
	r := httptest.NewRequest("GET", "http://192.168.1.10:8080/opds", nil)
	r.Host = "192.168.1.10:8080"
	got := requestOrigin(r)
	if got != "http://192.168.1.10:8080" {
		t.Fatalf("got %q, want http://192.168.1.10:8080", got)
	}
}

func TestRequestOrigin_domainHTTPS(t *testing.T) {
	r := httptest.NewRequest("GET", "https://books.example.com/opds", nil)
	r.Host = "books.example.com"
	r.TLS = &tls.ConnectionState{}
	got := requestOrigin(r)
	if got != "https://books.example.com" {
		t.Fatalf("got %q, want https://books.example.com", got)
	}
}

func TestRequestOrigin_forwardedHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/opds", nil)
	r.Host = "127.0.0.1:8080"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "books.example.com")
	got := requestOrigin(r)
	if got != "https://books.example.com" {
		t.Fatalf("got %q, want https://books.example.com", got)
	}
}

func TestRequestOrigin_forwardedSchemeAndPort(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1/opds", nil)
	r.Host = "127.0.0.1"
	r.Header.Set("X-Forwarded-Scheme", "https")
	r.Header.Set("X-Forwarded-Host", "books.example.com")
	r.Header.Set("X-Forwarded-Port", "8443")
	got := requestOrigin(r)
	if got != "https://books.example.com:8443" {
		t.Fatalf("got %q, want https://books.example.com:8443", got)
	}
}

func TestRequestOrigin_forwardedHeaderRFC7239(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1/opds", nil)
	r.Host = "127.0.0.1"
	r.Header.Set("Forwarded", `for=192.0.2.60;proto=https;host="books.example.com"`)
	got := requestOrigin(r)
	if got != "https://books.example.com" {
		t.Fatalf("got %q, want https://books.example.com", got)
	}
}

func TestOpdsBaseURL_ignoresConfiguredPublicURL(t *testing.T) {
	r := httptest.NewRequest("GET", "http://10.0.0.5/opds", nil)
	r.Host = "10.0.0.5"
	got := opdsBaseURL(r)
	if got != "http://10.0.0.5" {
		t.Fatalf("got %q, want http://10.0.0.5", got)
	}
}

func TestOpdsBaseURL_forwardedPrefix(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/opds/authors", nil)
	r.Host = "127.0.0.1:8080"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "books.example.com")
	r.Header.Set("X-Forwarded-Prefix", "/library")
	got := opdsBaseURL(r)
	if got != "https://books.example.com/library" {
		t.Fatalf("got %q, want https://books.example.com/library", got)
	}
}

func TestOpdsSelfURL_withForwardedPrefix(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/opds/authors?page=2", nil)
	r.Host = "127.0.0.1:8080"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "books.example.com")
	r.Header.Set("X-Forwarded-Prefix", "/library")
	got := opdsSelfURL(r)
	want := "https://books.example.com/library/opds/authors?page=2"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

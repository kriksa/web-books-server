package opds

import (
	"encoding/xml"
	"log"
	"net"
	"net/http"
	"strings"
)

// opdsBaseURL returns the external base URL for absolute links in OPDS feeds.
// It is derived from the incoming request so links work locally, behind nginx,
// and other reverse proxies without manual configuration.
func opdsBaseURL(r *http.Request) string {
	origin := requestOrigin(r)
	if prefix := forwardedPrefix(r); prefix != "" {
		return strings.TrimRight(origin, "/") + prefix
	}
	return origin
}

func requestOrigin(r *http.Request) string {
	scheme := requestScheme(r)
	host := requestHost(r, scheme)
	return scheme + "://" + strings.TrimRight(host, "/")
}

func requestScheme(r *http.Request) string {
	if proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto")); proto == "http" || proto == "https" {
		return proto
	}
	if scheme := firstHeaderValue(r.Header.Get("X-Forwarded-Scheme")); scheme == "http" || scheme == "https" {
		return scheme
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Ssl")), "on") {
		return "https"
	}
	if fwd := parseForwardedHeader(r.Header.Get("Forwarded")); fwd.Proto == "http" || fwd.Proto == "https" {
		return fwd.Proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func requestHost(r *http.Request, scheme string) string {
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		if fwd := parseForwardedHeader(r.Header.Get("Forwarded")); fwd.Host != "" {
			host = fwd.Host
		}
	}
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" && r.URL != nil {
		host = strings.TrimSpace(r.URL.Host)
	}
	if host == "" {
		host = "localhost"
		if r.URL != nil {
			if port := r.URL.Port(); port != "" {
				host = net.JoinHostPort(host, port)
			}
		}
	}

	if port := firstHeaderValue(r.Header.Get("X-Forwarded-Port")); port != "" {
		host = applyForwardedPort(host, scheme, port)
	}

	return host
}

func forwardedPrefix(r *http.Request) string {
	for _, key := range []string{"X-Forwarded-Prefix", "X-Script-Name"} {
		if p := firstHeaderValue(r.Header.Get(key)); p != "" {
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			return strings.TrimRight(p, "/")
		}
	}
	return ""
}

func applyForwardedPort(host, scheme, port string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		return host
	}
	return net.JoinHostPort(hostOnly(host), port)
}

func hostOnly(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

type forwardedParams struct {
	Proto string
	Host  string
}

func parseForwardedHeader(v string) forwardedParams {
	v = firstHeaderValue(v)
	if v == "" {
		return forwardedParams{}
	}
	var out forwardedParams
	for _, part := range strings.Split(v, ";") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		switch key {
		case "proto":
			if val == "http" || val == "https" {
				out.Proto = val
			}
		case "host":
			if val != "" {
				out.Host = val
			}
		}
	}
	return out
}

func opdsSelfURL(r *http.Request) string {
	u := r.URL.Path
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	return opdsBaseURL(r) + u
}

func firstHeaderValue(v string) string {
	if v == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(v, ",")[0])
}

func writeXMLResponse(w http.ResponseWriter, v any) {
	output, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Printf("Ошибка маршалинга XML: %v", err)
		http.Error(w, "Ошибка формирования XML", http.StatusInternalServerError)
		return
	}

	fullResponse := []byte(xml.Header)
	fullResponse = append(fullResponse, output...)

	if _, err := w.Write(fullResponse); err != nil {
		if strings.Contains(err.Error(), "broken pipe") {
			log.Printf("Клиент отключился во время отправки XML")
		} else {
			log.Printf("Ошибка записи XML: %v", err)
		}
	}
}

package httpx

import (
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
)

type BrowserHeaderProfile struct {
	UserAgent string
}

type BrowserRequestKind string

const (
	BrowserNavigationRequest BrowserRequestKind = "navigation"
	BrowserFormRequest       BrowserRequestKind = "form"
	BrowserAPIRequest        BrowserRequestKind = "api"
)

var browserUserAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
}

var browserUAIndex uint64

func NextBrowserUA() string {
	index := atomic.AddUint64(&browserUAIndex, 1) - 1
	return browserUserAgents[index%uint64(len(browserUserAgents))]
}

func NewBrowserHeaderProfile() BrowserHeaderProfile {
	return BrowserHeaderProfile{UserAgent: NextBrowserUA()}
}

func ApplyBrowserHeaders(req *http.Request, referer string, isForm bool) {
	kind := BrowserNavigationRequest
	if isForm {
		kind = BrowserFormRequest
	}
	ApplyBrowserHeadersWithProfile(req, referer, kind, BrowserHeaderProfile{})
}

func ApplyBrowserHeadersWithProfile(req *http.Request, referer string, kind BrowserRequestKind, profile BrowserHeaderProfile) {
	if req == nil {
		return
	}
	userAgent := strings.TrimSpace(profile.UserAgent)
	if userAgent == "" {
		userAgent = NextBrowserUA()
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", browserAcceptHeader(kind))
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	referer = strings.TrimSpace(referer)
	if referer == "" && req.URL != nil {
		referer = originOf(req.URL.String()) + "/"
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
		if origin := originOf(referer); origin != "" && kind != BrowserNavigationRequest {
			req.Header.Set("Origin", origin)
		}
	}

	if kind == BrowserFormRequest || kind == BrowserAPIRequest {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
}

func browserAcceptHeader(kind BrowserRequestKind) string {
	switch kind {
	case BrowserFormRequest:
		return "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"
	case BrowserAPIRequest:
		return "application/json, text/plain, */*"
	default:
		return "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"
	}
}

func originOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// Package fetch is the one throttled HTTP transport for every data.go.kr
// request opendatactl makes over plain HTTP — dataset search and OpenAPI describe
// (www.data.go.kr) and authenticated call (apis.data.go.kr). It hides the
// User-Agent, timeout, rate-limit throttle, and document status-gating +
// goquery parsing behind a small interface, so callers keep only their
// parse/shape logic. The CDP browser session (internal/portal/browser.go) is a
// separate transport and does not go through here.
package fetch

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// DefaultUserAgent identifies opendatactl honestly to the server operator.
const DefaultUserAgent = "opendatactl (+https://github.com/JungHoonGhae/opendatactl)"

// DefaultDelay is the minimum spacing between requests (politeness throttle).
const DefaultDelay = 700 * time.Millisecond

// DefaultMaxResponseBytes bounds a fully buffered portal/API response. Agents
// should page through bulk datasets; buffering an unbounded response in a
// long-lived MCP host can otherwise exhaust the process while decoding it.
const DefaultMaxResponseBytes int64 = 32 << 20 // 32 MiB

// ErrResponseTooLarge identifies a response that crossed the configured bound.
var ErrResponseTooLarge = fmt.Errorf("HTTP 응답이 허용 크기를 초과했습니다")

// Response is a raw HTTP response surfaced to a caller. Body is the fully-read
// payload; Status is passed through untouched (non-200 is not an error — the
// caller decides what a given status means).
type Response struct {
	Status      int
	ContentType string
	Body        []byte
}

// Client is a rate-limited, host-agnostic HTTP transport. A single Client
// shared across search/describe/call gives all of opendatactl's data.go.kr traffic
// one throttle. Safe for concurrent use.
type Client struct {
	userAgent string
	delay     time.Duration
	http      *http.Client
	maxBody   int64

	mu      sync.Mutex
	lastReq time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithDelay sets the minimum spacing between requests. Zero disables throttling.
func WithDelay(d time.Duration) Option {
	return func(c *Client) {
		if d >= 0 {
			c.delay = d
		}
	}
}

// WithMaxResponseBytes overrides the response bound. Positive values only.
func WithMaxResponseBytes(n int64) Option {
	return func(c *Client) {
		if n > 0 {
			c.maxBody = n
		}
	}
}

// New builds a Client with sane defaults.
func New(opts ...Option) *Client {
	c := &Client{
		userAgent: DefaultUserAgent,
		delay:     DefaultDelay,
		http:      NewHTTPClient(60 * time.Second),
		maxBody:   DefaultMaxResponseBytes,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// hostTransport keeps the data.go.kr TLS workaround local to that domain. API
// descriptions can point at an organisation's own host, and those requests must
// retain Go's normal TLS negotiation rather than inheriting a portal quirk.
type hostTransport struct {
	standard http.RoundTripper
	dataGoKR http.RoundTripper
}

func (t *hostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isDataGoKRHost(req.URL.Hostname()) {
		return t.dataGoKR.RoundTrip(req)
	}
	return t.standard.RoundTrip(req)
}

func isDataGoKRHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "data.go.kr" || strings.HasSuffix(host, ".data.go.kr")
}

// NewHTTPClient builds the HTTP client shared by the public and authenticated
// portal paths. data.go.kr's TLS 1.3 endpoint returned corrupt records to Go and
// Chromium (bad record MAC / ERR_SSL_PROTOCOL_ERROR, observed 2026-08-31), while
// the same endpoint completed a verified TLS 1.2 handshake. Cap only data.go.kr
// hosts at TLS 1.2; every other host keeps the standard transport.
func NewHTTPClient(timeout time.Duration) *http.Client {
	standard := http.DefaultTransport.(*http.Transport).Clone()
	dataGoKR := standard.Clone()
	dataGoKR.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &hostTransport{
			standard: standard,
			dataGoKR: dataGoKR,
		},
	}
}

// ponytail: single throttle across www+apis; split per-host if throughput matters.
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.delay > 0 && !c.lastReq.IsZero() {
		if wait := c.delay - time.Since(c.lastReq); wait > 0 {
			time.Sleep(wait)
		}
	}
	c.lastReq = time.Now()
}

// Get performs a throttled GET on a full URL and returns the raw response. Any
// HTTP status is returned as a *Response (data.go.kr answers 200 — occasionally
// non-200 — with a meaningful body); only a transport failure returns an error.
// The error, when non-nil, may include the URL — a caller passing secrets in the
// query (e.g. serviceKey) must redact it before surfacing.
func (c *Client) Get(ctx context.Context, rawURL string) (*Response, error) {
	return c.do(ctx, http.MethodGet, rawURL, nil, "")
}

// do is the one HTTP boundary so GET pages and the portal's form-backed detail
// fragments share the same throttle, headers, TLS policy, and response shape.
func (c *Client) do(ctx context.Context, method, rawURL string, requestBody io.Reader, contentType string) (*Response, error) {
	c.throttle()
	req, err := http.NewRequestWithContext(ctx, method, rawURL, requestBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/json,application/xml,*/*")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if ref := refererOf(rawURL); ref != "" {
		req.Header.Set("Referer", ref)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, rawURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > c.maxBody {
		return nil, fmt.Errorf("%w: %s %s (%d bytes 제한) — numOfRows를 줄여 페이지 단위로 호출하세요",
			ErrResponseTooLarge, method, rawURL, c.maxBody)
	}
	return &Response{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
	}, nil
}

// GetDoc performs a Get and parses the body as HTML. Unlike Get it gates on 200:
// the pages GetDoc serves (search list, OpenAPI detail) are only meaningful when
// served 200, so a non-200 is an error rather than a document to scrape.
func (c *Client) GetDoc(ctx context.Context, rawURL string) (*goquery.Document, error) {
	res, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if res.Status != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %d", rawURL, res.Status)
	}
	return goquery.NewDocumentFromReader(bytes.NewReader(res.Body))
}

// PostFormDoc submits an application/x-www-form-urlencoded POST and parses its
// HTML response. The redesigned portal uses this for one selected OpenAPI
// operation at a time; using the fragment endpoint avoids driving the UI and
// still keeps all data.go.kr requests behind the shared polite transport.
func (c *Client) PostFormDoc(ctx context.Context, rawURL string, form url.Values) (*goquery.Document, error) {
	res, err := c.do(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return nil, err
	}
	if res.Status != http.StatusOK {
		return nil, fmt.Errorf("POST %s: unexpected status %d", rawURL, res.Status)
	}
	return goquery.NewDocumentFromReader(bytes.NewReader(res.Body))
}

// refererOf returns "scheme://host/" for a URL, or "" if it can't be parsed.
// data.go.kr's front end is friendlier to requests that carry a same-site Referer.
func refererOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

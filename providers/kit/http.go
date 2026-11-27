package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/millerscout/qrtx/tracker"
)

// AuthFunc sets the credential headers on a request.
type AuthFunc func(r *http.Request, c Creds)

// BearerAuth sets Authorization: Bearer <token>.
func BearerAuth(r *http.Request, c Creds) { r.Header.Set("Authorization", "Bearer "+c.Token) }

// BasicAuth sets HTTP basic auth with email and token.
func BasicAuth(r *http.Request, c Creds) { r.SetBasicAuth(c.Email, c.Token) }

// Client is an HTTP client for one API base URL and scope. Credentials are
// resolved for the base URL's host before any request is made.
type Client struct {
	Base      *url.URL
	Headers   http.Header
	creds     Creds
	auth      AuthFunc
	hc        *http.Client
	deadline  time.Time
	sleep     func(time.Duration)
	userAgent string
}

// Deadline is $QRTX_TRACKER_DEADLINE (set by qrtx for each plugin call), or 20s
// from now.
func Deadline() time.Time {
	if v := os.Getenv("QRTX_TRACKER_DEADLINE"); v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t
		}
	}
	return time.Now().Add(20 * time.Second)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// NewClient checks base (https, or http only for a loopback host), resolves
// the credentials bound to its host for scope, and returns a client that
// refuses redirects to another scheme, host or port.
func NewClient(base, scope, userAgent string, auth AuthFunc) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Host == "" {
		return nil, &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: fmt.Sprintf("API URL %q is not a URL", base)}
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && isLoopback(u.Hostname()):
	default:
		return nil, &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: fmt.Sprintf("API URL %s must use https", base)}
	}
	creds, err := Credentials(u.Host, scope)
	if err != nil {
		return nil, err
	}
	c := &Client{Base: u, Headers: http.Header{}, creds: creds, auth: auth, deadline: Deadline(), sleep: time.Sleep, userAgent: userAgent}
	c.hc = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		first := via[0].URL
		if req.URL.Scheme != first.Scheme || !strings.EqualFold(req.URL.Host, first.Host) {
			return &tracker.AdapterError{Code: tracker.ErrCodeUnavailable, Message: fmt.Sprintf("refused redirect from %s to %s://%s", first.Host, req.URL.Scheme, req.URL.Host)}
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	}}
	return c, nil
}

// Request is one API call.
type Request struct {
	Method string
	Path   string // joined to the base path; may carry a query
	Query  url.Values
	Body   any
	// Safe marks a non-idempotent request as safe to retry (a create guarded
	// by client_ref). GET, PUT and PATCH are always safe.
	Safe bool
}

// Do sends req and decodes a JSON response into out (when non-nil). It
// returns the response headers. Failures are *tracker.AdapterError.
func (c *Client) Do(req Request, out any) (http.Header, error) {
	safe := req.Safe || req.Method == http.MethodGet || req.Method == http.MethodPut || req.Method == http.MethodPatch
	for attempt := 0; ; attempt++ {
		hdr, status, body, err := c.once(req)
		if err == nil && status < 300 {
			if out != nil && len(bytes.TrimSpace(body)) > 0 {
				if err := json.Unmarshal(body, out); err != nil {
					return hdr, &tracker.AdapterError{Code: tracker.ErrCodeUnavailable, Message: "malformed API response: " + err.Error()}
				}
			}
			return hdr, nil
		}
		aerr := mapFailure(status, hdr, body, err)
		var ae *tracker.AdapterError
		if errors.As(err, &ae) {
			aerr = ae // refused redirect
		}
		retryable := aerr.Code == tracker.ErrCodeUnavailable || aerr.Code == tracker.ErrCodeRateLimited
		if attempt > 0 || !safe || !retryable || (ae != nil && strings.HasPrefix(ae.Message, "refused redirect")) {
			return hdr, aerr
		}
		wait := time.Second
		if aerr.RetryAfter > 0 {
			wait = time.Duration(aerr.RetryAfter) * time.Second
		}
		if time.Now().Add(wait + time.Second).After(c.deadline) {
			return hdr, aerr
		}
		c.sleep(wait)
	}
}

func (c *Client) once(req Request) (http.Header, int, []byte, error) {
	u := *c.Base
	p := req.Path
	if i := strings.Index(p, "?"); i >= 0 {
		u.RawQuery = p[i+1:]
		p = p[:i]
	}
	u.Path = strings.TrimRight(c.Base.Path, "/") + "/" + strings.TrimLeft(p, "/")
	if req.Query != nil {
		q := u.Query()
		for k, vs := range req.Query {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		u.RawQuery = q.Encode()
	}
	var rd io.Reader
	if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return nil, 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithDeadline(context.Background(), c.deadline)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, req.Method, u.String(), rd)
	if err != nil {
		return nil, 0, nil, err
	}
	for k, vs := range c.Headers {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	r.Header.Set("Accept", firstNonEmpty(r.Header.Get("Accept"), "application/json"))
	if req.Body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("User-Agent", c.userAgent)
	c.auth(r, c.creds)
	resp, err := c.hc.Do(r)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			var ae *tracker.AdapterError
			if errors.As(ue.Err, &ae) {
				return nil, 0, nil, ae
			}
		}
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return resp.Header, resp.StatusCode, body, err
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func apiMessage(body []byte) string {
	var m struct {
		Message       string   `json:"message"`
		ErrorMessages []string `json:"errorMessages"`
		Errors        any      `json:"errors"`
	}
	if json.Unmarshal(body, &m) == nil {
		if m.Message != "" {
			if m.Errors != nil {
				b, _ := json.Marshal(m.Errors)
				return m.Message + ": " + string(b)
			}
			return m.Message
		}
		if len(m.ErrorMessages) > 0 {
			return strings.Join(m.ErrorMessages, "; ")
		}
		if m.Errors != nil {
			b, _ := json.Marshal(m.Errors)
			return string(b)
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func retryAfter(h http.Header) int {
	if v := h.Get("Retry-After"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	if h.Get("X-Ratelimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(h.Get("X-Ratelimit-Reset"), 10, 64); err == nil {
			if d := time.Until(time.Unix(reset, 0)); d > 0 {
				return int(d.Seconds()) + 1
			}
		}
	}
	return 0
}

func mapFailure(status int, h http.Header, body []byte, err error) *tracker.AdapterError {
	if err != nil {
		return &tracker.AdapterError{Code: tracker.ErrCodeUnavailable, Message: err.Error()}
	}
	msg := fmt.Sprintf("HTTP %d: %s", status, apiMessage(body))
	switch {
	case status == http.StatusTooManyRequests || (status == http.StatusForbidden && (h.Get("X-Ratelimit-Remaining") == "0" || h.Get("Retry-After") != "")):
		return &tracker.AdapterError{Code: tracker.ErrCodeRateLimited, Message: msg, RetryAfter: retryAfter(h)}
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &tracker.AdapterError{Code: tracker.ErrCodeAuth, Message: msg}
	case status == http.StatusNotFound || status == http.StatusGone:
		return &tracker.AdapterError{Code: tracker.ErrCodeNotFound, Message: msg}
	case status >= 500:
		return &tracker.AdapterError{Code: tracker.ErrCodeUnavailable, Message: msg, RetryAfter: retryAfter(h)}
	case status >= 300 && status < 400:
		return &tracker.AdapterError{Code: tracker.ErrCodeUnavailable, Message: msg}
	default:
		return &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: msg}
	}
}

// SetHTTPTransport replaces the transport (tests use it for TLS fakes). The
// redirect policy is kept.
func (c *Client) SetHTTPTransport(rt http.RoundTripper) { c.hc.Transport = rt }

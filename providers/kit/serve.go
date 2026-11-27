// Package kit is the plugin side of the sdd.adapter/v1 protocol: it reads the
// request qrtx writes to stdin, dispatches it to a provider, writes the one
// response and picks the exit code. It also gives providers host-bound
// credentials and an HTTP client that maps API failures to adapter error
// codes. Unstable until qrtx v1.
package kit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/millerscout/qrtx/tracker"
)

// Handler serves every op of one envelope kind.
type Handler interface {
	Handle(op string, config, args json.RawMessage) (any, error)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(op string, config, args json.RawMessage) (any, error)

// Handle calls f.
func (f HandlerFunc) Handle(op string, config, args json.RawMessage) (any, error) {
	return f(op, config, args)
}

// Main serves one request from stdin to stdout and exits.
func Main(kinds map[string]Handler) {
	os.Exit(Serve(kinds, os.Stdin, os.Stdout))
}

// Serve reads one envelope from in, dispatches it by kind and op, writes one
// response to out and returns the exit code (0 with result, 1 with error).
func Serve(kinds map[string]Handler, in io.Reader, out io.Writer) int {
	raw, err := io.ReadAll(in)
	if err != nil {
		return respond(out, nil, invalidf("reading request: %v", err))
	}
	var req tracker.AdapterRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return respond(out, nil, invalidf("request is not JSON: %v", err))
	}
	if req.Protocol != tracker.AdapterProtocol {
		return respond(out, nil, invalidf("protocol %q is not supported; this plugin speaks %s", req.Protocol, tracker.AdapterProtocol))
	}
	h, ok := kinds[req.Kind]
	if !ok {
		return respond(out, nil, invalidf("kind %q is not served by this plugin", req.Kind))
	}
	res, err := h.Handle(req.Op, req.Config, req.Args)
	return respond(out, res, err)
}

func invalidf(format string, a ...any) error {
	return &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: fmt.Sprintf(format, a...)}
}

func respond(out io.Writer, result any, err error) int {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	if err != nil {
		var ae *tracker.AdapterError
		if !errors.As(err, &ae) {
			ae = &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: err.Error()}
		}
		_ = enc.Encode(tracker.AdapterResponse{Error: ae})
		return 1
	}
	if result == nil {
		result = struct{}{}
	}
	b, merr := json.Marshal(result)
	if merr != nil {
		_ = enc.Encode(tracker.AdapterResponse{Error: &tracker.AdapterError{Code: tracker.ErrCodeInvalid, Message: merr.Error()}})
		return 1
	}
	_ = enc.Encode(tracker.AdapterResponse{Result: b})
	return 0
}

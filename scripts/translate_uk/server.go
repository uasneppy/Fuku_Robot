package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

// maxBody caps a write request. The longest locale string is a few KB; a megabyte
// is generous and still stops a runaway upload.
const maxBody = 1 << 20

type server struct{ p *Project }

// newServer returns the editor's handler. The guards exist because this server
// writes files and listens on a port any page in the same browser can try to reach:
// a Host check stops DNS rebinding, and the JSON content type plus the Origin
// check stop a cross-site form or fetch from posting a translation.
func newServer(p *Project) http.Handler {
	s := &server{p: p}
	mux := http.NewServeMux()
	// No method in the patterns on purpose: the handlers answer a wrong method
	// themselves, with an Allow header.
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/api/state", s.state)
	mux.HandleFunc("/api/check", s.post(s.p.Preview))
	mux.HandleFunc("/api/save", s.post(s.p.Save))
	mux.HandleFunc("/api/confirm", s.post(s.p.Confirm))
	return s.guard(mux)
}

func hostAllowed(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func originAllowed(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "http" && hostAllowed(u.Host)
}

func (s *server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !hostAllowed(r.Host) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this editor only answers on localhost"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// The page is one file with inline script and style and loads nothing from
	// anywhere; this policy keeps it that way and stops it being framed.
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	h.Set("X-Frame-Options", "DENY")
	_, _ = w.Write(indexHTML)
}

func (s *server) state(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	st, err := s.p.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// post builds a handler for a JSON {key, uk} request that runs action on it. check,
// save and confirm share it, and with it every guard.
func (s *server) post(action func(key string, uk []string) (KeyState, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "send application/json"})
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !originAllowed(origin) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-site request refused"})
			return
		}
		var req struct {
			Key string   `json:"key"`
			UK  []string `json:"uk"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		dec.DisallowUnknownFields()
		err := dec.Decode(&req)
		if err == nil {
			// A second JSON value after the first is as malformed as no JSON.
			if extra := dec.Decode(&struct{}{}); !errors.Is(extra, io.EOF) {
				err = errors.New("unexpected data after the JSON object")
			}
		}
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request too large"})
			return
		}
		if err != nil || req.Key == "" || req.UK == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be {\"key\": string, \"uk\": [string, ...]}"})
			return
		}
		ks, err := action(req.Key, req.UK)
		var ve *ValidationError
		switch {
		case errors.As(err, &ve):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"issues": ve.Issues})
		case errors.Is(err, ErrUnknownKey):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusOK, ks)
		}
	}
}

func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad -addr %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%q is not a loopback address: the editor writes files, so it only listens on 127.0.0.1, ::1 or localhost", addr)
	}
	return nil
}

// serve runs the editor until interrupted.
func serve(addr string, p *Project, stdout io.Writer) error {
	if err := requireLoopback(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w (is another copy running? pick a port with -addr)", addr, err)
	}
	srv := &http.Server{
		Handler:           newServer(p),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	fmt.Fprintf(stdout, "Translation editor: open http://%s in your browser (Ctrl+C to stop)\n", ln.Addr())
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

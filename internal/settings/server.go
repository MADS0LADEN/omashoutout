package settings

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/lkarlslund/shoutout/internal/config"
	"github.com/lkarlslund/shoutout/internal/discovery"
	"github.com/lkarlslund/shoutout/internal/service"
)

const Address = "127.0.0.1:17832"

//go:embed index.html
var page []byte

func Handler(s *service.Service) (http.Handler, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(secret[:])
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "shoutout-session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": s.Status(), "config": s.Config()})
	})
	mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		devices, err := discovery.Discover(ctx)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if devices == nil {
			devices = []discovery.Device{}
		}
		writeJSON(w, devices)
	})
	mux.HandleFunc("POST /api/config", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("shoutout-session")
		if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) != 1 || r.Header.Get("Origin") != "http://"+Address {
			http.Error(w, "Open settings from the local application address.", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var c config.Config
		if err = d.Decode(&c); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			http.Error(w, "unexpected trailing data", 400)
			return
		}
		if err = s.Update(c); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, c)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != Address {
			http.Error(w, "invalid host", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+Address {
			http.Error(w, "invalid origin", 403)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	}), nil
}
func Run(ctx context.Context, s *service.Service, listener net.Listener) error {
	h, err := Handler(s)
	if err != nil {
		return err
	}
	server := http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			server.Close()
		case <-done:
		}
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

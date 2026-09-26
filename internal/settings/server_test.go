package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/lkarlslund/shoutout/internal/config"
	"github.com/lkarlslund/shoutout/internal/service"
)

func TestLocalSettingsBoundary(t *testing.T) {
	s := service.New(config.Default(), filepath.Join(t.TempDir(), "config.json"))
	h, err := Handler(s)
	if err != nil {
		t.Fatal(err)
	}
	home := httptest.NewRequest("GET", "http://"+Address+"/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, home)
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("missing session cookie")
	}
	for _, test := range []struct {
		name, origin string
		cookie       bool
		volume       float64
		code         int
	}{{"no cookie", "http://" + Address, false, 0.01, 403}, {"foreign origin", "https://example.com", true, 0.01, 403}, {"unsafe volume", "http://" + Address, true, 0.06, 400}, {"valid", "http://" + Address, true, 0.01, 200}} {
		t.Run(test.name, func(t *testing.T) {
			c := config.Default()
			c.ReceiverVolume = test.volume
			b, _ := json.Marshal(c)
			r := httptest.NewRequest("POST", "http://"+Address+"/api/config", bytes.NewReader(b))
			r.Header.Set("Origin", test.origin)
			if test.cookie {
				r.AddCookie(cookies[0])
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.code {
				t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
			}
		})
	}
	r := httptest.NewRequest(http.MethodGet, "http://evil.example/api/status", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign host accepted")
	}
}

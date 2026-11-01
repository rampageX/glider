package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nadoo/glider/rule"
	"github.com/nadoo/glider/subscription"
)

func TestStatusIncludesSubscriptionsAndStaticMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p := rule.NewProxy([]string{"direct://"}, &rule.Strategy{Strategy: "rr", DialTimeout: 1, Check: "disable"}, nil)
	defer p.Close()
	provider := func() subscription.Status {
		return subscription.Status{Inputs: []subscription.InputStatus{{Title: "订阅一", URL: "https://example.invalid/…?…", Result: "SUCCESS"}}}
	}
	for _, s := range []*Server{New("", p), New("", p, provider)} {
		router := gin.New()
		router.GET("/api/status", s.handleStatus)
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if out.Code != 200 || out.Header().Get("Cache-Control") != "no-store" || (!strings.Contains(out.Body.String(), `"subscriptions"`) || !strings.Contains(out.Body.String(), `"protocol":"DIRECT"`)) {
			t.Fatal(out.Body.String())
		}
		if s.subscriptions != nil && !strings.Contains(out.Body.String(), "订阅一") {
			t.Fatal("missing title")
		}
	}
}

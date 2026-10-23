package hscontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apiv1 "github.com/juanfont/headscale/hscontrol/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPublicRouterOmitsAdminAPI proves /api is not on the listener inferscale
// dials. Admin stays on the unix socket. A bearer token does not bring it back.
func TestPublicRouterOmitsAdminAPI(t *testing.T) {
	app := createTestApp(t)
	handler := app.HTTPHandler()

	expiry := time.Now().Add(time.Hour)
	valid, _, err := app.state.CreateAPIKey(&expiry)
	require.NoError(t, err)

	paths := []string{
		"/api/v1/node",
		"/api/v1/openapi.yaml",
		"/api/v1/docs",
		"/api/v2/tailnet/-/devices",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			for _, auth := range []string{"", "Bearer " + valid} {
				req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
				if auth != "" {
					req.Header.Set("Authorization", auth)
				}

				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				assert.Equalf(t, http.StatusNotFound, rec.Code, "auth %q body: %s", auth, rec.Body.String())
				assert.NotContains(t, rec.Body.String(), "nodes")
			}
		})
	}

	// Control plane stays. /health is what inferscaled checks.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
}

// TestAPIV1AuthMiddleware proves the v1 mux still rejects a missing or bad
// bearer before the handler. The public router does not mount this mux.
func TestAPIV1AuthMiddleware(t *testing.T) {
	app := createTestApp(t)
	handler, _ := apiv1.Handler(apiv1.Backend{
		State:  app.state,
		Change: app.Change,
		Cfg:    app.cfg,
	})

	expiry := time.Now().Add(time.Hour)
	valid, _, err := app.state.CreateAPIKey(&expiry)
	require.NoError(t, err)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{name: "missing bearer", authHeader: "", wantStatus: http.StatusUnauthorized},
		{name: "no bearer prefix", authHeader: "tskey-invalid", wantStatus: http.StatusUnauthorized},
		{name: "invalid bearer token", authHeader: "Bearer tskey-invalid", wantStatus: http.StatusUnauthorized},
		{name: "valid api key", authHeader: "Bearer " + valid, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(
				context.Background(), http.MethodGet, "/api/v1/node", nil,
			)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equalf(t, tt.wantStatus, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

// TestAPIV1Unauthorized401 pins the 401 body on the v1 mux: RFC 7807 JSON
// containing "Unauthorized", under 100 bytes, and leaking no data.
func TestAPIV1Unauthorized401(t *testing.T) {
	app := createTestApp(t)
	handler, _ := apiv1.Handler(apiv1.Backend{
		State:  app.state,
		Change: app.Change,
		Cfg:    app.cfg,
	})

	req := httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/api/v1/node", nil,
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)

	body := rec.Body.Bytes()
	assert.Contains(t, string(body), "Unauthorized")
	assert.Lessf(t, len(body), 100, "401 body must stay small (no data leak): %s", body)

	var problem map[string]any
	require.NoError(t, json.Unmarshal(body, &problem), "401 body must be JSON: %s", body)
	assert.NotContains(t, problem, "users")
}

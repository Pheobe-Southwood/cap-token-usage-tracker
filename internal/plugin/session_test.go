package plugin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// testSessionToken mints a dashboard session for a runtime under test. Resource
// routes reject requests without one, so tests that exercise data endpoints
// must present it explicitly.
func testSessionToken(t *testing.T, r *pluginRuntime) string {
	t.Helper()
	token, _, err := r.createSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return token
}

// authedCall dispatches a management request with a valid dashboard session so
// tests keep exercising the data handlers instead of the access gate.
func authedCall(t *testing.T, r *pluginRuntime, raw []byte) (pluginapi.ManagementResponse, error) {
	t.Helper()
	var request pluginapi.ManagementRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return r.handleManagement(raw)
	}
	if request.Headers == nil {
		request.Headers = http.Header{}
	}
	if request.Headers.Get(pluginSessionHeader) == "" {
		request.Headers.Set(pluginSessionHeader, testSessionToken(t, r))
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	return r.handleManagement(encoded)
}

func testRuntimeWithRoutes(t *testing.T) *pluginRuntime {
	t.Helper()
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	raw, err := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.registerManagement(raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.shutdown() })
	return runtime
}

func callResource(t *testing.T, runtime *pluginRuntime, path string, session string) pluginapi.ManagementResponse {
	t.Helper()
	headers := http.Header{}
	if session != "" {
		headers.Set(pluginSessionHeader, session)
	}
	raw, err := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    path,
		Query:   url.Values{"range": {"24h"}},
		Headers: headers,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := runtime.handleManagement(raw)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestPluginSessionGateProtectsEveryDataRoute(t *testing.T) {
	runtime := testRuntimeWithRoutes(t)
	protected := sessionProtectedResourcePaths(runtime.routes)
	if len(protected) != 8 {
		t.Fatalf("protected routes = %v", protected)
	}
	for _, path := range protected {
		response := callResource(t, runtime, path, "")
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s without session = %d, want 401", path, response.StatusCode)
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Body, &payload); err != nil || payload["code"] != "session_required" {
			t.Fatalf("%s unauthorized payload = %s (%v)", path, response.Body, err)
		}
		if got := response.Headers.Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s Cache-Control = %q", path, got)
		}
		session := testSessionToken(t, runtime)
		if response = callResource(t, runtime, path, session); response.StatusCode == http.StatusUnauthorized {
			t.Fatalf("%s with valid session was rejected: %s", path, response.Body)
		}
	}
	// The dashboard shell stays reachable: it ships no data and the host serves
	// resource routes without an ambient credential.
	shell := callResource(t, runtime, runtime.routes.dashboardPath, "")
	if shell.StatusCode != http.StatusOK || len(shell.Body) == 0 {
		t.Fatalf("dashboard shell = %d (%d bytes)", shell.StatusCode, len(shell.Body))
	}
}

func TestPluginSessionRejectsMalformedAndExpiredTokens(t *testing.T) {
	runtime := testRuntimeWithRoutes(t)
	session := testSessionToken(t, runtime)
	if !runtime.validSession(session) {
		t.Fatal("fresh session rejected")
	}
	for _, invalid := range []string{"", " ", "not-base64", "AAAA", session + "x", session[:len(session)-1]} {
		if runtime.validSession(invalid) {
			t.Fatalf("accepted invalid session %q", invalid)
		}
	}
	digest, ok := sessionDigest(session)
	if !ok {
		t.Fatal("session digest failed")
	}
	runtime.sessionMu.Lock()
	runtime.sessions[digest] = time.Now().UTC().Add(-time.Second)
	runtime.sessionMu.Unlock()
	if runtime.validSession(session) {
		t.Fatal("expired session accepted")
	}
	if response := callResource(t, runtime, runtime.routes.resourceStatsInitialPath, session); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired session status = %d", response.StatusCode)
	}
}

func TestPluginSessionRevoke(t *testing.T) {
	runtime := testRuntimeWithRoutes(t)
	session := testSessionToken(t, runtime)
	raw, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    runtime.routes.sessionRevokePath,
		Headers: http.Header{pluginSessionHeader: []string{session}},
	})
	if response, err := runtime.handleManagement(raw); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("revoke response = %+v, %v", response, err)
	}
	if runtime.validSession(session) {
		t.Fatal("revoked session still valid")
	}
}

func TestPluginSessionTTLUsesConfiguredMinutes(t *testing.T) {
	config := testConfig(t)
	config.SessionTTLMinutes = 42
	runtime := &pluginRuntime{config: config}
	_, expires, err := runtime.createSession()
	if err != nil {
		t.Fatal(err)
	}
	want := time.Now().UTC().Add(42 * time.Minute)
	if delta := expires.Sub(want); delta < -time.Minute || delta > time.Minute {
		t.Fatalf("session expiry = %v, want about %v", expires, want)
	}
	config.SessionTTLMinutes = 0
	runtime = &pluginRuntime{config: config}
	_, expires, err = runtime.createSession()
	if err != nil {
		t.Fatal(err)
	}
	want = time.Now().UTC().Add(defaultSessionTTLMinutes * time.Minute)
	if delta := expires.Sub(want); delta < -time.Minute || delta > time.Minute {
		t.Fatalf("default session expiry = %v, want about %v", expires, want)
	}
}

func TestPluginSessionTableIsBounded(t *testing.T) {
	runtime := &pluginRuntime{config: testConfig(t)}
	var first string
	for index := 0; index <= maxActiveSessions; index++ {
		token, _, err := runtime.createSession()
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			first = token
		}
	}
	runtime.sessionMu.Lock()
	count := len(runtime.sessions)
	runtime.sessionMu.Unlock()
	if count > maxActiveSessions {
		t.Fatalf("active sessions = %d, want <= %d", count, maxActiveSessions)
	}
	if runtime.validSession(first) {
		t.Fatal("oldest session survived the table cap")
	}
}

func TestPluginSessionHeadersNeverAppearInResponses(t *testing.T) {
	runtime := testRuntimeWithRoutes(t)
	session := testSessionToken(t, runtime)
	response := callResource(t, runtime, runtime.routes.resourceStatsInitialPath, session)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("initial stats = %d", response.StatusCode)
	}
	if response.Headers.Get("X-Plugin-Session") != "" {
		t.Fatal("session echoed in response headers")
	}
}

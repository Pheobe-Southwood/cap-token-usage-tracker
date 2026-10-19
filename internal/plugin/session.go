package plugin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// The plugin serves its browser dashboard from an unauthenticated host route
// ("/v0/resource/plugins/<id>/..."), so the plugin enforces its own access
// control: every data route requires a session token that can only be minted
// through the management API, which the host protects with the CLIProxyAPI
// management key. A dashboard visitor therefore has to authenticate exactly
// like the Web UI does before any usage data leaves the plugin.
const (
	pluginSessionHeader = "X-Plugin-Session"
	// maxActiveSessions bounds the in-memory session table. Sessions are only
	// ever held as SHA-256 digests, never as the plaintext token.
	maxActiveSessions = 256
)

func (r *pluginRuntime) sessionTTL() time.Duration {
	r.mu.RLock()
	minutes := r.config.SessionTTLMinutes
	r.mu.RUnlock()
	if minutes <= 0 {
		minutes = defaultSessionTTLMinutes
	}
	return time.Duration(minutes) * time.Minute
}

// createSession mints a dashboard session token. The returned token is the only
// copy: the runtime stores a digest so a memory disclosure does not hand out a
// usable credential.
func (r *pluginRuntime) createSession() (string, time.Time, error) {
	ttl := r.sessionTTL()
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return "", time.Time{}, err
	}
	now := nowUTC()
	expires := now.Add(ttl)
	digest := sha256.Sum256(tokenBytes[:])

	r.sessionMu.Lock()
	if r.sessions == nil {
		r.sessions = make(map[[32]byte]time.Time)
	}
	r.pruneSessionsLocked(now)
	for len(r.sessions) >= maxActiveSessions {
		r.evictOldestSessionLocked()
	}
	r.sessions[digest] = expires
	r.sessionMu.Unlock()

	return base64.RawURLEncoding.EncodeToString(tokenBytes[:]), expires, nil
}

func (r *pluginRuntime) validSession(raw string) bool {
	digest, ok := sessionDigest(raw)
	if !ok {
		return false
	}
	return r.validSessionDigest(digest)
}

func (r *pluginRuntime) validSessionDigest(digest [32]byte) bool {
	now := nowUTC()
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	r.pruneSessionsLocked(now)
	expires, exists := r.sessions[digest]
	return exists && now.Before(expires)
}

func (r *pluginRuntime) revokeSession(raw string) {
	digest, ok := sessionDigest(raw)
	if !ok {
		return
	}
	r.sessionMu.Lock()
	delete(r.sessions, digest)
	r.sessionMu.Unlock()
}

func (r *pluginRuntime) pruneSessionsLocked(now time.Time) {
	for digest, expires := range r.sessions {
		if !now.Before(expires) {
			delete(r.sessions, digest)
		}
	}
}

func (r *pluginRuntime) evictOldestSessionLocked() {
	var oldest [32]byte
	var oldestExpiry time.Time
	first := true
	for digest, expires := range r.sessions {
		if first || expires.Before(oldestExpiry) {
			oldest, oldestExpiry, first = digest, expires, false
		}
	}
	if !first {
		delete(r.sessions, oldest)
	}
}

func sessionDigest(raw string) ([32]byte, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 128 {
		return [32]byte{}, false
	}
	tokenBytes, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(tokenBytes) != 32 {
		return [32]byte{}, false
	}
	return sha256.Sum256(tokenBytes), true
}

func pluginSessionFromRequest(request pluginapi.ManagementRequest) string {
	return request.Headers.Get(pluginSessionHeader)
}

func (r *pluginRuntime) sessionResponse() (pluginapi.ManagementResponse, error) {
	token, expires, err := r.createSession()
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": "could not create dashboard session"}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{
		"session":    token,
		"expires_at": expires,
	}), nil
}

func (r *pluginRuntime) revokeSessionResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	r.revokeSession(pluginSessionFromRequest(request))
	return jsonResponse(http.StatusOK, map[string]bool{"revoked": true}), nil
}

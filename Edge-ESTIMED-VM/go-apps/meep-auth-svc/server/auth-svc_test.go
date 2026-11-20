/*
 * Copyright (c) 2022  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sessions"
	"github.com/gorilla/mux"
)

// newTestPermissionsConfig builds a small config exercising a default, an
// endpoint-specific override, and a service-level default fallback.
func newTestPermissionsConfig() *PermissionsConfig {
	return &PermissionsConfig{
		Default: Permission{Mode: sm.ModeAllow},
		Services: []Service{
			{
				Name: "test-svc",
				Path: "/test-svc/v1",
				Sbox: false,
				Default: Permission{
					Mode:  sm.ModeAllow,
					Roles: map[string]string{},
				},
				Endpoints: []Endpoint{
					{
						Name:   "Blocked",
						Path:   "/blocked",
						Method: "GET",
						Sbox:   false,
						Mode:   sm.ModeBlock,
						Roles:  map[string]string{},
					},
				},
			},
		},
	}
}

// setupTestAuthSvc rebuilds the global authSvc permission cache & router from
// an in-memory config, bypassing the /permissions.yaml file on disk.
func setupTestAuthSvc(cfg *PermissionsConfig) {
	authSvc = new(AuthSvc)
	authSvc.router = mux.NewRouter().StrictSlash(true)
	cacheDefaultPermission(cfg)
	cacheServicePermissions(cfg)
	cacheFileserverPermissions(cfg)
}

func TestCacheDefaultPermission_ExplicitMode(t *testing.T) {
	cfg := &PermissionsConfig{Default: Permission{Mode: sm.ModeBlock}}
	authSvc = new(AuthSvc)
	cacheDefaultPermission(cfg)

	if authSvc.cache.Default == nil || authSvc.cache.Default.Mode != sm.ModeBlock {
		t.Fatalf("expected cached default mode %q, got %+v", sm.ModeBlock, authSvc.cache.Default)
	}
}

func TestCacheDefaultPermission_MissingModeFallsBackToAllow(t *testing.T) {
	cfg := &PermissionsConfig{Default: Permission{}}
	authSvc = new(AuthSvc)
	cacheDefaultPermission(cfg)

	if authSvc.cache.Default == nil || authSvc.cache.Default.Mode != sm.ModeAllow {
		t.Fatalf("expected fallback allow-all default, got %+v", authSvc.cache.Default)
	}
}

func newAuthRequest(originalURL, originalMethod, svc string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/auth/v1/authenticate?svc="+svc, nil)
	if originalURL != "" {
		req.Header.Set("X-Original-URL", originalURL)
	}
	if originalMethod != "" {
		req.Header.Set("X-Original-Method", originalMethod)
	}
	return req
}

func TestAsAuthenticate_MissingOriginalHeaders(t *testing.T) {
	setupTestAuthSvc(newTestPermissionsConfig())

	w := httptest.NewRecorder()
	asAuthenticate(w, newAuthRequest("", "", "test-svc"))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with missing original headers, got %d", w.Code)
	}
}

func TestAsAuthenticate_EndpointBlockOverride(t *testing.T) {
	setupTestAuthSvc(newTestPermissionsConfig())

	w := httptest.NewRecorder()
	asAuthenticate(w, newAuthRequest("/test-svc/v1/blocked", "GET", "test-svc"))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for blocked endpoint, got %d", w.Code)
	}
}

func TestAsAuthenticate_ServiceDefaultAllow(t *testing.T) {
	setupTestAuthSvc(newTestPermissionsConfig())

	w := httptest.NewRecorder()
	asAuthenticate(w, newAuthRequest("/test-svc/v1/other", "GET", "test-svc"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 via service default allow, got %d", w.Code)
	}
}

func TestAsAuthenticate_UnmatchedRouteUsesGlobalDefault(t *testing.T) {
	setupTestAuthSvc(newTestPermissionsConfig())

	w := httptest.NewRecorder()
	asAuthenticate(w, newAuthRequest("/nomatch", "GET", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 via global default allow, got %d", w.Code)
	}
}

func TestLoginLockout(t *testing.T) {
	loginAttemptsMutex.Lock()
	loginAttemptsByUser = make(map[string]*loginAttemptTracker)
	loginAttemptsMutex.Unlock()

	t.Setenv("MEEP_LOGIN_MAX_ATTEMPTS", "3")
	t.Setenv("MEEP_LOGIN_LOCKOUT_SECONDS", "1")

	const user = "alice"

	if isLoginLocked(user) {
		t.Fatal("user should not be locked before any failures")
	}

	recordLoginFailure(user)
	recordLoginFailure(user)
	if isLoginLocked(user) {
		t.Fatal("user should not be locked before reaching the attempt threshold")
	}

	recordLoginFailure(user)
	if !isLoginLocked(user) {
		t.Fatal("user should be locked out after reaching the attempt threshold")
	}

	time.Sleep(1200 * time.Millisecond)
	if isLoginLocked(user) {
		t.Fatal("lockout should have expired")
	}
}

func TestLoginLockout_ResetOnSuccess(t *testing.T) {
	loginAttemptsMutex.Lock()
	loginAttemptsByUser = make(map[string]*loginAttemptTracker)
	loginAttemptsMutex.Unlock()

	t.Setenv("MEEP_LOGIN_MAX_ATTEMPTS", "3")
	t.Setenv("MEEP_LOGIN_LOCKOUT_SECONDS", "60")

	const user = "bob"
	recordLoginFailure(user)
	recordLoginFailure(user)
	resetLoginAttempts(user)
	recordLoginFailure(user)

	if isLoginLocked(user) {
		t.Fatal("failure count should have been reset by resetLoginAttempts")
	}
}

func TestGetPostgisCredentials_Defaults(t *testing.T) {
	t.Setenv("MEEP_PG_USER", "")
	t.Setenv("MEEP_PG_PASSWORD", "")

	user, pwd := getPostgisCredentials()
	if user != defaultPostgisUser || pwd != defaultPostgisPwd {
		t.Fatalf("expected defaults %q/%q, got %q/%q", defaultPostgisUser, defaultPostgisPwd, user, pwd)
	}
}

func TestGetPostgisCredentials_EnvOverride(t *testing.T) {
	t.Setenv("MEEP_PG_USER", "customuser")
	t.Setenv("MEEP_PG_PASSWORD", "custompwd")

	user, pwd := getPostgisCredentials()
	if user != "customuser" || pwd != "custompwd" {
		t.Fatalf("expected env override, got %q/%q", user, pwd)
	}
}

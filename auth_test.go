package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupAndLogin(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	os.MkdirAll(configDir, 0755)
	os.Setenv("TEST_DB_PATH", filepath.Join(configDir, "auth_test.db"))
	defer os.Unsetenv("TEST_DB_PATH")

	cfg, err := createDefaultConfig()
	if err != nil {
		t.Fatalf("createDefaultConfig: %v", err)
	}
	cfg.BooksDir = tmpDir
	saveConfigInternal(cfg)

	dm, err := NewDBManager(cfg)
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}
	defer dm.Close()

	sm := &SystemManager{DB: dm, Config: cfg}
	sk := cfg.JWTSigningKey
	if len(sk) == 0 {
		t.Fatal("JWT key empty")
	}

	w := httptest.NewRecorder()
	handleSetupStatus(sm)(w, httptest.NewRequest("GET", "/api/setup-status", nil))
	if w.Code != 200 {
		t.Fatalf("setup-status: code %d", w.Code)
	}
	var st struct {
		SetupRequired bool `json:"setup_required"`
	}
	json.NewDecoder(w.Body).Decode(&st)
	if !st.SetupRequired {
		t.Fatal("expected setup_required true")
	}

	body, _ := json.Marshal(AuthRequest{Username: "Admin", Password: "secret123"})
	w = httptest.NewRecorder()
	handleSetup(sm, sk)(w, httptest.NewRequest("POST", "/api/setup", bytes.NewReader(body)))
	if w.Code != 200 {
		t.Fatalf("setup: code %d body %s", w.Code, w.Body.String())
	}
	var resp struct {
		Token string   `json:"token"`
		User  AuthUser `json:"user"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.User.Username != "Admin" || resp.User.Role != "admin" {
		t.Fatalf("unexpected user: %+v", resp.User)
	}

	w = httptest.NewRecorder()
	handleLogin(sm, sk)(w, httptest.NewRequest("POST", "/api/login", bytes.NewReader(body)))
	if w.Code != 200 {
		t.Fatalf("login: code %d", w.Code)
	}
}

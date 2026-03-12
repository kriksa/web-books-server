package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	os.MkdirAll(configDir, 0755)
	os.Setenv("TEST_DB_PATH", filepath.Join(configDir, "test.db"))
	defer os.Unsetenv("TEST_DB_PATH")

	cfg := &Config{BooksDir: tmpDir}
	dm, err := NewDBManager(cfg)
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}
	defer dm.Close()

	count, err := dm.UserCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 users, got %d", count)
	}

	err = dm.CreateUser("testuser", "hashedpassword", "user")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	count, _ = dm.UserCount()
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	id, hash, role, err := dm.GetUserByUsername("testuser")
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 || hash != "hashedpassword" || role != "user" {
		t.Fatalf("GetUserByUsername: id=%d hash=%s role=%s", id, hash, role)
	}

	err = dm.UpdateUserPassword(1, "newhash")
	if err != nil {
		t.Fatal(err)
	}
	_, hash, _, _ = dm.GetUserByID(1)
	if hash != "newhash" {
		t.Fatalf("password not updated: %s", hash)
	}

	err = dm.UpdateUsername(1, "newname")
	if err != nil {
		t.Fatal(err)
	}
	username, _, _, _ := dm.GetUserByID(1)
	if username != "newname" {
		t.Fatalf("username not updated: %s", username)
	}

	// Дубликат имени
	err = dm.CreateUser("newname", "x", "user")
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}

	deleted, err := dm.DeleteUser(1)
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if !deleted {
		t.Fatal("expected user to be deleted")
	}

	count, _ = dm.UserCount()
	if count != 0 {
		t.Fatalf("expected 0 users after delete, got %d", count)
	}
}

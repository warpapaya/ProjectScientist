package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestOperatorAuditVerifyAndDatabaseStatusCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	store, err := lab.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := store.CreateClient("Ops Lab", "ops@example.test", cliActor("operator", lab.RoleAdmin)); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	var out bytes.Buffer
	if err := run([]string{"project-scientist", "audit", "verify", "--db", dbPath}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("audit verify command: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "audit verify ok") {
		t.Fatalf("audit verify output = %q", got)
	}

	out.Reset()
	if err := run([]string{"project-scientist", "db", "status", "--db", dbPath}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("db status command: %v", err)
	}
	got := out.String()
	for _, want := range []string{"schema_version=9", "clients=1", "audit_events=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("db status output %q missing %q", got, want)
		}
	}
}

func TestOperatorSeedResetBackupRestoreCommands(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "project-scientist.db")
	backupPath := filepath.Join(dir, "backup.db")

	var out bytes.Buffer
	if err := run([]string{"project-scientist", "db", "migrate", "--db", dbPath}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("db migrate command: %v", err)
	}
	if err := run([]string{"project-scientist", "seed", "--db", dbPath}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("seed command: %v", err)
	}
	if err := run([]string{"project-scientist", "backup", "--db", dbPath, "--out", backupPath}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("backup command: %v", err)
	}
	if err := run([]string{"project-scientist", "reset", "--db", dbPath, "--force"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("reset command: %v", err)
	}
	out.Reset()
	if err := run([]string{"project-scientist", "db", "status", "--db", dbPath}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("db status after reset: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "clients=0") || !strings.Contains(got, "samples=0") {
		t.Fatalf("reset did not clear local db, status=%q", got)
	}
	if err := run([]string{"project-scientist", "restore", "--backup", backupPath, "--db", dbPath, "--force"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("restore command: %v", err)
	}
	out.Reset()
	if err := run([]string{"project-scientist", "db", "status", "--db", dbPath}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("db status after restore: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "clients=1") || !strings.Contains(got, "samples=1") {
		t.Fatalf("restore did not recover backup data, status=%q", got)
	}
}

func TestOperatorSmokeCommandChecksHealthAndState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte("ok"))
		case "/api/state":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":[],"samples":[],"audit":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"project-scientist", "smoke", "--base-url", server.URL}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("smoke command: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "smoke ok") {
		t.Fatalf("smoke output = %q", got)
	}
}

package main

import (
	"bytes"
	"html/template"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestProspectTrialSmokeCommandCoversLoginSeedAndRoutes(t *testing.T) {
	t.Setenv("PSC_INTERNAL_SESSION_TOKEN", "test-prospect-trial-token")
	t.Setenv("PSC_INTERNAL_SESSION_PASSWORD", "dev-password")
	t.Setenv("PSC_INTERNAL_SESSION_TTL", time.Hour.String())

	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := &app{
		store:            store,
		tmpl:             template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html"))),
		demoResetEnabled: true,
		fixturePath:      filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json"),
		sessions:         configuredInternalSessions(),
		now:              time.Now,
	}
	server := httptest.NewServer(application.routes())
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err = run([]string{"project-scientist", "smoke", "prospect-trial", "--base-url", server.URL, "--password", "dev-password"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("prospect trial smoke failed: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	out := stdout.String()
	for _, want := range []string{"prospect trial smoke ok", "routes=login,dashboard,samples,results,reports", "seeded_sample=S-000001"} {
		if !strings.Contains(out, want) {
			t.Fatalf("prospect smoke output missing %q: %s", want, out)
		}
	}
}

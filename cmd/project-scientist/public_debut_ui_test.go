package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestPublicDebutLoginCopyIsProspectSafe(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := &app{store: store, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))}

	rec := httptest.NewRecorder()
	application.loginPage(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	assertContainsAll(t, body, []string{
		"Sign in to the Project Scientist demo workspace",
		"Explore a synthetic lab workflow from sample receipt to report preview.",
	})
	assertContainsNone(t, body, []string{
		"manually injecting cookies",
		"PSC_INTERNAL_SESSION_USER",
		"PSC_INTERNAL_SESSION_PASSWORD",
	})
}

func TestPublicDebutSeededRoutesHideInternalCopyAndRawEnums(t *testing.T) {
	store := seedGuidedDemoStore(t)
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	checks := []struct {
		path string
		want []string
		ban  []string
	}{
		{
			path: "/dashboard",
			want: []string{"Synthetic lab workflow demo", "Demo workspace • synthetic data • audit trail enabled", "One sample, end to end", "1 of 4 workflow milestones complete", "Enter demo results"},
			ban:  []string{"Local dev", "lab-test/default-lab", "control wall", "backend", "authoritative", "from domain state"},
		},
		{
			path: "/samples",
			want: []string{"Current demo sample", "Status: Received", "Start preparation"},
			ban:  []string{"Move to in_prep", "Local dev"},
		},
		{
			path: "/results?sample_id=S-000001",
			want: []string{"Current demo work", "Enter results for S-000001", "0/4 results entered", "0/4 accepted", "No entered results are ready for review. Enter demo results first."},
			ban:  []string{"backend result APIs remain authoritative", "Reviewer separation remains enforced by the backend"},
		},
		{
			path: "/reports?sample_id=S-000001",
			want: []string{"Report readiness desk", "Report blocked", "Results 0/4 accepted", "Current status is Received", "Enter and accept the four requested results", "Release locked", "Complete the blockers above to release this report."},
			ban:  []string{"Results 0/0 accepted", "Blocked • blocked", "sample_status", "no_results", "no_current_report", "Release report", "Backend"},
		},
	}

	for _, check := range checks {
		body := renderIndexForPath(t, application, check.path)
		assertContainsAll(t, body, check.want)
		assertContainsNone(t, body, check.ban)
	}
}

func assertContainsNone(t *testing.T, body string, banned []string) {
	t.Helper()
	for _, ban := range banned {
		if strings.Contains(body, ban) {
			t.Fatalf("body unexpectedly contains %q\n%s", ban, body)
		}
	}
}

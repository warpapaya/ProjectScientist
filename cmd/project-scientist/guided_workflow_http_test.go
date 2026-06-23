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

func TestDashboardGuidedWorkflowRailShowsDemoResetEmptyState(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	body := renderIndexForPath(t, application, "/dashboard")
	assertContainsAll(t, body, []string{
		`data-testid="guided-workflow-rail"`,
		`One sample, end to end`,
		`Use the guided path to see how a lab moves a synthetic sample`,
		`data-testid="workflow-progress"`,
		`No synthetic demo sample loaded yet.`,
		`data-testid="demo-empty-state"`,
		`Load synthetic demo`,
	})
}

func TestDashboardGuidedWorkflowRailReflectsSeededSample(t *testing.T) {
	store := seedGuidedDemoStore(t)
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	body := renderIndexForPath(t, application, "/dashboard")
	assertContainsAll(t, body, []string{
		`data-testid="guided-workflow-rail"`,
		`data-testid="workflow-step-receive"`,
		`data-testid="workflow-step-results"`,
		`data-testid="workflow-step-review"`,
		`data-testid="workflow-step-report"`,
		`data-testid="workflow-progress"`,
		`S-000001`,
		`1 of 4 workflow milestones complete`,
		`href="/samples"`,
		`href="/results?sample_id=S-000001"`,
		`href="/results?sample_id=S-000001#result-review"`,
		`href="/reports?sample_id=S-000001"`,
	})
}

func TestSamplesGuidedPanelShowsSampleNextAction(t *testing.T) {
	store := seedGuidedDemoStore(t)
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	body := renderIndexForPath(t, application, "/samples")
	assertContainsAll(t, body, []string{
		`data-testid="guided-sample-panel"`,
		`S-000001`,
		`Okefenokee Synthetic Water Authority`,
		`4 analyses requested`,
		`data-testid="sample-open-results"`,
		`Enter results for S-000001`,
		`data-testid="sample-print-label"`,
		`data-testid="sample-next-transition"`,
	})
}

func TestResultsGuidedWorkItemShowsEntryAndReviewProgress(t *testing.T) {
	store := seedGuidedDemoStore(t)
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	body := renderIndexForPath(t, application, "/results?sample_id=S-000001")
	assertContainsAll(t, body, []string{
		`data-testid="result-work-item"`,
		`Enter results for S-000001`,
		`4 requested analyses are ready for entry`,
		`data-testid="result-entry-grid"`,
		`data-testid="result-row-ARL-000001"`,
		`data-testid="save-result-ARL-000001"`,
		`data-testid="results-complete-indicator"`,
		`0/4 results entered`,
		`data-testid="guided-review-panel"`,
		`data-testid="review-empty-state"`,
		`No entered results are ready for review.`,
	})
}

func TestReportsGuidedCardShowsReleaseReadiness(t *testing.T) {
	store := seedGuidedDemoStore(t)
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	body := renderIndexForPath(t, application, "/reports?sample_id=S-000001")
	assertContainsAll(t, body, []string{
		`data-testid="report-release-desk"`,
		`data-testid="report-card-S-000001"`,
		`data-testid="preview-report-S-000001"`,
		`Preview COA`,
		`data-testid="release-report-S-000001"`,
		`data-testid="release-blockers-S-000001"`,
		`data-testid="release-readiness-S-000001"`,
		`Results 0/4 accepted`,
		`No current report`,
	})
}

func seedGuidedDemoStore(t *testing.T) *lab.Store {
	t.Helper()
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "lab-dev", RequestID: "guided-test", CorrelationID: "guided-test", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleAdmin), string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer)}}}, Roles: []string{string(lab.RoleAdmin), string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer)}})
	if _, err := store.ResetAndSeedSyntheticDemo(filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json"), actor); err != nil {
		store.Close()
		t.Fatalf("seed demo: %v", err)
	}
	return store
}

func renderIndexForPath(t *testing.T, application *app, path string) string {
	t.Helper()
	req := newDefaultSessionRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	application.index(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func assertContainsAll(t *testing.T, body string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q\n%s", want, body)
		}
	}
}

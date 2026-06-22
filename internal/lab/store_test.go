package lab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSampleAssignsIDWorkflowAndAuditEvent(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "state.json"), filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	client, err := store.CreateClient("Clearline Demo Lab", "qa@example.test", "friday")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	sample, err := store.CreateSample(CreateSampleInput{
		ClientID: client.ID,
		Project:  "Drinking Water Compliance",
		Matrix:   "Water",
		Tests:    []string{"pH", "Turbidity"},
	}, "friday")
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if sample.ID == "" || !strings.HasPrefix(sample.ID, "S-") {
		t.Fatalf("expected generated S-* sample id, got %q", sample.ID)
	}
	if sample.Status != StatusReceived {
		t.Fatalf("expected status %q, got %q", StatusReceived, sample.Status)
	}
	if len(sample.Analyses) != 2 {
		t.Fatalf("expected 2 analyses, got %d", len(sample.Analyses))
	}

	events := readAuditEvents(t, filepath.Join(dir, "audit.jsonl"))
	if got := events[len(events)-1].Action; got != "sample.created" {
		t.Fatalf("expected last audit event sample.created, got %q", got)
	}
	if events[len(events)-1].Actor != "friday" {
		t.Fatalf("expected actor friday, got %q", events[len(events)-1].Actor)
	}
}

func TestWorkflowTransitionRequiresAllowedPathAndAudits(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "state.json"), filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	client, _ := store.CreateClient("CENLA Demo", "cenla@example.test", "ashley")
	sample, _ := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Metals", Matrix: "Soil", Tests: []string{"Lead"}}, "ashley")

	if err := store.TransitionSample(sample.ID, StatusReleased, "ashley"); err == nil {
		t.Fatalf("expected direct received -> released transition to fail")
	}
	for _, status := range []SampleStatus{StatusInPrep, StatusInAnalysis, StatusInReview, StatusReleased} {
		if err := store.TransitionSample(sample.ID, status, "ashley"); err != nil {
			t.Fatalf("transition to %s: %v", status, err)
		}
	}
	updated, ok := store.GetSample(sample.ID)
	if !ok || updated.Status != StatusReleased {
		t.Fatalf("expected released sample, got %#v", updated)
	}

	events := readAuditEvents(t, filepath.Join(dir, "audit.jsonl"))
	var transitions int
	for _, event := range events {
		if event.Action == "sample.transitioned" {
			transitions++
		}
	}
	if transitions != 4 {
		t.Fatalf("expected 4 transition audit events, got %d", transitions)
	}
}

func TestAuditLogIsHashChained(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "state.json"), filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	_, _ = store.CreateClient("Tindall Demo", "armando@example.test", "armando")
	_, _ = store.CreateClient("RJ Lee Demo", "demo@example.test", "friday")

	events := readAuditEvents(t, filepath.Join(dir, "audit.jsonl"))
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events")
	}
	for i := 1; i < len(events); i++ {
		if events[i].PreviousHash != events[i-1].Hash {
			t.Fatalf("event %d previous hash mismatch: got %q want %q", i, events[i].PreviousHash, events[i-1].Hash)
		}
		if events[i].Hash == "" {
			t.Fatalf("event %d missing hash", i)
		}
	}
}

func readAuditEvents(t *testing.T, path string) []AuditEvent {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	var events []AuditEvent
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var event AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode audit event %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

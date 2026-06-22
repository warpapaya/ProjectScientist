package main

import (
	"os"
	"strings"
	"testing"
)

func TestResultEntryGridWorkflowSpecIsImplementationReady(t *testing.T) {
	raw, err := os.ReadFile("../../docs/result-entry-grid-workflow.md")
	if err != nil {
		t.Fatalf("read result entry grid workflow spec: %v", err)
	}
	content := string(raw)
	for _, required := range []string{
		"Status: lab-test implementation-ready UX/backend contract",
		"POST /api/worksheets/{id}/results/draft",
		"POST /api/worksheets/{id}/results/complete",
		"GET /api/worksheets/{id}/result-entry",
		"Enter commits the current cell and advances to the next required value cell",
		"Shift+Enter moves to the previous editable result cell",
		"dirty",
		"saving",
		"saved",
		"invalid",
		"comments",
		"aria-live",
		"No spreadsheet clone",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("result entry spec missing %q", required)
		}
	}
}

func TestRoadmapLinksResultEntryGridWorkflowSpec(t *testing.T) {
	raw, err := os.ReadFile("../../docs/senaite-parity-roadmap.md")
	if err != nil {
		t.Fatalf("read roadmap: %v", err)
	}
	if !strings.Contains(string(raw), "docs/result-entry-grid-workflow.md") {
		t.Fatal("roadmap must link result entry grid workflow spec")
	}
}

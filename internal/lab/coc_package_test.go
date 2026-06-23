package lab

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCOCPackageGenerationCapturesCustodyAttachmentsHashAndAudit(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	actor := testActorWithRoles("package-releaser", RoleLabManager, RoleReportReleaser)
	client, _ := store.CreateClient("COC Package Client", "coc-package@example.test", actor)
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "PSC-RM-062", ClientSampleID: "FIELD-1", LabSampleID: "LAB-1", Matrix: "Water", Tests: []string{"pH"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if _, err := store.RecordCustodyEvent(CustodyEventInput{SampleID: sample.ID, Type: CustodyReceived, Location: "Receiving", Reason: "COC intake", OccurredAt: time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)}, actor); err != nil {
		t.Fatalf("record received custody: %v", err)
	}
	if _, err := store.RecordCustodyEvent(CustodyEventInput{SampleID: sample.ID, Type: CustodyTransferred, Location: "Prep", Reason: "prep handoff", OccurredAt: time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)}, actor); err != nil {
		t.Fatalf("record transfer custody: %v", err)
	}

	pkg, err := store.GenerateCOCPackage(COCPackageInput{
		SampleID:      sample.ID,
		PackageFormat: "application/vnd.project-scientist.coc+json",
		Attachments: []ReportPackageAttachmentInput{
			{Name: "custody-form.pdf", MediaType: "application/pdf", Content: []byte("synthetic custody form")},
			{Name: "field-notes.txt", MediaType: "text/plain", Content: []byte("synthetic field notes")},
		},
	}, actor)
	if err != nil {
		t.Fatalf("generate COC package: %v", err)
	}
	if pkg.ID == "" || pkg.SampleID != sample.ID || pkg.PackageFormat != "application/vnd.project-scientist.coc+json" || !strings.HasPrefix(pkg.ContentHash, "sha256:") {
		t.Fatalf("package identity/hash mismatch: %#v", pkg)
	}
	if len(pkg.CustodyEvents) != 2 || pkg.CustodyEvents[0].Type != CustodyReceived || pkg.CustodyEvents[1].Type != CustodyTransferred {
		t.Fatalf("package should freeze ordered custody history: %#v", pkg.CustodyEvents)
	}
	if len(pkg.Attachments) != 2 || pkg.Attachments[0].Name != "custody-form.pdf" || !strings.HasPrefix(pkg.Attachments[0].ContentHash, "sha256:") {
		t.Fatalf("package should preserve attachment metadata/hashes: %#v", pkg.Attachments)
	}
	if !strings.Contains(string(pkg.Content), "FIELD-1") || !strings.Contains(string(pkg.Content), "custody-form.pdf") {
		t.Fatalf("package content should include deterministic sample/custody/attachment manifest: %s", pkg.Content)
	}

	refetched, ok := store.COCPackage(pkg.ID)
	if !ok {
		t.Fatalf("refetch COC package")
	}
	if refetched.ContentHash != pkg.ContentHash || string(refetched.Content) != string(pkg.Content) || len(refetched.Attachments) != 2 {
		t.Fatalf("refetched package mismatch: got %#v want %#v", refetched, pkg)
	}
	if _, err := store.DB().Exec(`UPDATE coc_packages SET content_hash = 'mutated' WHERE id = ?`, pkg.ID); err == nil || !strings.Contains(err.Error(), "COC package is immutable") {
		t.Fatalf("expected package update to be rejected as immutable, got %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if !auditContains(events, "coc.package.generated", pkg.ID) {
		t.Fatalf("COC package generation was not audited: %#v", events)
	}
}

func TestCOCPackageHashIsStableForEquivalentInputs(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	actor := testActorWithRoles("package-releaser", RoleLabManager, RoleReportReleaser)
	client, _ := store.CreateClient("COC Stable Client", "coc-stable@example.test", actor)
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "PSC-RM-062", ClientSampleID: "FIELD-STABLE", LabSampleID: "LAB-STABLE", Matrix: "Soil", Tests: []string{"TSS"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if _, err := store.RecordCustodyEvent(CustodyEventInput{SampleID: sample.ID, Type: CustodyReceived, Location: "Receiving", Reason: "COC intake", OccurredAt: time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)}, actor); err != nil {
		t.Fatalf("record custody: %v", err)
	}

	inputA := COCPackageInput{SampleID: sample.ID, PackageFormat: "application/vnd.project-scientist.coc+json", Attachments: []ReportPackageAttachmentInput{{Name: "b.txt", MediaType: "text/plain", Content: []byte("bravo")}, {Name: "a.txt", MediaType: "text/plain", Content: []byte("alpha")}}}
	inputB := COCPackageInput{SampleID: sample.ID, PackageFormat: "application/vnd.project-scientist.coc+json", Attachments: []ReportPackageAttachmentInput{{Name: "a.txt", MediaType: "text/plain", Content: []byte("alpha")}, {Name: "b.txt", MediaType: "text/plain", Content: []byte("bravo")}}}

	left, err := buildCOCPackagePayload(sample, mustCustodyEvents(t, store, sample.ID), normalizeCOCPackageInput(inputA), time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build left payload: %v", err)
	}
	right, err := buildCOCPackagePayload(sample, mustCustodyEvents(t, store, sample.ID), normalizeCOCPackageInput(inputB), time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build right payload: %v", err)
	}
	if left.ContentHash != right.ContentHash || string(left.CanonicalJSON) != string(right.CanonicalJSON) {
		t.Fatalf("equivalent attachment sets should hash canonically\nleft=%s\nright=%s", left.CanonicalJSON, right.CanonicalJSON)
	}
}

func mustCustodyEvents(t *testing.T, store *Store, sampleID string) []CustodyEvent {
	t.Helper()
	sample, ok := store.GetSample(sampleID)
	if !ok {
		t.Fatalf("load sample %s", sampleID)
	}
	return sample.CustodyEvents
}

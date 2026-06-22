package lab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSampleLabelArtifactIsDeterministicPrintableAndAudited(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := testActor("label-accessioner")
	client, _ := store.CreateClient("Label Client", "labels@example.test", actor)
	containerRef, _ := store.CreateSampleReferenceItem(SampleReferenceItemInput{Kind: SampleReferenceContainer, Code: "HDPE", Name: "HDPE Bottle", Active: true}, actor)
	sample, err := store.CreateSample(CreateSampleInput{
		ClientID:       client.ID,
		Project:        "Label Project",
		ClientSampleID: "field alpha/01",
		LabSampleID:    "PS-2026-0001",
		Matrix:         "Water",
		Tests:          []string{"Lead"},
		Containers: []SampleContainerInput{{
			ContainerReferenceID: containerRef.ID,
			Volume:               "500 mL",
			Condition:            "intact seal",
		}},
	}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}

	artifact, err := store.GenerateSampleLabelArtifact(sample.ID, actor)
	if err != nil {
		t.Fatalf("generate label artifact: %v", err)
	}
	again, err := store.GenerateSampleLabelArtifact(sample.ID, actor)
	if err != nil {
		t.Fatalf("generate second label artifact: %v", err)
	}
	if artifact.ContentHash != again.ContentHash || artifact.PrintableText != again.PrintableText {
		t.Fatalf("label artifact must be deterministic across repeated generation\nfirst=%#v\nagain=%#v", artifact, again)
	}
	if artifact.SampleID != sample.ID || artifact.ArtifactID != "LBL-S-000001" || artifact.Format != "text/psc-label-v1" {
		t.Fatalf("unexpected artifact identity: %#v", artifact)
	}
	if !strings.HasPrefix(artifact.ContentHash, "sha256:") || len(artifact.ContentHash) != len("sha256:")+64 {
		t.Fatalf("content hash should be sha256-prefixed hex, got %q", artifact.ContentHash)
	}
	want := readFixture(t, filepath.Join("testdata", "sample_label_artifact.txt"))
	if artifact.PrintableText != want {
		t.Fatalf("printable label fixture mismatch\nwant:\n%s\ngot:\n%s", want, artifact.PrintableText)
	}
	if artifact.BarcodeValue != "PS-2026-0001" || artifact.QRPayload != "PSC:lab-test:default-lab:S-000001:PS-2026-0001" {
		t.Fatalf("unexpected scan payloads: barcode=%q qr=%q", artifact.BarcodeValue, artifact.QRPayload)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	last := events[len(events)-1]
	if last.Action != "sample.label_artifact.generated" || last.Resource.Type != "label_artifact" || last.Resource.ID != artifact.ArtifactID {
		t.Fatalf("missing label artifact audit event: %#v", last)
	}
	if last.Details["content_hash"] != artifact.ContentHash || last.Details["barcode_value"] != artifact.BarcodeValue || last.Details["qr_payload"] != artifact.QRPayload {
		t.Fatalf("audit event missing artifact scan/hash details: %#v", last.Details)
	}
}

func TestSampleLabelArtifactRejectsUnsafeScanIdentifiers(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := testActor("label-safety")
	client, _ := store.CreateClient("Unsafe Label Client", "unsafe-labels@example.test", actor)
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Unsafe", ClientSampleID: "field-01", LabSampleID: "PS 2026/01", Matrix: "Water", Tests: []string{"Lead"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}

	if _, err := store.GenerateSampleLabelArtifact(sample.ID, actor); err == nil || !strings.Contains(err.Error(), "barcode-safe") {
		t.Fatalf("expected barcode-safe identifier rejection, got %v", err)
	}
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(bytes)
}

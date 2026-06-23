package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSampleOperationsDoNotLeakCrossTenantExistence(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	alpha := Scope{TenantID: "tenant-alpha", LabID: "water-lab"}
	beta := Scope{TenantID: "tenant-beta", LabID: "water-lab"}
	alphaActor := testScopedActor("alpha-accessioner", alpha.TenantID)
	betaActor := testScopedActor("beta-accessioner", beta.TenantID)
	client, err := store.CreateClientForScope(alpha, "Alpha Client", "alpha@example.test", alphaActor)
	if err != nil {
		t.Fatalf("create alpha client: %v", err)
	}
	containerRef, err := store.CreateSampleReferenceItemForScope(beta, SampleReferenceItemInput{Kind: SampleReferenceContainer, Code: "HDPE", Name: "HDPE Bottle", Active: true}, betaActor)
	if err != nil {
		t.Fatalf("create beta container ref: %v", err)
	}
	sample, err := store.CreateSampleForScope(alpha, CreateSampleInput{ClientID: client.ID, Project: "Alpha Project", Matrix: "Water", Tests: []string{"pH"}}, alphaActor)
	if err != nil {
		t.Fatalf("create alpha sample: %v", err)
	}

	cases := []struct {
		name      string
		action    string
		operation func() error
	}{
		{
			name:      "transition",
			action:    "sample.transition.requested",
			operation: func() error { return store.TransitionSampleForScope(beta, sample.ID, StatusInPrep, betaActor) },
		},
		{
			name:   "container add",
			action: "sample.container.add.requested",
			operation: func() error {
				_, err := store.AddSampleContainerForScope(beta, sample.ID, SampleContainerInput{ContainerReferenceID: containerRef.ID, Volume: "500 mL"}, betaActor)
				return err
			},
		},
		{
			name:   "label artifact",
			action: "sample.label_artifact.requested",
			operation: func() error {
				_, err := store.GenerateSampleLabelArtifactForScope(beta, sample.ID, betaActor)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.operation()
			if err == nil {
				t.Fatalf("expected cross-tenant %s denial", tc.name)
			}
			if !strings.Contains(err.Error(), "unknown sample") || strings.Contains(err.Error(), "outside requested tenant/lab scope") {
				t.Fatalf("expected opaque unknown-sample denial without existence leak, got %v", err)
			}

			events, err := store.AuditEventsForScope(beta, 0)
			if err != nil {
				t.Fatalf("beta audit events: %v", err)
			}
			if !auditContainsDeniedReason(events, tc.action, sample.ID, "sample_not_found") {
				t.Fatalf("expected denied %s audit event with sample_not_found, got %#v", tc.action, events)
			}
		})
	}
}

func auditContainsDeniedReason(events []AuditEvent, action, resourceID, reason string) bool {
	for _, event := range events {
		if event.Action == action && event.Outcome == AuditOutcomeDenied && event.Resource.ID == resourceID && event.Reason == reason {
			return true
		}
	}
	return false
}

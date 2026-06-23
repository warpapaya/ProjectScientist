package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoldenFixtureQCBatchesPersistChecksAndGateRelease(t *testing.T) {
	dataset := loadGoldenMigrationDataset(t)
	scenarios := []struct {
		familyID       string
		wantChecks     []string
		terminalStatus QCBatchStatus
		decisionReason string
		wantRelease    bool
	}{
		{
			familyID:       "precast-industrial",
			wantChecks:     []string{"method blank", "LCS recovery", "duplicate RPD"},
			terminalStatus: QCBatchStatusAccepted,
			decisionReason: "wet chemistry QC accepted for synthetic fixture",
			wantRelease:    true,
		},
		{
			familyID:       "municipal-water",
			wantChecks:     []string{"holding time warning", "microbiology placeholder", "EDD rows reconcile"},
			terminalStatus: QCBatchStatusRejected,
			decisionReason: "holding-time warning and microbiology placeholder require release denial",
			wantRelease:    false,
		},
		{
			familyID:       "materials-forensics",
			wantChecks:     []string{"reviewer acknowledgement", "subcontract flag", "attachment manifest"},
			terminalStatus: QCBatchStatusAccepted,
			decisionReason: "subcontract resolved and reviewer acknowledgement captured",
			wantRelease:    true,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.familyID, func(t *testing.T) {
			store, sample, batch := seedGoldenQCBatchReleaseFixture(t, dataset, scenario.familyID)
			defer store.Close()

			loaded, ok := store.QCBatchByID(batch.ID)
			if !ok {
				t.Fatalf("batch not found")
			}
			if len(loaded.Items) != 2 || len(loaded.Relationships) != 1 {
				t.Fatalf("golden QC batch must preserve linked sample/QC item relationship, got items=%d relationships=%d", len(loaded.Items), len(loaded.Relationships))
			}
			for _, want := range scenario.wantChecks {
				if !stringSliceContainsSubstring(loaded.Checks, want) {
					t.Fatalf("golden QC batch checks missing %q in %#v", want, loaded.Checks)
				}
			}

			manager := testActorWithRoles("release-manager", RoleLabManager)
			if err := advanceSampleToReview(store, sample.ID, manager); err != nil {
				t.Fatalf("advance sample to review: %v", err)
			}
			acceptAllResultsForSample(t, store, sample.ID)

			if err := store.TransitionSample(sample.ID, StatusReleased, manager); err == nil || !strings.Contains(err.Error(), "QC batch") {
				t.Fatalf("expected unaccepted golden QC batch to block release, got %v", err)
			}

			blockedProof, err := store.SampleQCReadinessProof(sample.ID, testActorWithRoles("qc-manager", RoleLabManager))
			if err != nil {
				t.Fatalf("read blocked QC proof: %v", err)
			}
			if blockedProof.ReleaseReady || len(blockedProof.Blockers) != 1 || blockedProof.ReconciliationHash == "" {
				t.Fatalf("blocked readiness proof not defensible: %#v", blockedProof)
			}
			if len(blockedProof.Blockers[0].Checks) == 0 {
				t.Fatalf("readiness blocker must include modeled QC checks: %#v", blockedProof.Blockers[0])
			}

			if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusInReview, "ready for golden QC decision", manager); err != nil {
				t.Fatalf("move QC batch to review: %v", err)
			}
			if _, err := store.TransitionQCBatch(batch.ID, scenario.terminalStatus, scenario.decisionReason, manager); err != nil {
				t.Fatalf("terminal QC decision: %v", err)
			}

			proof, err := store.SampleQCReadinessProof(sample.ID, testActorWithRoles("qc-manager", RoleLabManager))
			if err != nil {
				t.Fatalf("read terminal QC proof: %v", err)
			}
			if proof.ReleaseReady != scenario.wantRelease || proof.ReconciliationHash == "" {
				t.Fatalf("unexpected terminal readiness proof: %#v", proof)
			}
			err = store.TransitionSample(sample.ID, StatusReleased, manager)
			if scenario.wantRelease && err != nil {
				t.Fatalf("expected release after accepted golden QC, got %v", err)
			}
			if !scenario.wantRelease && (err == nil || !strings.Contains(err.Error(), "QC batch")) {
				t.Fatalf("expected rejected golden QC to keep blocking release, got %v", err)
			}
		})
	}
}

func TestSampleQCReadinessProofRequiresSafeAuthorizationAndAuditsDenial(t *testing.T) {
	dataset := loadGoldenMigrationDataset(t)
	store, sample, _ := seedGoldenQCBatchReleaseFixture(t, dataset, "municipal-water")
	defer store.Close()

	_, err := store.SampleQCReadinessProof(sample.ID, testActorWithRoles("client-contact-without-audit-view", RoleClientContact))
	if err == nil || !strings.Contains(err.Error(), "authorization denied") {
		t.Fatalf("expected safe denial for unauthorized QC readiness proof, got %v", err)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if !auditDeniedEventExists(events, string(OperationAuditView), "sample", sample.ID) {
		t.Fatalf("expected denied audit event for unauthorized readiness proof")
	}
}

func seedGoldenQCBatchReleaseFixture(t *testing.T, dataset goldenMigrationDataset, familyID string) (*Store, Sample, QCBatch) {
	t.Helper()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	actor := catalogTestActor()
	client, method, service := createQCClientMethodAndService(t, store, actor)
	goldenSample := goldenSampleForFamily(t, dataset, familyID)
	goldenBatch := goldenQCBatchForFamily(t, dataset, familyID)

	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: familyID + " golden QC", ClientSampleID: goldenSample.ClientSampleID, Matrix: goldenSample.Matrix, AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if _, err := store.CreateResult(ResultInput{AnalysisRequestLineID: store.AnalysisRequestLinesForSample(sample.ID)[0].ID, Value: 1.0, RawValue: "1.0", Unit: "synthetic", Dilution: 1, AnalystID: "analyst-1"}, testActorWithRoles("analyst-1", RoleAnalyst)); err != nil {
		t.Fatalf("create result: %v", err)
	}
	qcSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: familyID + " golden QC", ClientSampleID: goldenBatch.ID + "-CONTROL", Matrix: goldenSample.Matrix, AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create QC sample: %v", err)
	}
	batch, err := store.CreateQCBatch(CreateQCBatchInput{Name: goldenBatch.ID, MethodID: method.ID, Matrix: goldenSample.Matrix, Checks: goldenBatch.Checks}, actor)
	if err != nil {
		t.Fatalf("create QC batch: %v", err)
	}
	if _, err := store.AddQCItemToBatch(batch.ID, CreateQCItemInput{SampleID: sample.ID, Role: QCItemRoleClientSample}, actor); err != nil {
		t.Fatalf("add client QC item: %v", err)
	}
	qcItem, err := store.AddQCItemToBatch(batch.ID, CreateQCItemInput{SampleID: qcSample.ID, Role: QCItemRoleQCSample, QCSampleKind: QCSampleKindMethodBlank}, actor)
	if err != nil {
		t.Fatalf("add QC item: %v", err)
	}
	if _, err := store.CreateQCRelationship(CreateQCRelationshipInput{QCBatchID: batch.ID, QCItemID: qcItem.ID, RelationshipType: QCRelationshipTypeBatchControl, RelatedSampleID: sample.ID}, actor); err != nil {
		t.Fatalf("create QC relationship: %v", err)
	}
	return store, sample, batch
}

func goldenSampleForFamily(t *testing.T, dataset goldenMigrationDataset, familyID string) goldenSample {
	t.Helper()
	for _, sample := range dataset.Samples {
		if sample.FamilyID == familyID {
			return sample
		}
	}
	t.Fatalf("missing golden sample for family %s", familyID)
	return goldenSample{}
}

func goldenQCBatchForFamily(t *testing.T, dataset goldenMigrationDataset, familyID string) goldenQCBatch {
	t.Helper()
	for _, batch := range dataset.QCBatches {
		if batch.FamilyID == familyID {
			return batch
		}
	}
	t.Fatalf("missing golden QC batch for family %s", familyID)
	return goldenQCBatch{}
}

func acceptAllResultsForSample(t *testing.T, store *Store, sampleID string) {
	t.Helper()
	for _, result := range store.ResultsForScope(DefaultScope) {
		if result.SampleID != sampleID {
			continue
		}
		if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "accepted for golden QC release test", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-1", RoleReviewer)); err != nil {
			t.Fatalf("accept result %s: %v", result.ID, err)
		}
	}
}

func stringSliceContainsSubstring(values []string, want string) bool {
	want = strings.ToLower(want)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), want) {
			return true
		}
	}
	return false
}

func auditDeniedEventExists(events []AuditEvent, action, resourceType, resourceID string) bool {
	for _, event := range events {
		if event.Action == action && event.Resource.Type == resourceType && event.Resource.ID == resourceID && event.Outcome == AuditOutcomeDenied {
			return true
		}
	}
	return false
}

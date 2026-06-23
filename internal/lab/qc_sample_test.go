package lab

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestQCSampleTaxonomyDefinesRequiredSENAITEParityKinds(t *testing.T) {
	kinds := QCSampleTaxonomy()
	want := []QCSampleKind{
		QCSampleKindMethodBlank,
		QCSampleKindTripBlank,
		QCSampleKindEquipmentBlank,
		QCSampleKindFieldDuplicate,
		QCSampleKindLabDuplicate,
		QCSampleKindMatrixSpike,
		QCSampleKindMatrixSpikeDuplicate,
		QCSampleKindLaboratoryControlSample,
		QCSampleKindControlSample,
		QCSampleKindInitialCalibrationVerification,
		QCSampleKindContinuingCalibrationVerification,
	}
	if len(kinds) != len(want) {
		t.Fatalf("taxonomy count got %d want %d: %#v", len(kinds), len(want), kinds)
	}
	for _, kind := range want {
		def, ok := QCDefinitionForKind(kind)
		if !ok {
			t.Fatalf("missing QC definition for %q", kind)
		}
		if def.Kind != kind || strings.TrimSpace(def.Label) == "" || strings.TrimSpace(def.Purpose) == "" {
			t.Fatalf("definition for %q is incomplete: %#v", kind, def)
		}
		if def.RelationshipRequired == RelationshipRequired && len(def.AllowedRelationshipTypes) == 0 {
			t.Fatalf("%q requires a relationship but exposes no allowed relationship types", kind)
		}
	}
}

func TestCreateQCSampleRelationshipPersistsSampleMethodLineAndTaxonomy(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	client, method, service := createQCClientMethodAndService(t, store, actor)
	clientSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "C-1", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create client sample: %v", err)
	}
	qcSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "MB-1", Matrix: "Reagent water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create qc sample: %v", err)
	}
	clientLines := store.AnalysisRequestLinesForSample(clientSample.ID)
	if len(clientLines) != 1 {
		t.Fatalf("expected client sample line, got %d", len(clientLines))
	}

	rel, err := store.CreateQCSampleRelationship(CreateQCSampleRelationshipInput{
		QCSampleID:          qcSample.ID,
		QCSampleKind:        QCSampleKindMethodBlank,
		RelationshipType:    QCRelationshipTypeBatchControl,
		RelatedSampleID:     clientSample.ID,
		MethodID:            method.ID,
		AnalysisRequestLine: clientLines[0].ID,
		BatchID:             "BATCH-2026-001",
		Notes:               "method blank for synthetic metals batch",
	}, actor)
	if err != nil {
		t.Fatalf("create QC relationship: %v", err)
	}
	if rel.QCSampleID != qcSample.ID || rel.RelatedSampleID != clientSample.ID || rel.MethodID != method.ID || rel.AnalysisRequestLine != clientLines[0].ID {
		t.Fatalf("relationship did not preserve identity links: %#v", rel)
	}
	if rel.QCSampleKind != QCSampleKindMethodBlank || rel.RelationshipType != QCRelationshipTypeBatchControl || rel.BatchID != "BATCH-2026-001" {
		t.Fatalf("relationship taxonomy fields wrong: %#v", rel)
	}

	forSample := store.QCSampleRelationshipsForSample(clientSample.ID)
	if len(forSample) != 1 || forSample[0].ID != rel.ID {
		t.Fatalf("expected relationship by related sample, got %#v", forSample)
	}
	forQC := store.QCSampleRelationshipsForQCSample(qcSample.ID)
	if len(forQC) != 1 || forQC[0].ID != rel.ID {
		t.Fatalf("expected relationship by QC sample, got %#v", forQC)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	var audited bool
	for _, event := range events {
		if event.Action == "qc_sample.relationship.created" && event.Resource.Type == "qc_sample_relationship" && event.Resource.ID == rel.ID && event.Outcome == AuditOutcomeAllowed {
			audited = true
		}
	}
	if !audited {
		t.Fatalf("expected allowed QC relationship audit event")
	}
}

func TestQCSampleTaxonomyReturnsDefensiveRelationshipTypeCopies(t *testing.T) {
	taxonomy := QCSampleTaxonomy()
	if len(taxonomy) == 0 || len(taxonomy[0].AllowedRelationshipTypes) == 0 {
		t.Fatalf("expected taxonomy relationship types")
	}
	taxonomy[0].AllowedRelationshipTypes[0] = QCRelationshipType("mutated")
	fresh := QCSampleTaxonomy()
	if fresh[0].AllowedRelationshipTypes[0] == QCRelationshipType("mutated") {
		t.Fatalf("QCSampleTaxonomy exposed mutable relationship type slice")
	}

	def, ok := QCDefinitionForKind(QCSampleKindMethodBlank)
	if !ok || len(def.AllowedRelationshipTypes) == 0 {
		t.Fatalf("expected method blank definition")
	}
	def.AllowedRelationshipTypes[0] = QCRelationshipType("mutated")
	freshDef, ok := QCDefinitionForKind(QCSampleKindMethodBlank)
	if !ok || freshDef.AllowedRelationshipTypes[0] == QCRelationshipType("mutated") {
		t.Fatalf("QCDefinitionForKind exposed mutable relationship type slice")
	}
}

func TestCreateQCSampleRelationshipRejectsInvalidTargetShapes(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	client, method, service := createQCClientMethodAndService(t, store, actor)
	qcSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "QC-1", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create qc sample: %v", err)
	}

	cases := []struct {
		name    string
		input   CreateQCSampleRelationshipInput
		wantErr string
	}{
		{
			name:    "duplicate rejects method only",
			input:   CreateQCSampleRelationshipInput{QCSampleID: qcSample.ID, QCSampleKind: QCSampleKindFieldDuplicate, RelationshipType: QCRelationshipTypeDuplicateOf, MethodID: method.ID},
			wantErr: "requires related sample or analysis request line",
		},
		{
			name:    "spike rejects method only",
			input:   CreateQCSampleRelationshipInput{QCSampleID: qcSample.ID, QCSampleKind: QCSampleKindMatrixSpike, RelationshipType: QCRelationshipTypeSpikeOf, MethodID: method.ID},
			wantErr: "requires related sample or analysis request line",
		},
		{
			name:    "calibration rejects batch only",
			input:   CreateQCSampleRelationshipInput{QCSampleID: qcSample.ID, QCSampleKind: QCSampleKindInitialCalibrationVerification, RelationshipType: QCRelationshipTypeCalibrationForMethod, BatchID: "BATCH-ONLY"},
			wantErr: "requires method or method-bearing analysis request line",
		},
		{
			name:    "method control rejects related sample only",
			input:   CreateQCSampleRelationshipInput{QCSampleID: qcSample.ID, QCSampleKind: QCSampleKindLaboratoryControlSample, RelationshipType: QCRelationshipTypeControlForMethod, RelatedSampleID: qcSample.ID},
			wantErr: "requires method or method-bearing analysis request line",
		},
		{
			name:    "batch control rejects sample only without batch",
			input:   CreateQCSampleRelationshipInput{QCSampleID: qcSample.ID, QCSampleKind: QCSampleKindMethodBlank, RelationshipType: QCRelationshipTypeBatchControl, RelatedSampleID: qcSample.ID},
			wantErr: "requires batch id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.CreateQCSampleRelationship(tc.input, actor)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q error, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestCreateQCSampleRelationshipAcceptsValidTargetShapes(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	client, method, service := createQCClientMethodAndService(t, store, actor)
	clientSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "C-2", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create client sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(clientSample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected client analysis line, got %d", len(lines))
	}

	cases := []struct {
		name  string
		kind  QCSampleKind
		rel   QCRelationshipType
		input CreateQCSampleRelationshipInput
	}{
		{name: "duplicate with source sample", kind: QCSampleKindFieldDuplicate, rel: QCRelationshipTypeDuplicateOf, input: CreateQCSampleRelationshipInput{RelatedSampleID: clientSample.ID}},
		{name: "spike with source line", kind: QCSampleKindMatrixSpike, rel: QCRelationshipTypeSpikeOf, input: CreateQCSampleRelationshipInput{AnalysisRequestLine: lines[0].ID}},
		{name: "calibration with method", kind: QCSampleKindInitialCalibrationVerification, rel: QCRelationshipTypeCalibrationForMethod, input: CreateQCSampleRelationshipInput{MethodID: method.ID}},
		{name: "method control with method-bearing line", kind: QCSampleKindLaboratoryControlSample, rel: QCRelationshipTypeControlForMethod, input: CreateQCSampleRelationshipInput{AnalysisRequestLine: lines[0].ID}},
		{name: "batch control with batch id", kind: QCSampleKindMethodBlank, rel: QCRelationshipTypeBatchControl, input: CreateQCSampleRelationshipInput{BatchID: "BATCH-2026-002"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			qcSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: tc.name, Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
			if err != nil {
				t.Fatalf("create qc sample: %v", err)
			}
			input := tc.input
			input.QCSampleID = qcSample.ID
			input.QCSampleKind = tc.kind
			input.RelationshipType = tc.rel
			if _, err := store.CreateQCSampleRelationship(input, actor); err != nil {
				t.Fatalf("expected valid relationship shape, got %v", err)
			}
		})
	}
}

func TestQCSampleRelationshipValidationScopeAndAuthorization(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	client, _, service := createQCClientMethodAndService(t, store, actor)
	clientSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create client sample: %v", err)
	}
	qcSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create qc sample: %v", err)
	}

	if _, err := store.CreateQCSampleRelationship(CreateQCSampleRelationshipInput{QCSampleID: qcSample.ID, QCSampleKind: QCSampleKindMatrixSpike, RelationshipType: QCRelationshipTypeBatchControl, RelatedSampleID: clientSample.ID}, actor); err == nil || !strings.Contains(err.Error(), "relationship type") {
		t.Fatalf("expected incompatible relationship type denial, got %v", err)
	}
	clientActor := actorWithRoles("client-contact", RoleClientContact)
	_, err = store.CreateQCSampleRelationship(CreateQCSampleRelationshipInput{QCSampleID: qcSample.ID, QCSampleKind: QCSampleKindMethodBlank, RelationshipType: QCRelationshipTypeBatchControl, RelatedSampleID: clientSample.ID}, clientActor)
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected authorization denial, got %v", err)
	}
	otherScope := Scope{TenantID: "other-tenant", LabID: DefaultLabID}
	otherActor := MustActorContext(ActorContextInput{UserID: "other-manager", DisplayName: "Other Manager", AuthProvider: "test", RequestID: "other", CorrelationID: "other", TenantMemberships: []TenantMembership{{TenantID: otherScope.TenantID, Roles: []string{string(RoleLabManager)}}}, Roles: []string{string(RoleLabManager)}})
	_, err = store.CreateQCSampleRelationshipForScope(otherScope, CreateQCSampleRelationshipInput{QCSampleID: qcSample.ID, QCSampleKind: QCSampleKindMethodBlank, RelationshipType: QCRelationshipTypeBatchControl, RelatedSampleID: clientSample.ID}, otherActor)
	if err == nil || !strings.Contains(err.Error(), "outside requested tenant/lab scope") {
		t.Fatalf("expected scope denial, got %v", err)
	}
}

func createQCClientMethodAndService(t *testing.T, store *Store, actor ActorContext) (Client, CatalogMethod, AnalysisService) {
	t.Helper()
	client, err := store.CreateClient("QC Client", "qc@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	dept, err := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Metals", SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create department: %v", err)
	}
	method, err := store.CreateCatalogMethod(CatalogMethodInput{Name: "EPA 200.8"}, actor)
	if err != nil {
		t.Fatalf("create method: %v", err)
	}
	service, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "Lead", DepartmentID: dept.ID, MethodID: method.ID, SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return client, method, service
}

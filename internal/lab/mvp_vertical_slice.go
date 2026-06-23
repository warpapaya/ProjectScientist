package lab

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type MVPVerticalSliceInput struct {
	ClientName      string
	ContactName     string
	ProjectName     string
	WorkOrder       string
	ClientSampleID  string
	LabSampleID     string
	AnalysisProfile string
	TemplateID      string
	TemplateVersion string
}

type MVPVerticalSliceSummary struct {
	Scope                Scope
	Client               Client
	Site                 Site
	Contact              Contact
	ContactRole          ContactRole
	Project              Project
	Sample               Sample
	Label                LabelArtifact
	AnalysisRequestLines []AnalysisRequestLine
	Worksheet            Worksheet
	Results              []Result
	QCBatch              QCBatch
	Report               ReleasedReportArtifact
	DeniedControls       []string
}

func (s *Store) RunMVPVerticalSlice(input MVPVerticalSliceInput, actor ActorContext) (MVPVerticalSliceSummary, error) {
	return s.RunMVPVerticalSliceForScope(defaultScope(), input, actor)
}

func (s *Store) RunMVPVerticalSliceForScope(scope Scope, input MVPVerticalSliceInput, actor ActorContext) (MVPVerticalSliceSummary, error) {
	scope, err := normalizeScope(scope)
	if err != nil {
		return MVPVerticalSliceSummary{}, err
	}
	input = normalizeMVPVerticalSliceInput(input)
	if err := Authorize(scope, OperationAdminConfigure, actor); err != nil {
		return MVPVerticalSliceSummary{}, err
	}

	summary := MVPVerticalSliceSummary{Scope: scope}

	client, err := s.CreateClientForScope(scope, input.ClientName, "mvp-synthetic@example.test", actor)
	if err != nil {
		return summary, err
	}
	summary.Client = client
	site, err := s.CreateSiteForScope(scope, SiteInput{ClientID: client.ID, Name: "MVP Synthetic Intake Site", Division: "Lab Test", Address: "100 Lab Test Way"}, actor)
	if err != nil {
		return summary, err
	}
	summary.Site = site
	contact, err := s.CreateContactForScope(scope, ContactInput{ClientID: client.ID, SiteID: site.ID, Name: input.ContactName, Email: "sam.submitter@example.test", Phone: "555-0100"}, actor)
	if err != nil {
		return summary, err
	}
	summary.Contact = contact
	role, err := s.AssignContactRoleForScope(scope, ContactRoleInput{ContactID: contact.ID, Role: "sample_submitter"}, actor)
	if err != nil {
		return summary, err
	}
	summary.ContactRole = role

	matrixRef, err := s.CreateSampleReferenceItemForScope(scope, SampleReferenceItemInput{Kind: SampleReferenceMatrix, Name: "Drinking Water", Code: "DW", Active: true, SortOrder: 1}, actor)
	if err != nil {
		return summary, err
	}
	containerRef, err := s.CreateSampleReferenceItemForScope(scope, SampleReferenceItemInput{Kind: SampleReferenceContainer, Name: "250 mL HDPE Bottle", Code: "HDPE250", Active: true, SortOrder: 1}, actor)
	if err != nil {
		return summary, err
	}
	preservativeRef, err := s.CreateSampleReferenceItemForScope(scope, SampleReferenceItemInput{Kind: SampleReferencePreservative, Name: "Nitric Acid", Code: "HNO3", Active: true, SortOrder: 1}, actor)
	if err != nil {
		return summary, err
	}
	conditionRef, err := s.CreateSampleReferenceItemForScope(scope, SampleReferenceItemInput{Kind: SampleReferenceReceivedCondition, Name: "Received intact on ice", Code: "ICE-OK", Active: true, SortOrder: 1}, actor)
	if err != nil {
		return summary, err
	}

	dept, err := s.CreateCatalogDepartmentForScope(scope, CatalogDepartmentInput{Name: "Wet Chemistry", SortOrder: 1}, actor)
	if err != nil {
		return summary, err
	}
	unit, err := s.CreateCatalogUnitForScope(scope, CatalogUnitInput{Name: "Milligrams per Liter", Symbol: "mg/L"}, actor)
	if err != nil {
		return summary, err
	}
	method, err := s.CreateCatalogMethodForScope(scope, CatalogMethodInput{Name: "EPA 200.8", Description: "Synthetic metals method for MVP vertical slice"}, actor)
	if err != nil {
		return summary, err
	}
	lead, err := s.CreateCatalogAnalyteForScope(scope, CatalogAnalyteInput{Name: "Lead", DefaultUnitID: unit.ID}, actor)
	if err != nil {
		return summary, err
	}
	copper, err := s.CreateCatalogAnalyteForScope(scope, CatalogAnalyteInput{Name: "Copper", DefaultUnitID: unit.ID}, actor)
	if err != nil {
		return summary, err
	}
	leadService, err := s.CreateAnalysisServiceForScope(scope, AnalysisServiceInput{Name: "Lead", DepartmentID: dept.ID, MethodID: method.ID, AnalyteID: lead.ID, UnitID: unit.ID, SortOrder: 1}, actor)
	if err != nil {
		return summary, err
	}
	copperService, err := s.CreateAnalysisServiceForScope(scope, AnalysisServiceInput{Name: "Copper", DepartmentID: dept.ID, MethodID: method.ID, AnalyteID: copper.ID, UnitID: unit.ID, SortOrder: 2}, actor)
	if err != nil {
		return summary, err
	}
	profile, err := s.CreateAnalysisProfileForScope(scope, AnalysisProfileInput{Name: input.AnalysisProfile, ServiceIDs: []string{leadService.ID, copperService.ID}}, actor)
	if err != nil {
		return summary, err
	}
	project, err := s.CreateProjectForScope(scope, ProjectInput{ClientID: client.ID, SiteID: site.ID, Name: input.ProjectName, WorkOrder: input.WorkOrder, DefaultMatrix: matrixRef.Name, DefaultTests: []string{leadService.Name, copperService.Name}}, actor)
	if err != nil {
		return summary, err
	}
	summary.Project = project
	if _, err := s.UpsertClientDefaultsForScope(scope, ClientDefaultsInput{ClientID: client.ID, ReportTemplate: input.TemplateID, InvoiceEmail: "billing@example.test", DefaultMatrix: matrixRef.Name, DefaultTests: []string{leadService.Name, copperService.Name}}, actor); err != nil {
		return summary, err
	}

	sampledAt := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	receivedAt := sampledAt.Add(2 * time.Hour)
	sample, err := s.CreateSampleForScope(scope, CreateSampleInput{
		ClientID:            client.ID,
		ProjectID:           project.ID,
		ClientSampleID:      input.ClientSampleID,
		LabSampleID:         input.LabSampleID,
		MatrixReferenceID:   matrixRef.ID,
		ContainerID:         containerRef.ID,
		PreservativeID:      preservativeRef.ID,
		ReceivedConditionID: conditionRef.ID,
		SampledAt:           sampledAt,
		ReceivedAt:          receivedAt,
		Priority:            PriorityRoutine,
		Comments:            "synthetic MVP vertical slice sample",
		AnalysisProfileIDs:  []string{profile.ID},
		Containers: []SampleContainerInput{{
			ContainerReferenceID: containerRef.ID,
			PreservativeID:       preservativeRef.ID,
			ReceivedConditionID:  conditionRef.ID,
			Volume:               "250 mL",
			Condition:            "intact, 4 C",
			AliquotInstructions:  "metals aliquot",
			Aliquots:             []SampleAliquotInput{{DepartmentID: dept.ID, MethodID: method.ID, Volume: "50 mL", Purpose: "analysis"}},
		}},
	}, actor)
	if err != nil {
		return summary, err
	}
	summary.Sample = sample

	if err := s.TransitionSampleForScope(scope, sample.ID, StatusInReview, actor); err != nil {
		summary.DeniedControls = append(summary.DeniedControls, "illegal_workflow_jump:"+err.Error())
	} else {
		return summary, errors.New("illegal workflow jump unexpectedly succeeded")
	}

	label, err := s.GenerateSampleLabelArtifactForScope(scope, sample.ID, actor)
	if err != nil {
		return summary, err
	}
	summary.Label = label
	lines := s.AnalysisRequestLinesForSampleForScope(scope, sample.ID)
	if len(lines) == 0 {
		return summary, errors.New("sample intake did not create analysis request lines")
	}
	summary.AnalysisRequestLines = lines

	if _, err := s.CreateResultForScope(scope, ResultInput{AnalysisRequestLineID: lines[0].ID, Value: 9.9, RawValue: "9.9 mg/L", Unit: "mg/L", Dilution: 1}, mvpActor(scope, "mvp-client-contact", RoleClientContact)); err != nil {
		summary.DeniedControls = append(summary.DeniedControls, "unauthorized_mutation:"+err.Error())
	} else {
		return summary, errors.New("unauthorized result mutation unexpectedly succeeded")
	}

	if err := s.TransitionSampleForScope(scope, sample.ID, StatusInPrep, actor); err != nil {
		return summary, err
	}
	if err := s.TransitionSampleForScope(scope, sample.ID, StatusInAnalysis, actor); err != nil {
		return summary, err
	}

	worksheet, err := s.CreateWorksheetForScope(scope, CreateWorksheetInput{AnalysisRequestLineIDs: analysisRequestLineIDs(lines), BatchID: "MVP-BATCH-001", AnalystID: "mvp-analyst"}, actor)
	if err != nil {
		return summary, err
	}
	if err := s.TransitionWorksheetForScope(scope, worksheet.ID, WorksheetStatusInProgress, actor); err != nil {
		return summary, err
	}

	results := make([]Result, 0, len(lines))
	for i, line := range lines {
		result, err := s.CreateResultForScope(scope, ResultInput{AnalysisRequestLineID: line.ID, Value: []float64{0.0042, 0.0105}[i%2], RawValue: []string{"0.0042 mg/L", "0.0105 mg/L"}[i%2], Unit: "mg/L", MDL: 0.001, RL: 0.005, Dilution: 1, AnalystID: "mvp-analyst", InstrumentID: "ICP-MS-MVP", Comments: "entered from MVP vertical slice"}, mvpActor(scope, "mvp-analyst", RoleAnalyst))
		if err != nil {
			return summary, err
		}
		reviewed, err := s.ReviewResultForScope(scope, result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "accepted for MVP release", EnforceReviewerSeparation: true}, mvpActor(scope, "mvp-reviewer", RoleReviewer))
		if err != nil {
			return summary, err
		}
		results = append(results, reviewed)
	}
	summary.Results = results
	if err := s.TransitionWorksheetForScope(scope, worksheet.ID, WorksheetStatusCompleted, actor); err != nil {
		return summary, err
	}
	if loaded, ok := s.GetWorksheetForScope(scope, worksheet.ID); ok {
		summary.Worksheet = loaded
	} else {
		return summary, fmt.Errorf("worksheet %q disappeared", worksheet.ID)
	}

	if err := s.TransitionSampleForScope(scope, sample.ID, StatusInReview, actor); err != nil {
		return summary, err
	}

	qcSample, err := s.CreateSampleForScope(scope, CreateSampleInput{ClientID: client.ID, ProjectID: project.ID, ClientSampleID: input.ClientSampleID + "-MB", LabSampleID: input.LabSampleID + "-MB", MatrixReferenceID: matrixRef.ID, AnalysisServiceIDs: []string{leadService.ID}}, actor)
	if err != nil {
		return summary, err
	}
	batch, err := s.CreateQCBatchForScope(scope, CreateQCBatchInput{Name: "MVP QC batch", MethodID: method.ID, Matrix: matrixRef.Name}, actor)
	if err != nil {
		return summary, err
	}
	if _, err := s.AddQCItemToBatchForScope(scope, batch.ID, CreateQCItemInput{SampleID: sample.ID, Role: QCItemRoleClientSample}, actor); err != nil {
		return summary, err
	}
	qcItem, err := s.AddQCItemToBatchForScope(scope, batch.ID, CreateQCItemInput{SampleID: qcSample.ID, Role: QCItemRoleQCSample, QCSampleKind: QCSampleKindMethodBlank}, actor)
	if err != nil {
		return summary, err
	}
	if _, err := s.CreateQCRelationshipForScope(scope, CreateQCRelationshipInput{QCBatchID: batch.ID, QCItemID: qcItem.ID, RelationshipType: QCRelationshipTypeBatchControl, RelatedSampleID: sample.ID}, actor); err != nil {
		return summary, err
	}
	if err := s.TransitionSampleForScope(scope, sample.ID, StatusReleased, actor); err != nil {
		summary.DeniedControls = append(summary.DeniedControls, "release_before_preconditions:"+err.Error())
	} else {
		return summary, errors.New("release before accepted QC unexpectedly succeeded")
	}
	if _, err := s.TransitionQCBatchForScope(scope, batch.ID, QCBatchStatusInReview, "ready for MVP review", actor); err != nil {
		return summary, err
	}
	acceptedBatch, err := s.TransitionQCBatchForScope(scope, batch.ID, QCBatchStatusAccepted, "accepted for MVP release", actor)
	if err != nil {
		return summary, err
	}
	summary.QCBatch = acceptedBatch

	if err := s.TransitionSampleForScope(scope, sample.ID, StatusReleased, actor); err != nil {
		return summary, err
	}
	if loaded, ok := s.GetSampleForScope(scope, sample.ID); ok {
		summary.Sample = loaded
	} else {
		return summary, fmt.Errorf("sample %q disappeared", sample.ID)
	}

	if _, err := s.GenerateCOAReportArtifactForScope(Scope{TenantID: "other-tenant", LabID: scope.LabID}, COAGenerationInput{SampleID: sample.ID, Template: COATemplate{ID: input.TemplateID, Version: input.TemplateVersion, Style: COAStyleCENLA, LabName: "Clearline Demo Lab", ClientName: client.Name}}, mvpActor(Scope{TenantID: "other-tenant", LabID: scope.LabID}, "other-releaser", RoleReportReleaser)); err != nil {
		summary.DeniedControls = append(summary.DeniedControls, "cross_tenant_attempt:"+err.Error())
	} else {
		return summary, errors.New("cross-tenant report generation unexpectedly succeeded")
	}
	released, err := s.GenerateCOAReportArtifactForScope(scope, COAGenerationInput{SampleID: sample.ID, Template: COATemplate{ID: input.TemplateID, Version: input.TemplateVersion, Style: COAStyleCENLA, LabName: "Clearline Demo Lab", ClientName: client.Name}}, mvpActor(scope, "mvp-releaser", RoleReportReleaser))
	if err != nil {
		return summary, err
	}
	summary.Report = released
	if _, err := s.UpdateResultForScope(scope, results[0].ID, ResultInput{AnalysisRequestLineID: lines[0].ID, Value: 99, RawValue: "99 mg/L", Unit: "mg/L", Dilution: 1}, mvpActor(scope, "mvp-analyst", RoleAnalyst)); err != nil {
		summary.DeniedControls = append(summary.DeniedControls, "mutate_released_artifact:"+err.Error())
	} else {
		return summary, errors.New("locked result update unexpectedly succeeded")
	}
	return summary, nil
}

func normalizeMVPVerticalSliceInput(input MVPVerticalSliceInput) MVPVerticalSliceInput {
	if strings.TrimSpace(input.ClientName) == "" {
		input.ClientName = "Clearline Demo Client"
	}
	if strings.TrimSpace(input.ContactName) == "" {
		input.ContactName = "Synthetic Submitter"
	}
	if strings.TrimSpace(input.ProjectName) == "" {
		input.ProjectName = "MVP synthetic project"
	}
	if strings.TrimSpace(input.WorkOrder) == "" {
		input.WorkOrder = "WO-MVP-005"
	}
	if strings.TrimSpace(input.ClientSampleID) == "" {
		input.ClientSampleID = "MVP-CLIENT-001"
	}
	if strings.TrimSpace(input.LabSampleID) == "" {
		input.LabSampleID = "MVP-LAB-001"
	}
	if strings.TrimSpace(input.AnalysisProfile) == "" {
		input.AnalysisProfile = "MVP Metals Profile"
	}
	if strings.TrimSpace(input.TemplateID) == "" {
		input.TemplateID = "coa-mvp-standard"
	}
	if strings.TrimSpace(input.TemplateVersion) == "" {
		input.TemplateVersion = "2026.06-mvp"
	}
	return input
}

func analysisRequestLineIDs(lines []AnalysisRequestLine) []string {
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		ids = append(ids, line.ID)
	}
	return ids
}

func mvpActor(scope Scope, userID string, roles ...Role) ActorContext {
	roleStrings := make([]string, 0, len(roles))
	for _, role := range roles {
		roleStrings = append(roleStrings, string(role))
	}
	return MustActorContext(ActorContextInput{UserID: userID, DisplayName: userID, AuthProvider: "mvp-vertical-slice", RequestID: "req-" + userID, CorrelationID: "mvp-005", TenantMemberships: []TenantMembership{{TenantID: scope.TenantID, Roles: roleStrings}}, Roles: roleStrings})
}

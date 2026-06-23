package lab

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type canonicalSampleReconciliationRow struct {
	Row               int    `json:"row,omitempty"`
	SourceID          string `json:"source_id,omitempty"`
	ImportedID        string `json:"imported_id,omitempty"`
	ClientSampleID    string `json:"client_sample_id"`
	LabSampleID       string `json:"lab_sample_id"`
	Matrix            string `json:"matrix"`
	ContainerCount    int    `json:"container_count"`
	AnalysisCount     int    `json:"analysis_count"`
	CustodyEventCount int    `json:"custody_event_count"`
}

func (s *Store) SampleImportReconciliationReportForScope(scope Scope, result ImportResult, actor ActorContext) (ImportReconciliationReport, error) {
	scope, err := normalizeScope(scope)
	if err != nil {
		return ImportReconciliationReport{}, err
	}
	if result.Entity != ImportEntitySamples {
		return ImportReconciliationReport{}, fmt.Errorf("unsupported reconciliation entity %q", result.Entity)
	}
	actor = normalizeActorContext(actor, "sample-import-reconciliation")
	samples := s.SamplesForScope(scope)
	sampleByID := map[string]Sample{}
	for _, sample := range samples {
		sampleByID[sample.ID] = sample
	}
	report := ImportReconciliationReport{Provenance: ReconciliationProvenance{Source: result.Source, Entity: ImportEntitySamples, Format: result.Format, GeneratedAt: time.Now().UTC(), GeneratedBy: actor.UserID}, SourceCount: result.TotalRows}
	if report.Provenance.Source == "" {
		report.Provenance.Source = "inline." + string(result.Format)
	}
	if report.Provenance.Format == "" {
		report.Provenance.Format = ImportFormatCSV
	}

	importedIDs := map[string]bool{}
	sourceHashRows := make([]canonicalSampleReconciliationRow, 0, len(result.Rows))
	importedHashRows := make([]canonicalSampleReconciliationRow, 0, len(result.Rows))
	for _, row := range result.Rows {
		if row.Action == ReconciliationActionSkip {
			continue
		}
		report.ImportedCount++
		source := canonicalSampleSourceRow(row)
		sourceHashRows = append(sourceHashRows, source)
		importedIDs[row.ID] = true
		sample, ok := sampleByID[row.ID]
		if !ok {
			report.MissingRecords = append(report.MissingRecords, ReconciliationRecord{Row: row.Row, SourceID: source.SourceID, ImportedID: row.ID, SourceHash: hashAny(source), Reason: "import result references a sample that is not present"})
			continue
		}
		imported := canonicalSampleImportedRow(row, sample)
		importedHashRows = append(importedHashRows, imported)
		if sampleRowsMatch(source, imported) {
			report.MatchedCount++
			continue
		}
		report.MismatchedRecords = append(report.MismatchedRecords, ReconciliationRecord{Row: row.Row, SourceID: source.SourceID, ImportedID: row.ID, SourceHash: hashAny(source), ImportedHash: hashAny(imported), Reason: "source row and imported sample differ", FieldDiffs: sampleFieldDiffs(source, imported)})
	}
	for _, sample := range samples {
		if importedIDs[sample.ID] {
			continue
		}
		extra := canonicalSampleReconciliationRow{ImportedID: sample.ID, ClientSampleID: sample.ClientSampleID, LabSampleID: sample.LabSampleID, Matrix: sample.Matrix, ContainerCount: len(sample.Containers), AnalysisCount: len(sample.Analyses), CustodyEventCount: len(sample.CustodyEvents)}
		report.ExtraRecords = append(report.ExtraRecords, ReconciliationRecord{ImportedID: sample.ID, ImportedHash: hashAny(extra), Reason: "sample exists in Project Scientist but not in import result"})
		importedHashRows = append(importedHashRows, extra)
	}
	sort.Slice(report.MissingRecords, func(i, j int) bool { return report.MissingRecords[i].Row < report.MissingRecords[j].Row })
	sort.Slice(report.MismatchedRecords, func(i, j int) bool { return report.MismatchedRecords[i].Row < report.MismatchedRecords[j].Row })
	sort.Slice(report.ExtraRecords, func(i, j int) bool { return report.ExtraRecords[i].ImportedID < report.ExtraRecords[j].ImportedID })
	report.SourceHash = hashAny(sourceHashRows)
	report.ImportedHash = hashAny(importedHashRows)
	if err := s.auditReconciliationReport(scope, report, actor); err != nil {
		return ImportReconciliationReport{}, err
	}
	events, err := s.AuditEventsForScope(scope, 0)
	if err != nil {
		return ImportReconciliationReport{}, err
	}
	if len(events) > 0 {
		report.AuditEventID = events[len(events)-1].EventID
	}
	return report, nil
}

func canonicalSampleSourceRow(row ImportRowResult) canonicalSampleReconciliationRow {
	return canonicalSampleReconciliationRow{Row: row.Row, SourceID: strings.TrimSpace(row.Data["legacy_id"]), ImportedID: row.ID, ClientSampleID: strings.TrimSpace(row.Data["client_sample_id"]), LabSampleID: strings.TrimSpace(row.Data["lab_sample_id"]), Matrix: strings.TrimSpace(row.Data["matrix"]), ContainerCount: len(stringSliceFromJSON(row.Data["containers_json"])), AnalysisCount: len(stringSliceFromJSON(row.Data["analyses_json"])), CustodyEventCount: len(stringSliceFromJSON(row.Data["custody_events_json"]))}
}

func canonicalSampleImportedRow(row ImportRowResult, sample Sample) canonicalSampleReconciliationRow {
	return canonicalSampleReconciliationRow{Row: row.Row, SourceID: strings.TrimSpace(row.Data["legacy_id"]), ImportedID: sample.ID, ClientSampleID: strings.TrimSpace(sample.ClientSampleID), LabSampleID: strings.TrimSpace(sample.LabSampleID), Matrix: strings.TrimSpace(sample.Matrix), ContainerCount: len(sample.Containers), AnalysisCount: len(sample.Analyses), CustodyEventCount: len(sample.CustodyEvents)}
}

func sampleRowsMatch(source, imported canonicalSampleReconciliationRow) bool {
	return source.ClientSampleID == imported.ClientSampleID && source.LabSampleID == imported.LabSampleID && source.Matrix == imported.Matrix && source.ContainerCount == imported.ContainerCount && source.AnalysisCount == imported.AnalysisCount && source.CustodyEventCount == imported.CustodyEventCount
}

func sampleFieldDiffs(source, imported canonicalSampleReconciliationRow) map[string]ReconciliationDiff {
	diffs := map[string]ReconciliationDiff{}
	add := func(field, src, imp string) {
		if src != imp {
			diffs[field] = ReconciliationDiff{Source: src, Imported: imp}
		}
	}
	add("client_sample_id", source.ClientSampleID, imported.ClientSampleID)
	add("lab_sample_id", source.LabSampleID, imported.LabSampleID)
	add("matrix", source.Matrix, imported.Matrix)
	add("container_count", fmt.Sprint(source.ContainerCount), fmt.Sprint(imported.ContainerCount))
	add("analysis_count", fmt.Sprint(source.AnalysisCount), fmt.Sprint(imported.AnalysisCount))
	add("custody_event_count", fmt.Sprint(source.CustodyEventCount), fmt.Sprint(imported.CustodyEventCount))
	return diffs
}

package lab

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ImportReconciliationReport struct {
	Provenance        ReconciliationProvenance `json:"provenance"`
	SourceCount       int                      `json:"source_count"`
	ImportedCount     int                      `json:"imported_count"`
	MatchedCount      int                      `json:"matched_count"`
	MissingRecords    []ReconciliationRecord   `json:"missing_records,omitempty"`
	ExtraRecords      []ReconciliationRecord   `json:"extra_records,omitempty"`
	MismatchedRecords []ReconciliationRecord   `json:"mismatched_records,omitempty"`
	SourceHash        string                   `json:"source_hash"`
	ImportedHash      string                   `json:"imported_hash"`
	AuditEventID      string                   `json:"audit_event_id"`
}

type ReconciliationProvenance struct {
	Source      string       `json:"source"`
	Entity      string       `json:"entity"`
	Format      ImportFormat `json:"format"`
	GeneratedAt time.Time    `json:"generated_at"`
	GeneratedBy string       `json:"generated_by"`
}

type ReconciliationRecord struct {
	Row          int                           `json:"row,omitempty"`
	SourceID     string                        `json:"source_id,omitempty"`
	ImportedID   string                        `json:"imported_id,omitempty"`
	SourceHash   string                        `json:"source_hash,omitempty"`
	ImportedHash string                        `json:"imported_hash,omitempty"`
	Reason       string                        `json:"reason,omitempty"`
	FieldDiffs   map[string]ReconciliationDiff `json:"field_diffs,omitempty"`
}

type ReconciliationDiff struct {
	Source   string `json:"source"`
	Imported string `json:"imported"`
}

func (s *Store) ClientImportReconciliationReportForScope(scope Scope, result ImportResult, actor ActorContext) (ImportReconciliationReport, error) {
	scope, err := normalizeScope(scope)
	if err != nil {
		return ImportReconciliationReport{}, err
	}
	if result.Entity != "" && result.Entity != ImportEntityClients {
		return ImportReconciliationReport{}, fmt.Errorf("unsupported reconciliation entity %q", result.Entity)
	}
	actor = normalizeActorContext(actor, "import-reconciliation")
	clients := s.ClientsForScope(scope)
	clientByID := map[string]Client{}
	for _, client := range clients {
		clientByID[client.ID] = client
	}

	report := ImportReconciliationReport{
		Provenance:  ReconciliationProvenance{Source: result.Source, Entity: ImportEntityClients, Format: result.Format, GeneratedAt: time.Now().UTC(), GeneratedBy: actor.UserID},
		SourceCount: result.TotalRows,
	}
	if report.Provenance.Source == "" {
		report.Provenance.Source = "inline." + string(result.Format)
	}
	if report.Provenance.Format == "" {
		report.Provenance.Format = ImportFormatCSV
	}

	importedIDs := map[string]bool{}
	sourceHashRows := make([]canonicalReconciliationRow, 0, len(result.Rows))
	importedHashRows := make([]canonicalReconciliationRow, 0, len(result.Rows))
	for _, row := range result.Rows {
		if row.Action == ReconciliationActionSkip {
			continue
		}
		report.ImportedCount++
		source := canonicalSourceRow(row)
		sourceHashRows = append(sourceHashRows, source)
		importedIDs[row.ID] = true
		client, ok := clientByID[row.ID]
		if !ok {
			report.MissingRecords = append(report.MissingRecords, ReconciliationRecord{Row: row.Row, SourceID: source.SourceID, ImportedID: row.ID, SourceHash: hashAny(source), Reason: "import result references an object that is not present"})
			continue
		}
		imported := canonicalClientRow(row, client)
		importedHashRows = append(importedHashRows, imported)
		if source.Name == imported.Name && source.Email == imported.Email {
			report.MatchedCount++
			continue
		}
		report.MismatchedRecords = append(report.MismatchedRecords, ReconciliationRecord{Row: row.Row, SourceID: source.SourceID, ImportedID: row.ID, SourceHash: hashAny(source), ImportedHash: hashAny(imported), Reason: "source row and imported object differ", FieldDiffs: fieldDiffs(source, imported)})
	}

	for _, client := range clients {
		if importedIDs[client.ID] {
			continue
		}
		extra := canonicalReconciliationRow{ImportedID: client.ID, Name: strings.TrimSpace(client.Name), Email: strings.TrimSpace(client.Email)}
		report.ExtraRecords = append(report.ExtraRecords, ReconciliationRecord{ImportedID: client.ID, ImportedHash: hashAny(extra), Reason: "object exists in Project Scientist but not in import result"})
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

func (s *Store) auditReconciliationReport(scope Scope, report ImportReconciliationReport, actor ActorContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withTx(func(tx *sql.Tx) error {
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "import.reconciliation_reported", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "import_reconciliation", ID: report.Provenance.Source}, Details: map[string]any{"entity": report.Provenance.Entity, "format": string(report.Provenance.Format), "source": report.Provenance.Source, "source_count": report.SourceCount, "imported_count": report.ImportedCount, "matched_count": report.MatchedCount, "missing_count": len(report.MissingRecords), "extra_count": len(report.ExtraRecords), "mismatched_count": len(report.MismatchedRecords), "source_hash": report.SourceHash, "imported_hash": report.ImportedHash}})
	})
}

func (r ImportReconciliationReport) Markdown() string {
	var b strings.Builder
	b.WriteString("# Import Reconciliation Report\n\n")
	b.WriteString(fmt.Sprintf("Source: %s\n", r.Provenance.Source))
	b.WriteString(fmt.Sprintf("Entity: %s\n", r.Provenance.Entity))
	b.WriteString(fmt.Sprintf("Format: %s\n", r.Provenance.Format))
	b.WriteString(fmt.Sprintf("Generated by: %s\n", r.Provenance.GeneratedBy))
	if !r.Provenance.GeneratedAt.IsZero() {
		b.WriteString(fmt.Sprintf("Generated at: %s\n", r.Provenance.GeneratedAt.Format(time.RFC3339)))
	}
	b.WriteString(fmt.Sprintf("Audit event: %s\n", r.AuditEventID))
	b.WriteString(fmt.Sprintf("Source SHA-256: %s\n", r.SourceHash))
	b.WriteString(fmt.Sprintf("Imported SHA-256: %s\n\n", r.ImportedHash))
	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("Source rows: %d\n", r.SourceCount))
	b.WriteString(fmt.Sprintf("Imported rows: %d\n", r.ImportedCount))
	b.WriteString(fmt.Sprintf("Matched: %d\n", r.MatchedCount))
	b.WriteString(fmt.Sprintf("Missing: %d\n", len(r.MissingRecords)))
	b.WriteString(fmt.Sprintf("Extra: %d\n", len(r.ExtraRecords)))
	b.WriteString(fmt.Sprintf("Mismatched: %d\n\n", len(r.MismatchedRecords)))
	writeReconciliationSection(&b, "Missing records", r.MissingRecords)
	writeReconciliationSection(&b, "Extra records", r.ExtraRecords)
	writeReconciliationSection(&b, "Mismatched records", r.MismatchedRecords)
	return b.String()
}

func writeReconciliationSection(b *strings.Builder, title string, records []ReconciliationRecord) {
	b.WriteString("## " + title + "\n\n")
	if len(records) == 0 {
		b.WriteString("None.\n\n")
		return
	}
	for _, record := range records {
		b.WriteString(fmt.Sprintf("- row=%d source_id=%s imported_id=%s reason=%s\n", record.Row, record.SourceID, record.ImportedID, record.Reason))
		if len(record.FieldDiffs) > 0 {
			fields := make([]string, 0, len(record.FieldDiffs))
			for field := range record.FieldDiffs {
				fields = append(fields, field)
			}
			sort.Strings(fields)
			for _, field := range fields {
				diff := record.FieldDiffs[field]
				b.WriteString(fmt.Sprintf("  - %s: source=%q imported=%q\n", field, diff.Source, diff.Imported))
			}
		}
	}
	b.WriteString("\n")
}

type canonicalReconciliationRow struct {
	Row        int    `json:"row,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
	ImportedID string `json:"imported_id,omitempty"`
	Name       string `json:"name"`
	Email      string `json:"email"`
}

func canonicalSourceRow(row ImportRowResult) canonicalReconciliationRow {
	return canonicalReconciliationRow{Row: row.Row, SourceID: strings.TrimSpace(row.Data["legacy_id"]), ImportedID: row.ID, Name: strings.TrimSpace(row.Data["name"]), Email: strings.TrimSpace(row.Data["email"])}
}

func canonicalClientRow(row ImportRowResult, client Client) canonicalReconciliationRow {
	return canonicalReconciliationRow{Row: row.Row, SourceID: strings.TrimSpace(row.Data["legacy_id"]), ImportedID: client.ID, Name: strings.TrimSpace(client.Name), Email: strings.TrimSpace(client.Email)}
}

func fieldDiffs(source, imported canonicalReconciliationRow) map[string]ReconciliationDiff {
	diffs := map[string]ReconciliationDiff{}
	if source.Name != imported.Name {
		diffs["name"] = ReconciliationDiff{Source: source.Name, Imported: imported.Name}
	}
	if source.Email != imported.Email {
		diffs["email"] = ReconciliationDiff{Source: source.Email, Imported: imported.Email}
	}
	return diffs
}

func hashAny(value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

type canonicalAnalysisResultReconciliationRow struct {
	Row                    int    `json:"row,omitempty"`
	SourceID               string `json:"source_id,omitempty"`
	ImportedID             string `json:"imported_id,omitempty"`
	AnalysisRequestLineID  string `json:"analysis_request_line_id"`
	Service                string `json:"service"`
	Method                 string `json:"method"`
	Unit                   string `json:"unit"`
	Result                 string `json:"result"`
	Qualifier              string `json:"qualifier"`
	MDL                    string `json:"mdl"`
	RL                     string `json:"rl"`
	Comments               string `json:"comments"`
	CatalogSnapshotID      string `json:"catalog_snapshot_id,omitempty"`
	CatalogSnapshotVersion int    `json:"catalog_snapshot_version,omitempty"`
}

func (s *Store) AnalysisResultImportReconciliationReportForScope(scope Scope, result ImportResult, actor ActorContext) (ImportReconciliationReport, error) {
	scope, err := normalizeScope(scope)
	if err != nil {
		return ImportReconciliationReport{}, err
	}
	if result.Entity != "" && result.Entity != ImportEntityAnalysisResults {
		return ImportReconciliationReport{}, fmt.Errorf("unsupported reconciliation entity %q", result.Entity)
	}
	actor = normalizeActorContext(actor, "analysis-result-reconciliation")
	report := ImportReconciliationReport{
		Provenance:  ReconciliationProvenance{Source: result.Source, Entity: ImportEntityAnalysisResults, Format: result.Format, GeneratedAt: time.Now().UTC(), GeneratedBy: actor.UserID},
		SourceCount: result.TotalRows,
	}
	if report.Provenance.Source == "" {
		report.Provenance.Source = "inline." + string(result.Format)
	}
	if report.Provenance.Format == "" {
		report.Provenance.Format = ImportFormatCSV
	}

	sourceHashRows := make([]canonicalAnalysisResultReconciliationRow, 0, len(result.Rows))
	importedHashRows := make([]canonicalAnalysisResultReconciliationRow, 0, len(result.Rows))
	for _, row := range result.Rows {
		if row.Action == ReconciliationActionSkip {
			continue
		}
		report.ImportedCount++
		source := canonicalAnalysisResultSourceRow(row)
		sourceHashRows = append(sourceHashRows, source)
		persisted, ok := s.analysisResultReconciliationImportedRow(scope, row.ID, row.Row, row.Data)
		if !ok {
			report.MissingRecords = append(report.MissingRecords, ReconciliationRecord{Row: row.Row, SourceID: source.SourceID, ImportedID: row.ID, SourceHash: hashAny(source), Reason: "import result references a result that is not present"})
			continue
		}
		importedHashRows = append(importedHashRows, persisted)
		if analysisResultRowsMatch(source, persisted) {
			report.MatchedCount++
			continue
		}
		report.MismatchedRecords = append(report.MismatchedRecords, ReconciliationRecord{Row: row.Row, SourceID: source.SourceID, ImportedID: row.ID, SourceHash: hashAny(source), ImportedHash: hashAny(persisted), Reason: "source analysis result row and imported result/snapshot differ", FieldDiffs: analysisResultFieldDiffs(source, persisted)})
	}
	sort.Slice(report.MissingRecords, func(i, j int) bool { return report.MissingRecords[i].Row < report.MissingRecords[j].Row })
	sort.Slice(report.MismatchedRecords, func(i, j int) bool { return report.MismatchedRecords[i].Row < report.MismatchedRecords[j].Row })
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

func canonicalAnalysisResultSourceRow(row ImportRowResult) canonicalAnalysisResultReconciliationRow {
	return canonicalAnalysisResultReconciliationRow{Row: row.Row, SourceID: strings.TrimSpace(row.Data["legacy_id"]), ImportedID: row.ID, AnalysisRequestLineID: strings.TrimSpace(row.Data["analysis_request_line_id"]), Service: strings.TrimSpace(row.Data["service"]), Method: strings.TrimSpace(row.Data["method"]), Unit: strings.TrimSpace(row.Data["unit"]), Result: strings.TrimSpace(row.Data["result"]), Qualifier: strings.TrimSpace(row.Data["qualifier"]), MDL: normalizeNumericString(row.Data["mdl"]), RL: normalizeNumericString(row.Data["rl"]), Comments: strings.TrimSpace(row.Data["comments"])}
}

func (s *Store) analysisResultReconciliationImportedRow(scope Scope, resultID string, rowNumber int, source ImportRow) (canonicalAnalysisResultReconciliationRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := resultByIDTx(s.db, resultID)
	if err != nil || result.TenantID != scope.TenantID || result.LabID != scope.LabID {
		return canonicalAnalysisResultReconciliationRow{}, false
	}
	line, err := analysisRequestLineByIDTx(s.db, result.AnalysisRequestLineID)
	if err != nil || line.TenantID != scope.TenantID || line.LabID != scope.LabID {
		return canonicalAnalysisResultReconciliationRow{}, false
	}
	return canonicalAnalysisResultReconciliationRow{Row: rowNumber, SourceID: strings.TrimSpace(source["legacy_id"]), ImportedID: result.ID, AnalysisRequestLineID: result.AnalysisRequestLineID, Service: strings.TrimSpace(line.Name), Method: strings.TrimSpace(line.MethodName), Unit: strings.TrimSpace(result.Unit), Result: strings.TrimSpace(result.RawValue), Qualifier: strings.TrimSpace(result.Qualifier), MDL: normalizeNumericString(fmt.Sprintf("%g", result.MDL)), RL: normalizeNumericString(fmt.Sprintf("%g", result.RL)), Comments: strings.TrimSpace(result.Comments), CatalogSnapshotID: line.CatalogSnapshotID, CatalogSnapshotVersion: line.CatalogSnapshotVersion}, true
}

func analysisResultRowsMatch(source, imported canonicalAnalysisResultReconciliationRow) bool {
	return source.AnalysisRequestLineID == imported.AnalysisRequestLineID && source.Service == imported.Service && source.Method == imported.Method && source.Unit == imported.Unit && source.Result == imported.Result && source.Qualifier == imported.Qualifier && source.MDL == imported.MDL && source.RL == imported.RL && source.Comments == imported.Comments && imported.CatalogSnapshotID != "" && imported.CatalogSnapshotVersion > 0
}

func analysisResultFieldDiffs(source, imported canonicalAnalysisResultReconciliationRow) map[string]ReconciliationDiff {
	diffs := map[string]ReconciliationDiff{}
	add := func(field, sourceValue, importedValue string) {
		if sourceValue != importedValue {
			diffs[field] = ReconciliationDiff{Source: sourceValue, Imported: importedValue}
		}
	}
	add("analysis_request_line_id", source.AnalysisRequestLineID, imported.AnalysisRequestLineID)
	add("service", source.Service, imported.Service)
	add("method", source.Method, imported.Method)
	add("unit", source.Unit, imported.Unit)
	add("result", source.Result, imported.Result)
	add("qualifier", source.Qualifier, imported.Qualifier)
	add("mdl", source.MDL, imported.MDL)
	add("rl", source.RL, imported.RL)
	add("comments", source.Comments, imported.Comments)
	if imported.CatalogSnapshotID == "" || imported.CatalogSnapshotVersion == 0 {
		diffs["catalog_snapshot"] = ReconciliationDiff{Source: "immutable snapshot present", Imported: fmt.Sprintf("id=%s version=%d", imported.CatalogSnapshotID, imported.CatalogSnapshotVersion)}
	}
	return diffs
}

func normalizeNumericString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "0"
	}
	var value float64
	if _, err := fmt.Sscanf(trimmed, "%f", &value); err != nil {
		return trimmed
	}
	return fmt.Sprintf("%g", value)
}

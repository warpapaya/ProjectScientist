package lab

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ImportFormat string

const (
	ImportFormatCSV  ImportFormat = "csv"
	ImportFormatJSON ImportFormat = "json"
	ImportFormatXLSX ImportFormat = "xlsx"
)

const ImportEntityClients = "clients"
const ImportEntityAnalysisResults = "analysis_results"

const (
	ReconciliationActionCreate = "create"
	ReconciliationActionSkip   = "skip"
)

type ImportRow map[string]string

type ImportOptions struct {
	Format     ImportFormat
	Entity     string
	DryRun     bool
	Source     string
	Reconciler ImportReconciler
}

type ExportOptions struct {
	Format ImportFormat
	Entity string
}

type ImportReconciler func(row ImportRow, existing ReconciliationSnapshot) ReconciliationDecision

type ReconciliationSnapshot struct {
	Clients []Client
}

type ReconciliationDecision struct {
	Action     string
	ExistingID string
	Reason     string
}

type ImportValidationError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ImportRowResult struct {
	Row        int       `json:"row"`
	Action     string    `json:"action"`
	ID         string    `json:"id,omitempty"`
	ExistingID string    `json:"existing_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	Data       ImportRow `json:"data"`
}

type ImportResult struct {
	DryRun      bool                    `json:"dry_run"`
	Source      string                  `json:"source"`
	Entity      string                  `json:"entity"`
	Format      ImportFormat            `json:"format"`
	TotalRows   int                     `json:"total_rows"`
	ValidRows   int                     `json:"valid_rows"`
	CreatedRows int                     `json:"created_rows"`
	SkippedRows int                     `json:"skipped_rows"`
	Errors      []ImportValidationError `json:"errors,omitempty"`
	Rows        []ImportRowResult       `json:"rows"`
}

func (s *Store) ImportForScope(scope Scope, payload []byte, opts ImportOptions, actor ActorContext) (ImportResult, error) {
	scope, err := normalizeScope(scope)
	if err != nil {
		return ImportResult{}, err
	}
	opts = normalizeImportOptions(opts)
	rows, err := decodeImportRows(payload, opts.Format)
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{DryRun: opts.DryRun, Source: opts.Source, Entity: opts.Entity, Format: opts.Format, TotalRows: len(rows), Rows: make([]ImportRowResult, 0, len(rows))}
	validationErrors := validateImportRows(opts.Entity, rows)
	if len(validationErrors) > 0 {
		result.Errors = validationErrors
		return result, importValidationError(validationErrors)
	}
	result.ValidRows = len(rows)
	existing := s.reconciliationSnapshotForScope(scope)
	for i, row := range rows {
		rowNum := i + 1
		decision := ReconciliationDecision{Action: ReconciliationActionCreate}
		if opts.Reconciler != nil {
			decision = normalizeReconciliationDecision(opts.Reconciler(copyImportRow(row), existing))
		}
		rowResult := ImportRowResult{Row: rowNum, Action: decision.Action, ExistingID: strings.TrimSpace(decision.ExistingID), Reason: strings.TrimSpace(decision.Reason), Data: copyImportRow(row)}
		if decision.Action == ReconciliationActionSkip {
			result.SkippedRows++
		} else if opts.DryRun {
			rowResult.Action = ReconciliationActionCreate
		} else {
			result.CreatedRows++
		}
		result.Rows = append(result.Rows, rowResult)
	}
	if opts.DryRun || len(rows) == 0 {
		return result, nil
	}
	if opts.Entity == ImportEntityAnalysisResults {
		return s.importAnalysisResultsForScope(scope, rows, opts, result, actor)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationImportRun, actor, AuditResource{Type: "import", ID: opts.Source}, map[string]any{"entity": opts.Entity, "format": string(opts.Format), "row_count": len(rows)})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			return ErrAuthorizationDenied
		}
		for i := range result.Rows {
			if result.Rows[i].Action == ReconciliationActionSkip {
				continue
			}
			client, err := insertImportedClientTx(tx, scope, rows[i])
			if err != nil {
				return err
			}
			result.Rows[i].ID = client.ID
			if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "client.imported", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "client", ID: client.ID}, Details: map[string]any{"source": opts.Source, "row": i + 1, "legacy_id": rows[i]["legacy_id"], "name": client.Name}}); err != nil {
				return err
			}
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "import.completed", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "import", ID: opts.Source}, Details: map[string]any{"entity": opts.Entity, "format": string(opts.Format), "source": opts.Source, "total_rows": result.TotalRows, "created_rows": result.CreatedRows, "skipped_rows": result.SkippedRows}})
	})
	if err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

func (s *Store) ExportForScope(scope Scope, opts ExportOptions) ([]byte, error) {
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil, err
	}
	format := opts.Format
	if format == "" {
		format = ImportFormatCSV
	}
	entity := strings.TrimSpace(opts.Entity)
	if entity == "" {
		entity = ImportEntityClients
	}
	if entity != ImportEntityClients {
		return nil, fmt.Errorf("unsupported export entity %q", entity)
	}
	clients := s.ClientsForScope(scope)
	rows := make([]ImportRow, 0, len(clients))
	for _, client := range clients {
		rows = append(rows, ImportRow{"id": client.ID, "name": client.Name, "email": client.Email, "tenant_id": client.TenantID, "lab_id": client.LabID})
	}
	headers := []string{"id", "name", "email", "tenant_id", "lab_id"}
	switch format {
	case ImportFormatCSV:
		return encodeRowsCSV(rows, headers)
	case ImportFormatJSON:
		return json.MarshalIndent(rows, "", "  ")
	case ImportFormatXLSX:
		return EncodeRowsXLSX(rows, headers)
	default:
		return nil, fmt.Errorf("unsupported export format %q", format)
	}
}

func normalizeImportOptions(opts ImportOptions) ImportOptions {
	if opts.Format == "" {
		opts.Format = ImportFormatCSV
	}
	if strings.TrimSpace(opts.Entity) == "" {
		opts.Entity = ImportEntityClients
	} else {
		opts.Entity = strings.TrimSpace(opts.Entity)
	}
	if strings.TrimSpace(opts.Source) == "" {
		opts.Source = "inline." + string(opts.Format)
	} else {
		opts.Source = strings.TrimSpace(opts.Source)
	}
	return opts
}

func decodeImportRows(payload []byte, format ImportFormat) ([]ImportRow, error) {
	switch format {
	case ImportFormatCSV:
		return decodeRowsCSV(payload)
	case ImportFormatJSON:
		return decodeRowsJSON(payload)
	case ImportFormatXLSX:
		return DecodeRowsXLSX(payload)
	default:
		return nil, fmt.Errorf("unsupported import format %q", format)
	}
}

func decodeRowsCSV(payload []byte) ([]ImportRow, error) {
	reader := csv.NewReader(bytes.NewReader(payload))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	headers := normalizeHeaders(records[0])
	rows := make([]ImportRow, 0, len(records)-1)
	for _, record := range records[1:] {
		row := ImportRow{}
		for i, header := range headers {
			if header == "" {
				continue
			}
			if i < len(record) {
				row[header] = strings.TrimSpace(record[i])
			} else {
				row[header] = ""
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func decodeRowsJSON(payload []byte) ([]ImportRow, error) {
	var raw []map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	rows := make([]ImportRow, 0, len(raw))
	for _, item := range raw {
		row := ImportRow{}
		for key, value := range item {
			row[normalizeHeader(key)] = strings.TrimSpace(fmt.Sprint(value))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func validateImportRows(entity string, rows []ImportRow) []ImportValidationError {
	switch entity {
	case ImportEntityClients:
		return validateClientImportRows(rows)
	case ImportEntityAnalysisResults:
		return validateAnalysisResultImportRows(rows)
	default:
		return []ImportValidationError{{Row: 0, Field: "entity", Message: fmt.Sprintf("unsupported import entity %q", entity)}}
	}
}

func validateClientImportRows(rows []ImportRow) []ImportValidationError {
	out := []ImportValidationError{}
	for i, row := range rows {
		rowNum := i + 1
		if strings.TrimSpace(row["name"]) == "" {
			out = append(out, ImportValidationError{Row: rowNum, Field: "name", Message: "name is required"})
		}
	}
	return out
}

func validateAnalysisResultImportRows(rows []ImportRow) []ImportValidationError {
	out := []ImportValidationError{}
	for i, row := range rows {
		rowNum := i + 1
		for _, field := range []string{"analysis_request_line_id", "service", "method", "unit", "result"} {
			if strings.TrimSpace(row[field]) == "" {
				out = append(out, ImportValidationError{Row: rowNum, Field: field, Message: field + " is required"})
			}
		}
		for _, field := range []string{"mdl", "rl"} {
			if value := strings.TrimSpace(row[field]); value != "" {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil || parsed < 0 {
					out = append(out, ImportValidationError{Row: rowNum, Field: field, Message: field + " must be a non-negative number"})
				}
			}
		}
	}
	return out
}

type importValidationError []ImportValidationError

func (e importValidationError) Error() string {
	parts := make([]string, 0, len(e))
	for _, item := range e {
		parts = append(parts, fmt.Sprintf("row %d %s: %s", item.Row, item.Field, item.Message))
	}
	return "import validation failed: " + strings.Join(parts, "; ")
}

func normalizeReconciliationDecision(decision ReconciliationDecision) ReconciliationDecision {
	decision.Action = strings.TrimSpace(decision.Action)
	if decision.Action == "" {
		decision.Action = ReconciliationActionCreate
	}
	if decision.Action != ReconciliationActionCreate && decision.Action != ReconciliationActionSkip {
		decision.Action = ReconciliationActionCreate
	}
	return decision
}

func (s *Store) reconciliationSnapshotForScope(scope Scope) ReconciliationSnapshot {
	return ReconciliationSnapshot{Clients: s.ClientsForScope(scope)}
}

func insertImportedClientTx(tx *sql.Tx, scope Scope, row ImportRow) (Client, error) {
	next, err := nextCounter(tx, "next_client")
	if err != nil {
		return Client{}, err
	}
	now := time.Now().UTC()
	client := Client{ID: fmt.Sprintf("C-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: strings.TrimSpace(row["name"]), Email: strings.TrimSpace(row["email"]), CreatedAt: now}
	if _, err := tx.Exec(`INSERT INTO clients(id, tenant_id, lab_id, name, email, created_at) VALUES (?, ?, ?, ?, ?, ?)`, client.ID, client.TenantID, client.LabID, client.Name, client.Email, formatTime(client.CreatedAt)); err != nil {
		return Client{}, err
	}
	return client, nil
}

func encodeRowsCSV(rows []ImportRow, headers []string) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, row := range rows {
		record := make([]string, len(headers))
		for i, header := range headers {
			record[i] = row[header]
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}

func normalizeHeaders(headers []string) []string {
	out := make([]string, len(headers))
	for i, header := range headers {
		out[i] = normalizeHeader(header)
	}
	return out
}

func normalizeHeader(header string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(header), " ", "_"))
}

func copyImportRow(row ImportRow) ImportRow {
	out := ImportRow{}
	for key, value := range row {
		out[key] = value
	}
	return out
}

func EncodeRowsXLSX(rows []ImportRow, headers []string) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   encodeWorksheetXML(rows, headers),
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		writer, err := zipWriter.Create(path)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte(files[path])); err != nil {
			return nil, err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeRowsXLSX(payload []byte) ([]ImportRow, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, err
	}
	var sheet []byte
	sharedStrings := []string{}
	for _, file := range zipReader.File {
		switch file.Name {
		case "xl/sharedStrings.xml":
			data, err := readZipFile(file)
			if err != nil {
				return nil, err
			}
			sharedStrings = parseSharedStrings(data)
		case "xl/worksheets/sheet1.xml":
			data, err := readZipFile(file)
			if err != nil {
				return nil, err
			}
			sheet = data
		}
	}
	if len(sheet) == 0 {
		return nil, errors.New("xlsx missing xl/worksheets/sheet1.xml")
	}
	grid, err := parseWorksheetRows(sheet, sharedStrings)
	if err != nil {
		return nil, err
	}
	if len(grid) == 0 {
		return nil, nil
	}
	headers := normalizeHeaders(grid[0])
	rows := make([]ImportRow, 0, len(grid)-1)
	for _, record := range grid[1:] {
		row := ImportRow{}
		for i, header := range headers {
			if header == "" {
				continue
			}
			if i < len(record) {
				row[header] = strings.TrimSpace(record[i])
			} else {
				row[header] = ""
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func encodeWorksheetXML(rows []ImportRow, headers []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	writeXLSXRow(&b, 1, headers)
	for i, row := range rows {
		values := make([]string, len(headers))
		for j, header := range headers {
			values[j] = row[header]
		}
		writeXLSXRow(&b, i+2, values)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func writeXLSXRow(b *strings.Builder, rowNumber int, values []string) {
	b.WriteString(`<row r="` + strconv.Itoa(rowNumber) + `">`)
	for i, value := range values {
		cellRef := columnName(i+1) + strconv.Itoa(rowNumber)
		b.WriteString(`<c r="` + cellRef + `" t="inlineStr"><is><t>` + xmlEscape(value) + `</t></is></c>`)
	}
	b.WriteString(`</row>`)
}

func parseWorksheetRows(payload []byte, sharedStrings []string) ([][]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	rows := [][]string{}
	var current []string
	inValue := false
	inText := false
	cellType := ""
	cellRef := ""
	cellValue := ""
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				current = []string{}
			case "c":
				cellType, cellRef, cellValue = "", "", ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "t" {
						cellType = attr.Value
					}
					if attr.Name.Local == "r" {
						cellRef = attr.Value
					}
				}
			case "v":
				inValue = true
			case "t":
				inText = true
			}
		case xml.CharData:
			if inValue || inText {
				cellValue += string([]byte(t))
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inValue = false
			case "t":
				inText = false
			case "c":
				value := cellValue
				if cellType == "s" {
					idx, _ := strconv.Atoi(strings.TrimSpace(cellValue))
					if idx >= 0 && idx < len(sharedStrings) {
						value = sharedStrings[idx]
					}
				}
				col := columnIndex(cellRef)
				if col <= 0 {
					col = len(current) + 1
				}
				for len(current) < col {
					current = append(current, "")
				}
				current[col-1] = strings.TrimSpace(value)
			case "row":
				rows = append(rows, current)
			}
		}
	}
	return rows, nil
}

func parseSharedStrings(payload []byte) []string {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	out := []string{}
	inText := false
	var current strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "si" {
				current.Reset()
			}
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.CharData:
			if inText {
				current.WriteString(string([]byte(t)))
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
			if t.Name.Local == "si" {
				out = append(out, current.String())
			}
		}
	}
	return out
}

func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func columnName(index int) string {
	name := ""
	for index > 0 {
		index--
		name = string(rune('A'+index%26)) + name
		index /= 26
	}
	return name
}

func columnIndex(ref string) int {
	index := 0
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			index = index*26 + int(r-'A'+1)
		} else if r >= 'a' && r <= 'z' {
			index = index*26 + int(r-'a'+1)
		} else {
			break
		}
	}
	return index
}

func xmlEscape(value string) string {
	return html.EscapeString(value)
}

func (s *Store) importAnalysisResultsForScope(scope Scope, rows []ImportRow, opts ImportOptions, result ImportResult, actor ActorContext) (ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationImportRun, actor, AuditResource{Type: "import", ID: opts.Source}, map[string]any{"entity": opts.Entity, "format": string(opts.Format), "row_count": len(rows)})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			return ErrAuthorizationDenied
		}
		for i := range result.Rows {
			if result.Rows[i].Action == ReconciliationActionSkip {
				continue
			}
			created, err := insertImportedAnalysisResultTx(tx, scope, rows[i], actor)
			if err != nil {
				return fmt.Errorf("row %d: %w", i+1, err)
			}
			result.Rows[i].ID = created.ID
			if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "analysis_result.imported", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "result", ID: created.ID}, Details: map[string]any{"source": opts.Source, "row": i + 1, "legacy_id": rows[i]["legacy_id"], "analysis_request_line_id": created.AnalysisRequestLineID, "unit": created.Unit, "qualifier": created.Qualifier, "mdl": created.MDL, "rl": created.RL}}); err != nil {
				return err
			}
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "import.completed", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "import", ID: opts.Source}, Details: map[string]any{"entity": opts.Entity, "format": string(opts.Format), "source": opts.Source, "total_rows": result.TotalRows, "created_rows": result.CreatedRows, "skipped_rows": result.SkippedRows}})
	})
	if err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

func insertImportedAnalysisResultTx(tx *sql.Tx, scope Scope, row ImportRow, actor ActorContext) (Result, error) {
	line, err := analysisRequestLineByIDTx(tx, row["analysis_request_line_id"])
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Result{}, fmt.Errorf("unknown analysis request line %q", row["analysis_request_line_id"])
		}
		return Result{}, err
	}
	if line.TenantID != scope.TenantID || line.LabID != scope.LabID {
		return Result{}, fmt.Errorf("analysis request line %q is outside requested tenant/lab scope", line.ID)
	}
	if line.Status == AnalysisRequestLineStatusCancelled {
		return Result{}, fmt.Errorf("cannot import result for cancelled analysis request line %q", line.ID)
	}
	if service := strings.TrimSpace(row["service"]); !strings.EqualFold(service, strings.TrimSpace(line.Name)) {
		return Result{}, fmt.Errorf("service %q does not match analysis request line %q service %q", service, line.ID, line.Name)
	}
	if method := strings.TrimSpace(row["method"]); method != "" && line.MethodName != "" && method != line.MethodName {
		return Result{}, fmt.Errorf("method %q does not match immutable analysis request line %q method snapshot %q", method, line.ID, line.MethodName)
	}
	next, err := nextCounter(tx, "next_result")
	if err != nil {
		return Result{}, err
	}
	now := time.Now().UTC()
	rawValue := strings.TrimSpace(row["result"])
	value := parseImportedResultValue(rawValue)
	mdl := parseOptionalNonNegativeFloat(row["mdl"])
	rl := parseOptionalNonNegativeFloat(row["rl"])
	input := ResultInput{AnalysisRequestLineID: line.ID, Value: value, RawValue: rawValue, Unit: row["unit"], Qualifier: row["qualifier"], MDL: mdl, RL: rl, Dilution: 1, Comments: row["comments"], AnalystID: normalizeActorContext(actor, "migration-import").UserID}
	input = normalizeResultInput(input)
	if err := validateResultInput(input, true); err != nil {
		return Result{}, err
	}
	created := resultFromInput(scope, fmt.Sprintf("R-%06d", next), line.SampleID, input, now)
	if _, err := tx.Exec(`INSERT INTO results(id, tenant_id, lab_id, sample_id, analysis_request_line_id, value, raw_value, unit, qualifier, mdl, rl, loq, dilution, uncertainty, comments, analyst_id, instrument_id, status, reviewed_by, review_comments, reviewed_at, reopen_reason, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, created.ID, created.TenantID, created.LabID, created.SampleID, created.AnalysisRequestLineID, created.Value, created.RawValue, created.Unit, created.Qualifier, created.MDL, created.RL, created.LOQ, created.Dilution, created.Uncertainty, created.Comments, created.AnalystID, created.InstrumentID, string(created.Status), created.ReviewedBy, created.ReviewComments, formatOptionalTime(created.ReviewedAt), created.ReopenReason, formatTime(created.CreatedAt), formatTime(created.UpdatedAt)); err != nil {
		return Result{}, err
	}
	return created, nil
}

func parseImportedResultValue(raw string) float64 {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(strings.Trim(fields[0], "<>="), 64)
	if err != nil {
		return 0
	}
	return value
}

func parseOptionalNonNegativeFloat(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

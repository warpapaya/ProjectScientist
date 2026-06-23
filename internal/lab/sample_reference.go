package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SampleReferenceKind string

const (
	SampleReferenceMatrix            SampleReferenceKind = "matrix"
	SampleReferenceContainer         SampleReferenceKind = "container"
	SampleReferencePreservative      SampleReferenceKind = "preservative"
	SampleReferenceStorageLocation   SampleReferenceKind = "storage_location"
	SampleReferenceReceivedCondition SampleReferenceKind = "received_condition"
)

type SampleReferenceItem struct {
	ID          string              `json:"id"`
	TenantID    string              `json:"tenant_id"`
	LabID       string              `json:"lab_id"`
	Kind        SampleReferenceKind `json:"kind"`
	Name        string              `json:"name"`
	Code        string              `json:"code,omitempty"`
	Description string              `json:"description,omitempty"`
	SortOrder   int                 `json:"sort_order"`
	Active      bool                `json:"active"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type SampleReferenceItemInput struct {
	Kind        SampleReferenceKind `json:"kind"`
	Name        string              `json:"name"`
	Code        string              `json:"code"`
	Description string              `json:"description"`
	SortOrder   int                 `json:"sort_order"`
	Active      bool                `json:"active"`
}

type SampleReferenceSeedSummary struct {
	MatrixCount            int `json:"matrix_count"`
	ContainerCount         int `json:"container_count"`
	PreservativeCount      int `json:"preservative_count"`
	StorageLocationCount   int `json:"storage_location_count"`
	ReceivedConditionCount int `json:"received_condition_count"`
	TotalCount             int `json:"total_count"`
}

func (s *Store) CreateSampleReferenceItem(input SampleReferenceItemInput, actor ActorContext) (SampleReferenceItem, error) {
	return s.CreateSampleReferenceItemForScope(defaultScope(), input, actor)
}

func (s *Store) CreateSampleReferenceItemForScope(scope Scope, input SampleReferenceItemInput, actor ActorContext) (SampleReferenceItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return SampleReferenceItem{}, err
	}
	input, err = normalizeSampleReferenceInput(input)
	if err != nil {
		return SampleReferenceItem{}, err
	}
	var item SampleReferenceItem
	err = s.withTx(func(tx *sql.Tx) error {
		if err := requireAuthorizedOperationTx(tx, scope, OperationCatalogConfigure, actor, AuditResource{Type: "sample_reference", ID: "configure"}, nil); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_sample_reference")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		item = SampleReferenceItem{ID: fmt.Sprintf("SR-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Kind: input.Kind, Name: input.Name, Code: input.Code, Description: input.Description, SortOrder: input.SortOrder, Active: input.Active, CreatedAt: now, UpdatedAt: now}
		if _, err := tx.Exec(`INSERT INTO sample_reference_items(id, tenant_id, lab_id, kind, name, code, description, sort_order, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.TenantID, item.LabID, string(item.Kind), item.Name, item.Code, item.Description, item.SortOrder, boolInt(item.Active), formatTime(item.CreatedAt), formatTime(item.UpdatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, sampleReferenceAudit(scope, actor, "sample_reference.created", item, map[string]any{"name": item.Name, "kind": string(item.Kind), "code": item.Code}))
	})
	return item, err
}

func (s *Store) UpdateSampleReferenceItem(id string, input SampleReferenceItemInput, actor ActorContext) (SampleReferenceItem, error) {
	return s.UpdateSampleReferenceItemForScope(defaultScope(), id, input, actor)
}

func (s *Store) UpdateSampleReferenceItemForScope(scope Scope, id string, input SampleReferenceItemInput, actor ActorContext) (SampleReferenceItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return SampleReferenceItem{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return SampleReferenceItem{}, errors.New("sample reference id is required")
	}
	input, err = normalizeSampleReferenceInput(input)
	if err != nil {
		return SampleReferenceItem{}, err
	}
	var item SampleReferenceItem
	err = s.withTx(func(tx *sql.Tx) error {
		if err := requireAuthorizedOperationTx(tx, scope, OperationCatalogConfigure, actor, AuditResource{Type: "sample_reference", ID: "configure"}, nil); err != nil {
			return err
		}
		existing, err := sampleReferenceItemByIDTx(tx, scope, id)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		item = SampleReferenceItem{ID: existing.ID, TenantID: scope.TenantID, LabID: scope.LabID, Kind: input.Kind, Name: input.Name, Code: input.Code, Description: input.Description, SortOrder: input.SortOrder, Active: input.Active, CreatedAt: existing.CreatedAt, UpdatedAt: now}
		if _, err := tx.Exec(`UPDATE sample_reference_items SET kind = ?, name = ?, code = ?, description = ?, sort_order = ?, active = ?, updated_at = ? WHERE id = ? AND tenant_id = ? AND lab_id = ?`, string(item.Kind), item.Name, item.Code, item.Description, item.SortOrder, boolInt(item.Active), formatTime(item.UpdatedAt), item.ID, scope.TenantID, scope.LabID); err != nil {
			return err
		}
		return appendAuditTx(tx, sampleReferenceAudit(scope, actor, "sample_reference.updated", item, map[string]any{"name": item.Name, "kind": string(item.Kind), "code": item.Code}))
	})
	return item, err
}

func (s *Store) DeleteSampleReferenceItem(id string, actor ActorContext) error {
	return s.DeleteSampleReferenceItemForScope(defaultScope(), id, actor)
}

func (s *Store) DeleteSampleReferenceItemForScope(scope Scope, id string, actor ActorContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("sample reference id is required")
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := requireAuthorizedOperationTx(tx, scope, OperationCatalogConfigure, actor, AuditResource{Type: "sample_reference", ID: "configure"}, nil); err != nil {
			return err
		}
		item, err := sampleReferenceItemByIDTx(tx, scope, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM sample_reference_items WHERE id = ? AND tenant_id = ? AND lab_id = ?`, id, scope.TenantID, scope.LabID); err != nil {
			return err
		}
		return appendAuditTx(tx, sampleReferenceAudit(scope, actor, "sample_reference.deleted", item, map[string]any{"name": item.Name, "kind": string(item.Kind), "code": item.Code}))
	})
}

func (s *Store) SampleReferenceItems(kind SampleReferenceKind) []SampleReferenceItem {
	return s.SampleReferenceItemsForScope(defaultScope(), kind)
}

func (s *Store) SampleReferenceItemsForScope(scope Scope, kind SampleReferenceKind) []SampleReferenceItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	kind, err = normalizeSampleReferenceKind(kind)
	if err != nil {
		return nil
	}
	items, err := sampleReferenceItemsQuery(s.db, scope, `AND kind = ? AND active = 1`, []any{string(kind)})
	if err != nil {
		return nil
	}
	return items
}

func (s *Store) AllSampleReferenceItems() []SampleReferenceItem {
	return s.AllSampleReferenceItemsForScope(defaultScope())
}

func (s *Store) AllSampleReferenceItemsForScope(scope Scope) []SampleReferenceItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	items, err := sampleReferenceItemsQuery(s.db, scope, `AND active = 1`, nil)
	if err != nil {
		return nil
	}
	return items
}

func (s *Store) SeedDemoSampleReferenceData(actor ActorContext) (SampleReferenceSeedSummary, error) {
	return s.SeedDemoSampleReferenceDataForScope(defaultScope(), actor)
}

func (s *Store) SeedDemoSampleReferenceDataForScope(scope Scope, actor ActorContext) (SampleReferenceSeedSummary, error) {
	defs := demoSampleReferenceDefinitions()
	for _, def := range defs {
		if err := s.seedSampleReferenceItem(scope, def, actor); err != nil {
			return SampleReferenceSeedSummary{}, err
		}
	}
	return s.sampleReferenceSeedSummary(scope), nil
}

func (s *Store) seedSampleReferenceItem(scope Scope, input SampleReferenceItemInput, actor ActorContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return err
	}
	input, err = normalizeSampleReferenceInput(input)
	if err != nil {
		return err
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := requireAuthorizedOperationTx(tx, scope, OperationCatalogConfigure, actor, AuditResource{Type: "sample_reference", ID: "configure"}, nil); err != nil {
			return err
		}
		var existingID string
		err := tx.QueryRow(`SELECT id FROM sample_reference_items WHERE tenant_id = ? AND lab_id = ? AND kind = ? AND code = ?`, scope.TenantID, scope.LabID, string(input.Kind), input.Code).Scan(&existingID)
		if err == nil {
			_, err = tx.Exec(`UPDATE sample_reference_items SET name = ?, description = ?, sort_order = ?, active = 1, updated_at = ? WHERE id = ?`, input.Name, input.Description, input.SortOrder, formatTime(time.Now().UTC()), existingID)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		next, err := nextCounter(tx, "next_sample_reference")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		item := SampleReferenceItem{ID: fmt.Sprintf("SR-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Kind: input.Kind, Name: input.Name, Code: input.Code, Description: input.Description, SortOrder: input.SortOrder, Active: true, CreatedAt: now, UpdatedAt: now}
		if _, err := tx.Exec(`INSERT INTO sample_reference_items(id, tenant_id, lab_id, kind, name, code, description, sort_order, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.TenantID, item.LabID, string(item.Kind), item.Name, item.Code, item.Description, item.SortOrder, 1, formatTime(item.CreatedAt), formatTime(item.UpdatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, sampleReferenceAudit(scope, actor, "sample_reference.seeded", item, map[string]any{"name": item.Name, "kind": string(item.Kind), "code": item.Code}))
	})
}

func (s *Store) sampleReferenceSeedSummary(scope Scope) SampleReferenceSeedSummary {
	counts := map[SampleReferenceKind]int{}
	for _, kind := range sampleReferenceKinds() {
		counts[kind] = len(s.SampleReferenceItemsForScope(scope, kind))
	}
	summary := SampleReferenceSeedSummary{MatrixCount: counts[SampleReferenceMatrix], ContainerCount: counts[SampleReferenceContainer], PreservativeCount: counts[SampleReferencePreservative], StorageLocationCount: counts[SampleReferenceStorageLocation], ReceivedConditionCount: counts[SampleReferenceReceivedCondition]}
	summary.TotalCount = summary.MatrixCount + summary.ContainerCount + summary.PreservativeCount + summary.StorageLocationCount + summary.ReceivedConditionCount
	return summary
}

func demoSampleReferenceDefinitions() []SampleReferenceItemInput {
	return []SampleReferenceItemInput{
		{Kind: SampleReferenceMatrix, Name: "Drinking Water", Code: "DW", Description: "Potable water samples", SortOrder: 10, Active: true},
		{Kind: SampleReferenceMatrix, Name: "Wastewater", Code: "WW", Description: "Municipal or industrial wastewater", SortOrder: 20, Active: true},
		{Kind: SampleReferenceMatrix, Name: "Soil", Code: "SOIL", Description: "Soil and sediment samples", SortOrder: 30, Active: true},
		{Kind: SampleReferenceContainer, Name: "500 mL HDPE Bottle", Code: "HDPE-500", Description: "Routine inorganic water container", SortOrder: 10, Active: true},
		{Kind: SampleReferenceContainer, Name: "40 mL VOA Vial", Code: "VOA-40", Description: "Volatile organic analysis vial", SortOrder: 20, Active: true},
		{Kind: SampleReferenceContainer, Name: "Glass Jar", Code: "GLASS-JAR", Description: "Soil/sediment glass jar", SortOrder: 30, Active: true},
		{Kind: SampleReferencePreservative, Name: "None", Code: "NONE", Description: "No chemical preservative", SortOrder: 10, Active: true},
		{Kind: SampleReferencePreservative, Name: "HNO3", Code: "HNO3", Description: "Nitric acid preserved", SortOrder: 20, Active: true},
		{Kind: SampleReferencePreservative, Name: "HCl", Code: "HCL", Description: "Hydrochloric acid preserved", SortOrder: 30, Active: true},
		{Kind: SampleReferenceStorageLocation, Name: "Walk-in Cooler", Code: "COOLER", Description: "Refrigerated sample storage", SortOrder: 10, Active: true},
		{Kind: SampleReferenceStorageLocation, Name: "Ambient Receiving Shelf", Code: "AMBIENT", Description: "Room-temperature receiving hold", SortOrder: 20, Active: true},
		{Kind: SampleReferenceReceivedCondition, Name: "Acceptable", Code: "OK", Description: "Received acceptable for analysis", SortOrder: 10, Active: true},
		{Kind: SampleReferenceReceivedCondition, Name: "Broken Container", Code: "BROKEN", Description: "Container broken or leaking", SortOrder: 20, Active: true},
		{Kind: SampleReferenceReceivedCondition, Name: "Insufficient Volume", Code: "LOW-VOLUME", Description: "Not enough sample volume received", SortOrder: 30, Active: true},
		{Kind: SampleReferenceReceivedCondition, Name: "Temperature Out of Range", Code: "TEMP-OOR", Description: "Receipt temperature outside method criteria", SortOrder: 40, Active: true},
	}
}

func normalizeSampleReferenceInput(input SampleReferenceItemInput) (SampleReferenceItemInput, error) {
	kind, err := normalizeSampleReferenceKind(input.Kind)
	if err != nil {
		return SampleReferenceItemInput{}, err
	}
	input.Kind = kind
	input.Name = strings.TrimSpace(input.Name)
	input.Code = strings.TrimSpace(input.Code)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" {
		return SampleReferenceItemInput{}, errors.New("sample reference name is required")
	}
	if input.Code == "" {
		input.Code = sampleReferenceCode(input.Name)
	}
	return input, nil
}

func normalizeSampleReferenceKind(kind SampleReferenceKind) (SampleReferenceKind, error) {
	switch SampleReferenceKind(strings.TrimSpace(string(kind))) {
	case SampleReferenceMatrix:
		return SampleReferenceMatrix, nil
	case SampleReferenceContainer:
		return SampleReferenceContainer, nil
	case SampleReferencePreservative:
		return SampleReferencePreservative, nil
	case SampleReferenceStorageLocation:
		return SampleReferenceStorageLocation, nil
	case SampleReferenceReceivedCondition:
		return SampleReferenceReceivedCondition, nil
	default:
		return "", fmt.Errorf("unknown sample reference kind %q", kind)
	}
}

func sampleReferenceKinds() []SampleReferenceKind {
	return []SampleReferenceKind{SampleReferenceMatrix, SampleReferenceContainer, SampleReferencePreservative, SampleReferenceStorageLocation, SampleReferenceReceivedCondition}
}

func sampleReferenceCode(name string) string {
	code := strings.ToUpper(strings.TrimSpace(name))
	code = strings.ReplaceAll(code, " ", "-")
	return code
}

func sampleReferenceAudit(scope Scope, actor ActorContext, action string, item SampleReferenceItem, details map[string]any) auditWrite {
	return auditWrite{Scope: scope, Actor: actor, Action: action, Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample_reference", ID: item.ID}, Details: details}
}

func sampleReferenceItemByIDTx(q interface{ QueryRow(string, ...any) *sql.Row }, scope Scope, id string) (SampleReferenceItem, error) {
	return scanSampleReferenceItem(q.QueryRow(`SELECT id, tenant_id, lab_id, kind, name, code, description, sort_order, active, created_at, updated_at FROM sample_reference_items WHERE id = ? AND tenant_id = ? AND lab_id = ?`, id, scope.TenantID, scope.LabID))
}

func sampleReferenceItemsQuery(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope, extraWhere string, extraArgs []any) ([]SampleReferenceItem, error) {
	args := append([]any{scope.TenantID, scope.LabID}, extraArgs...)
	rows, err := q.Query(`SELECT id, tenant_id, lab_id, kind, name, code, description, sort_order, active, created_at, updated_at FROM sample_reference_items WHERE tenant_id = ? AND lab_id = ? `+extraWhere+` ORDER BY kind, sort_order, name, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SampleReferenceItem{}
	for rows.Next() {
		item, err := scanSampleReferenceItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type sampleReferenceScanner interface{ Scan(dest ...any) error }

func scanSampleReferenceItem(row sampleReferenceScanner) (SampleReferenceItem, error) {
	var item SampleReferenceItem
	var kind, created, updated string
	var active int
	if err := row.Scan(&item.ID, &item.TenantID, &item.LabID, &kind, &item.Name, &item.Code, &item.Description, &item.SortOrder, &active, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SampleReferenceItem{}, fmt.Errorf("unknown sample reference item")
		}
		return SampleReferenceItem{}, err
	}
	item.Kind = SampleReferenceKind(kind)
	item.Active = active == 1
	item.CreatedAt, _ = parseTime(created)
	item.UpdatedAt, _ = parseTime(updated)
	return item, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

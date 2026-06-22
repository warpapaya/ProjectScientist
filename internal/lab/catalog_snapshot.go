package lab

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CatalogSnapshot struct {
	ID          string              `json:"id"`
	TenantID    string              `json:"tenant_id"`
	LabID       string              `json:"lab_id"`
	Version     int                 `json:"version"`
	ContentHash string              `json:"content_hash"`
	Departments []CatalogDepartment `json:"departments"`
	Units       []CatalogUnit       `json:"units"`
	Methods     []CatalogMethod     `json:"methods"`
	Analytes    []CatalogAnalyte    `json:"analytes"`
	Services    []AnalysisService   `json:"services"`
	Profiles    []AnalysisProfile   `json:"profiles"`
	CreatedAt   time.Time           `json:"created_at"`
}

type catalogSnapshotData struct {
	Departments []CatalogDepartment `json:"departments"`
	Units       []CatalogUnit       `json:"units"`
	Methods     []CatalogMethod     `json:"methods"`
	Analytes    []CatalogAnalyte    `json:"analytes"`
	Services    []AnalysisService   `json:"services"`
	Profiles    []AnalysisProfile   `json:"profiles"`
}

func (s *Store) CatalogSnapshots() []CatalogSnapshot {
	return s.CatalogSnapshotsForScope(defaultScope())
}

func (s *Store) CatalogSnapshotsForScope(scope Scope) []CatalogSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, lab_id, version, content_hash, data_json, created_at FROM catalog_snapshots WHERE tenant_id = ? AND lab_id = ? ORDER BY version, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	snapshots := []CatalogSnapshot{}
	for rows.Next() {
		snapshot, err := scanCatalogSnapshot(rows)
		if err != nil {
			return nil
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *Store) CatalogSnapshot(id string) (CatalogSnapshot, bool) {
	return s.CatalogSnapshotForScope(defaultScope(), id)
}

func (s *Store) CatalogSnapshotForScope(scope Scope, id string) (CatalogSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return CatalogSnapshot{}, false
	}
	snapshot, err := catalogSnapshotByIDTx(s.db, scope, id)
	if err != nil {
		return CatalogSnapshot{}, false
	}
	return snapshot, true
}

func (s *Store) CurrentCatalogSnapshot() (CatalogSnapshot, bool) {
	return s.CurrentCatalogSnapshotForScope(defaultScope())
}

func (s *Store) CurrentCatalogSnapshotForScope(scope Scope) (CatalogSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return CatalogSnapshot{}, false
	}
	snapshot, ok, err := currentCatalogSnapshotTx(s.db, scope)
	if err != nil || !ok {
		return CatalogSnapshot{}, false
	}
	return snapshot, true
}

func createCatalogSnapshotTx(tx *sql.Tx, scope Scope, actor ActorContext, reason string) (CatalogSnapshot, error) {
	data, err := catalogSnapshotDataTx(tx, scope)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	next, err := nextCounter(tx, "next_catalog_snapshot")
	if err != nil {
		return CatalogSnapshot{}, err
	}
	now := time.Now().UTC()
	sum := sha256.Sum256(payload)
	snapshot := CatalogSnapshot{ID: fmt.Sprintf("CS-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Version: next, ContentHash: hex.EncodeToString(sum[:]), Departments: data.Departments, Units: data.Units, Methods: data.Methods, Analytes: data.Analytes, Services: data.Services, Profiles: data.Profiles, CreatedAt: now}
	if _, err := tx.Exec(`INSERT INTO catalog_snapshots(id, tenant_id, lab_id, version, content_hash, data_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, snapshot.ID, snapshot.TenantID, snapshot.LabID, snapshot.Version, snapshot.ContentHash, string(payload), formatTime(snapshot.CreatedAt)); err != nil {
		return CatalogSnapshot{}, err
	}
	if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "catalog.snapshot.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "catalog_snapshot", ID: snapshot.ID, Version: fmt.Sprintf("%d", snapshot.Version)}, Details: map[string]any{"reason": strings.TrimSpace(reason), "content_hash": snapshot.ContentHash}}); err != nil {
		return CatalogSnapshot{}, err
	}
	return snapshot, nil
}

func catalogSnapshotDataTx(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope) (catalogSnapshotData, error) {
	departments, err := catalogDepartmentsQuery(q, scope)
	if err != nil {
		return catalogSnapshotData{}, err
	}
	units, err := catalogUnitsQuery(q, scope)
	if err != nil {
		return catalogSnapshotData{}, err
	}
	methods, err := catalogMethodsQuery(q, scope)
	if err != nil {
		return catalogSnapshotData{}, err
	}
	analytes, err := catalogAnalytesQuery(q, scope)
	if err != nil {
		return catalogSnapshotData{}, err
	}
	services, err := analysisServicesQuery(q, scope, "", nil)
	if err != nil {
		return catalogSnapshotData{}, err
	}
	profiles, err := analysisProfilesQuery(q, scope)
	if err != nil {
		return catalogSnapshotData{}, err
	}
	return catalogSnapshotData{Departments: departments, Units: units, Methods: methods, Analytes: analytes, Services: services, Profiles: profiles}, nil
}

func catalogDepartmentsQuery(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope) ([]CatalogDepartment, error) {
	rows, err := q.Query(`SELECT id, tenant_id, lab_id, name, sort_order, created_at FROM catalog_departments WHERE tenant_id = ? AND lab_id = ? ORDER BY sort_order, name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CatalogDepartment{}
	for rows.Next() {
		var item CatalogDepartment
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.Name, &item.SortOrder, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items, rows.Err()
}

func catalogUnitsQuery(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope) ([]CatalogUnit, error) {
	rows, err := q.Query(`SELECT id, tenant_id, lab_id, name, symbol, created_at FROM catalog_units WHERE tenant_id = ? AND lab_id = ? ORDER BY symbol, name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CatalogUnit{}
	for rows.Next() {
		var item CatalogUnit
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.Name, &item.Symbol, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items, rows.Err()
}

func catalogMethodsQuery(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope) ([]CatalogMethod, error) {
	rows, err := q.Query(`SELECT id, tenant_id, lab_id, name, description, created_at FROM catalog_methods WHERE tenant_id = ? AND lab_id = ? ORDER BY name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CatalogMethod{}
	for rows.Next() {
		var item CatalogMethod
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.Name, &item.Description, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items, rows.Err()
}

func catalogAnalytesQuery(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope) ([]CatalogAnalyte, error) {
	rows, err := q.Query(`SELECT id, tenant_id, lab_id, name, default_unit_id, created_at FROM catalog_analytes WHERE tenant_id = ? AND lab_id = ? ORDER BY name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CatalogAnalyte{}
	for rows.Next() {
		var item CatalogAnalyte
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.Name, &item.DefaultUnitID, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items, rows.Err()
}

func analysisProfilesQuery(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope) ([]AnalysisProfile, error) {
	rows, err := q.Query(`SELECT id, tenant_id, lab_id, name, created_at FROM analysis_profiles WHERE tenant_id = ? AND lab_id = ? ORDER BY name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []AnalysisProfile{}
	for rows.Next() {
		var profile AnalysisProfile
		var created string
		if err := rows.Scan(&profile.ID, &profile.TenantID, &profile.LabID, &profile.Name, &created); err != nil {
			return nil, err
		}
		profile.CreatedAt, _ = parseTime(created)
		services, err := profileServicesTx(q, scope, profile.ID)
		if err != nil {
			return nil, err
		}
		profile.Services = services
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

type catalogSnapshotScanner interface{ Scan(dest ...any) error }

func scanCatalogSnapshot(row catalogSnapshotScanner) (CatalogSnapshot, error) {
	var snapshot CatalogSnapshot
	var dataJSON, created string
	if err := row.Scan(&snapshot.ID, &snapshot.TenantID, &snapshot.LabID, &snapshot.Version, &snapshot.ContentHash, &dataJSON, &created); err != nil {
		return CatalogSnapshot{}, err
	}
	var data catalogSnapshotData
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		return CatalogSnapshot{}, err
	}
	snapshot.Departments = data.Departments
	snapshot.Units = data.Units
	snapshot.Methods = data.Methods
	snapshot.Analytes = data.Analytes
	snapshot.Services = data.Services
	snapshot.Profiles = data.Profiles
	snapshot.CreatedAt, _ = parseTime(created)
	return snapshot, nil
}

func catalogSnapshotByIDTx(q interface {
	QueryRow(string, ...any) *sql.Row
}, scope Scope, id string) (CatalogSnapshot, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return CatalogSnapshot{}, sql.ErrNoRows
	}
	return scanCatalogSnapshot(q.QueryRow(`SELECT id, tenant_id, lab_id, version, content_hash, data_json, created_at FROM catalog_snapshots WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, id))
}

func currentCatalogSnapshotTx(q interface {
	QueryRow(string, ...any) *sql.Row
}, scope Scope) (CatalogSnapshot, bool, error) {
	snapshot, err := scanCatalogSnapshot(q.QueryRow(`SELECT id, tenant_id, lab_id, version, content_hash, data_json, created_at FROM catalog_snapshots WHERE tenant_id = ? AND lab_id = ? ORDER BY version DESC, id DESC LIMIT 1`, scope.TenantID, scope.LabID))
	if errors.Is(err, sql.ErrNoRows) {
		return CatalogSnapshot{}, false, nil
	}
	if err != nil {
		return CatalogSnapshot{}, false, err
	}
	return snapshot, true, nil
}

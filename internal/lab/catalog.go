package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CatalogDepartment struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	LabID     string    `json:"lab_id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

type CatalogUnit struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	LabID     string    `json:"lab_id"`
	Name      string    `json:"name"`
	Symbol    string    `json:"symbol"`
	CreatedAt time.Time `json:"created_at"`
}

type CatalogMethod struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	LabID       string    `json:"lab_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CatalogAnalyte struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	LabID         string    `json:"lab_id"`
	Name          string    `json:"name"`
	DefaultUnitID string    `json:"default_unit_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type AnalysisService struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	LabID          string    `json:"lab_id"`
	Name           string    `json:"name"`
	DepartmentID   string    `json:"department_id"`
	DepartmentName string    `json:"department_name"`
	MethodID       string    `json:"method_id,omitempty"`
	MethodName     string    `json:"method_name,omitempty"`
	AnalyteID      string    `json:"analyte_id,omitempty"`
	AnalyteName    string    `json:"analyte_name,omitempty"`
	UnitID         string    `json:"unit_id,omitempty"`
	UnitSymbol     string    `json:"unit_symbol,omitempty"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
}

type AnalysisProfile struct {
	ID        string            `json:"id"`
	TenantID  string            `json:"tenant_id"`
	LabID     string            `json:"lab_id"`
	Name      string            `json:"name"`
	Services  []AnalysisService `json:"services"`
	CreatedAt time.Time         `json:"created_at"`
}

type CatalogDepartmentInput struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type CatalogUnitInput struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}

type CatalogUnitUpdateInput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}

type CatalogMethodInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CatalogAnalyteInput struct {
	Name          string `json:"name"`
	DefaultUnitID string `json:"default_unit_id"`
}

type AnalysisServiceInput struct {
	Name         string `json:"name"`
	DepartmentID string `json:"department_id"`
	MethodID     string `json:"method_id"`
	AnalyteID    string `json:"analyte_id"`
	UnitID       string `json:"unit_id"`
	SortOrder    int    `json:"sort_order"`
}

type AnalysisProfileInput struct {
	Name       string   `json:"name"`
	ServiceIDs []string `json:"service_ids"`
}

func (s *Store) CreateCatalogDepartment(input CatalogDepartmentInput, actor ActorContext) (CatalogDepartment, error) {
	return s.CreateCatalogDepartmentForScope(defaultScope(), input, actor)
}

func (s *Store) CreateCatalogDepartmentForScope(scope Scope, input CatalogDepartmentInput, actor ActorContext) (CatalogDepartment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return CatalogDepartment{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CatalogDepartment{}, errors.New("department name is required")
	}
	var department CatalogDepartment
	err = s.withTx(func(tx *sql.Tx) error {
		if err := Authorize(scope, OperationCatalogConfigure, actor); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_catalog_department")
		if err != nil {
			return err
		}
		department = CatalogDepartment{ID: fmt.Sprintf("D-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: name, SortOrder: input.SortOrder, CreatedAt: time.Now().UTC()}
		if _, err := tx.Exec(`INSERT INTO catalog_departments(id, tenant_id, lab_id, name, sort_order, created_at) VALUES (?, ?, ?, ?, ?, ?)`, department.ID, department.TenantID, department.LabID, department.Name, department.SortOrder, formatTime(department.CreatedAt)); err != nil {
			return err
		}
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: string(OperationCatalogConfigure), Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "catalog_department", ID: department.ID}, Details: map[string]any{"name": department.Name}}); err != nil {
			return err
		}
		_, err = createCatalogSnapshotTx(tx, scope, actor, "catalog_department.created")
		return err
	})
	return department, err
}

func (s *Store) CreateCatalogUnit(input CatalogUnitInput, actor ActorContext) (CatalogUnit, error) {
	return s.CreateCatalogUnitForScope(defaultScope(), input, actor)
}

func (s *Store) UpdateCatalogUnit(input CatalogUnitUpdateInput, actor ActorContext) (CatalogUnit, error) {
	return s.UpdateCatalogUnitForScope(defaultScope(), input, actor)
}

func (s *Store) UpdateCatalogUnitForScope(scope Scope, input CatalogUnitUpdateInput, actor ActorContext) (CatalogUnit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return CatalogUnit{}, err
	}
	id := strings.TrimSpace(input.ID)
	name, symbol := strings.TrimSpace(input.Name), strings.TrimSpace(input.Symbol)
	if id == "" {
		return CatalogUnit{}, errors.New("unit id is required")
	}
	if name == "" || symbol == "" {
		return CatalogUnit{}, errors.New("unit name and symbol are required")
	}
	var unit CatalogUnit
	err = s.withTx(func(tx *sql.Tx) error {
		if err := Authorize(scope, OperationCatalogConfigure, actor); err != nil {
			return err
		}
		var created string
		if err := tx.QueryRow(`SELECT id, tenant_id, lab_id, name, symbol, created_at FROM catalog_units WHERE id = ? AND tenant_id = ? AND lab_id = ?`, id, scope.TenantID, scope.LabID).Scan(&unit.ID, &unit.TenantID, &unit.LabID, &unit.Name, &unit.Symbol, &created); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown unit %q", id)
			}
			return err
		}
		if _, err := tx.Exec(`UPDATE catalog_units SET name = ?, symbol = ? WHERE id = ? AND tenant_id = ? AND lab_id = ?`, name, symbol, id, scope.TenantID, scope.LabID); err != nil {
			return err
		}
		unit.Name = name
		unit.Symbol = symbol
		unit.CreatedAt, _ = parseTime(created)
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: string(OperationCatalogConfigure), Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "catalog_unit", ID: unit.ID}, Details: map[string]any{"symbol": unit.Symbol, "updated": true}}); err != nil {
			return err
		}
		_, err = createCatalogSnapshotTx(tx, scope, actor, "catalog_unit.updated")
		return err
	})
	return unit, err
}

func (s *Store) CreateCatalogUnitForScope(scope Scope, input CatalogUnitInput, actor ActorContext) (CatalogUnit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return CatalogUnit{}, err
	}
	name, symbol := strings.TrimSpace(input.Name), strings.TrimSpace(input.Symbol)
	if name == "" || symbol == "" {
		return CatalogUnit{}, errors.New("unit name and symbol are required")
	}
	var unit CatalogUnit
	err = s.withTx(func(tx *sql.Tx) error {
		if err := Authorize(scope, OperationCatalogConfigure, actor); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_catalog_unit")
		if err != nil {
			return err
		}
		unit = CatalogUnit{ID: fmt.Sprintf("U-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: name, Symbol: symbol, CreatedAt: time.Now().UTC()}
		if _, err := tx.Exec(`INSERT INTO catalog_units(id, tenant_id, lab_id, name, symbol, created_at) VALUES (?, ?, ?, ?, ?, ?)`, unit.ID, unit.TenantID, unit.LabID, unit.Name, unit.Symbol, formatTime(unit.CreatedAt)); err != nil {
			return err
		}
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: string(OperationCatalogConfigure), Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "catalog_unit", ID: unit.ID}, Details: map[string]any{"symbol": unit.Symbol}}); err != nil {
			return err
		}
		_, err = createCatalogSnapshotTx(tx, scope, actor, "catalog_unit.created")
		return err
	})
	return unit, err
}

func (s *Store) CreateCatalogMethod(input CatalogMethodInput, actor ActorContext) (CatalogMethod, error) {
	return s.CreateCatalogMethodForScope(defaultScope(), input, actor)
}

func (s *Store) CreateCatalogMethodForScope(scope Scope, input CatalogMethodInput, actor ActorContext) (CatalogMethod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return CatalogMethod{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CatalogMethod{}, errors.New("method name is required")
	}
	var method CatalogMethod
	err = s.withTx(func(tx *sql.Tx) error {
		if err := Authorize(scope, OperationCatalogConfigure, actor); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_catalog_method")
		if err != nil {
			return err
		}
		method = CatalogMethod{ID: fmt.Sprintf("M-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: name, Description: strings.TrimSpace(input.Description), CreatedAt: time.Now().UTC()}
		if _, err := tx.Exec(`INSERT INTO catalog_methods(id, tenant_id, lab_id, name, description, created_at) VALUES (?, ?, ?, ?, ?, ?)`, method.ID, method.TenantID, method.LabID, method.Name, method.Description, formatTime(method.CreatedAt)); err != nil {
			return err
		}
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: string(OperationCatalogConfigure), Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "catalog_method", ID: method.ID}, Details: map[string]any{"name": method.Name}}); err != nil {
			return err
		}
		_, err = createCatalogSnapshotTx(tx, scope, actor, "catalog_method.created")
		return err
	})
	return method, err
}

func (s *Store) CreateCatalogAnalyte(input CatalogAnalyteInput, actor ActorContext) (CatalogAnalyte, error) {
	return s.CreateCatalogAnalyteForScope(defaultScope(), input, actor)
}

func (s *Store) CreateCatalogAnalyteForScope(scope Scope, input CatalogAnalyteInput, actor ActorContext) (CatalogAnalyte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return CatalogAnalyte{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CatalogAnalyte{}, errors.New("analyte name is required")
	}
	var analyte CatalogAnalyte
	err = s.withTx(func(tx *sql.Tx) error {
		if err := Authorize(scope, OperationCatalogConfigure, actor); err != nil {
			return err
		}
		if err := requireOptionalScopedID(tx, "catalog_units", scope, input.DefaultUnitID); err != nil {
			return fmt.Errorf("default unit: %w", err)
		}
		next, err := nextCounter(tx, "next_catalog_analyte")
		if err != nil {
			return err
		}
		analyte = CatalogAnalyte{ID: fmt.Sprintf("A-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: name, DefaultUnitID: strings.TrimSpace(input.DefaultUnitID), CreatedAt: time.Now().UTC()}
		if _, err := tx.Exec(`INSERT INTO catalog_analytes(id, tenant_id, lab_id, name, default_unit_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`, analyte.ID, analyte.TenantID, analyte.LabID, analyte.Name, analyte.DefaultUnitID, formatTime(analyte.CreatedAt)); err != nil {
			return err
		}
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: string(OperationCatalogConfigure), Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "catalog_analyte", ID: analyte.ID}, Details: map[string]any{"name": analyte.Name}}); err != nil {
			return err
		}
		_, err = createCatalogSnapshotTx(tx, scope, actor, "catalog_analyte.created")
		return err
	})
	return analyte, err
}

func (s *Store) CreateAnalysisService(input AnalysisServiceInput, actor ActorContext) (AnalysisService, error) {
	return s.CreateAnalysisServiceForScope(defaultScope(), input, actor)
}

func (s *Store) CreateAnalysisServiceForScope(scope Scope, input AnalysisServiceInput, actor ActorContext) (AnalysisService, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return AnalysisService{}, err
	}
	name := strings.TrimSpace(input.Name)
	departmentID := strings.TrimSpace(input.DepartmentID)
	if name == "" || departmentID == "" {
		return AnalysisService{}, errors.New("service name and department are required")
	}
	var service AnalysisService
	err = s.withTx(func(tx *sql.Tx) error {
		if err := Authorize(scope, OperationCatalogConfigure, actor); err != nil {
			return err
		}
		for table, id := range map[string]string{"catalog_departments": departmentID, "catalog_methods": input.MethodID, "catalog_analytes": input.AnalyteID, "catalog_units": input.UnitID} {
			if table == "catalog_departments" {
				if err := requireScopedID(tx, table, scope, id); err != nil {
					return fmt.Errorf("department: %w", err)
				}
				continue
			}
			if err := requireOptionalScopedID(tx, table, scope, id); err != nil {
				return fmt.Errorf("%s: %w", table, err)
			}
		}
		next, err := nextCounter(tx, "next_analysis_service")
		if err != nil {
			return err
		}
		service = AnalysisService{ID: fmt.Sprintf("AS-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: name, DepartmentID: departmentID, MethodID: strings.TrimSpace(input.MethodID), AnalyteID: strings.TrimSpace(input.AnalyteID), UnitID: strings.TrimSpace(input.UnitID), SortOrder: input.SortOrder, CreatedAt: time.Now().UTC()}
		if _, err := tx.Exec(`INSERT INTO analysis_services(id, tenant_id, lab_id, name, department_id, method_id, analyte_id, unit_id, sort_order, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, service.ID, service.TenantID, service.LabID, service.Name, service.DepartmentID, service.MethodID, service.AnalyteID, service.UnitID, service.SortOrder, formatTime(service.CreatedAt)); err != nil {
			return err
		}
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: string(OperationCatalogConfigure), Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "analysis_service", ID: service.ID}, Details: map[string]any{"name": service.Name}}); err != nil {
			return err
		}
		loaded, err := analysisServiceByIDTx(tx, scope, service.ID)
		if err != nil {
			return err
		}
		service = loaded
		_, err = createCatalogSnapshotTx(tx, scope, actor, "analysis_service.created")
		return err
	})
	return service, err
}

func (s *Store) CreateAnalysisProfile(input AnalysisProfileInput, actor ActorContext) (AnalysisProfile, error) {
	return s.CreateAnalysisProfileForScope(defaultScope(), input, actor)
}

func (s *Store) CreateAnalysisProfileForScope(scope Scope, input AnalysisProfileInput, actor ActorContext) (AnalysisProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return AnalysisProfile{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len(input.ServiceIDs) == 0 {
		return AnalysisProfile{}, errors.New("profile name and at least one service are required")
	}
	var profile AnalysisProfile
	err = s.withTx(func(tx *sql.Tx) error {
		if err := Authorize(scope, OperationCatalogConfigure, actor); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_analysis_profile")
		if err != nil {
			return err
		}
		profile = AnalysisProfile{ID: fmt.Sprintf("P-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: name, CreatedAt: time.Now().UTC()}
		if _, err := tx.Exec(`INSERT INTO analysis_profiles(id, tenant_id, lab_id, name, created_at) VALUES (?, ?, ?, ?, ?)`, profile.ID, profile.TenantID, profile.LabID, profile.Name, formatTime(profile.CreatedAt)); err != nil {
			return err
		}
		for i, rawID := range input.ServiceIDs {
			serviceID := strings.TrimSpace(rawID)
			if serviceID == "" {
				continue
			}
			if err := requireScopedID(tx, "analysis_services", scope, serviceID); err != nil {
				return fmt.Errorf("service %q: %w", serviceID, err)
			}
			if _, err := tx.Exec(`INSERT INTO analysis_profile_services(profile_id, service_id, sort_order) VALUES (?, ?, ?)`, profile.ID, serviceID, i+1); err != nil {
				return err
			}
		}
		services, err := profileServicesTx(tx, scope, profile.ID)
		if err != nil {
			return err
		}
		if len(services) == 0 {
			return errors.New("profile requires at least one non-empty service")
		}
		profile.Services = services
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: string(OperationCatalogConfigure), Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "analysis_profile", ID: profile.ID}, Details: map[string]any{"name": profile.Name, "service_count": len(profile.Services)}}); err != nil {
			return err
		}
		_, err = createCatalogSnapshotTx(tx, scope, actor, "analysis_profile.created")
		return err
	})
	return profile, err
}

func (s *Store) CatalogDepartments() []CatalogDepartment {
	return s.CatalogDepartmentsForScope(defaultScope())
}
func (s *Store) CatalogUnits() []CatalogUnit       { return s.CatalogUnitsForScope(defaultScope()) }
func (s *Store) CatalogMethods() []CatalogMethod   { return s.CatalogMethodsForScope(defaultScope()) }
func (s *Store) CatalogAnalytes() []CatalogAnalyte { return s.CatalogAnalytesForScope(defaultScope()) }
func (s *Store) AnalysisServices() []AnalysisService {
	return s.AnalysisServicesForScope(defaultScope())
}
func (s *Store) AnalysisProfiles() []AnalysisProfile {
	return s.AnalysisProfilesForScope(defaultScope())
}

func (s *Store) CatalogDepartmentsForScope(scope Scope) []CatalogDepartment {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, lab_id, name, sort_order, created_at FROM catalog_departments WHERE tenant_id = ? AND lab_id = ? ORDER BY sort_order, name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	items := []CatalogDepartment{}
	for rows.Next() {
		var item CatalogDepartment
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.Name, &item.SortOrder, &created); err != nil {
			return nil
		}
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items
}

func (s *Store) CatalogUnitsForScope(scope Scope) []CatalogUnit {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, lab_id, name, symbol, created_at FROM catalog_units WHERE tenant_id = ? AND lab_id = ? ORDER BY symbol, name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	items := []CatalogUnit{}
	for rows.Next() {
		var item CatalogUnit
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.Name, &item.Symbol, &created); err != nil {
			return nil
		}
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items
}

func (s *Store) CatalogMethodsForScope(scope Scope) []CatalogMethod {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, lab_id, name, description, created_at FROM catalog_methods WHERE tenant_id = ? AND lab_id = ? ORDER BY name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	items := []CatalogMethod{}
	for rows.Next() {
		var item CatalogMethod
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.Name, &item.Description, &created); err != nil {
			return nil
		}
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items
}

func (s *Store) CatalogAnalytesForScope(scope Scope) []CatalogAnalyte {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, lab_id, name, default_unit_id, created_at FROM catalog_analytes WHERE tenant_id = ? AND lab_id = ? ORDER BY name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	items := []CatalogAnalyte{}
	for rows.Next() {
		var item CatalogAnalyte
		var created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.Name, &item.DefaultUnitID, &created); err != nil {
			return nil
		}
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items
}

func (s *Store) AnalysisServicesForScope(scope Scope) []AnalysisService {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	services, err := analysisServicesQuery(s.db, scope, "", nil)
	if err != nil {
		return nil
	}
	return services
}

func (s *Store) AnalysisProfilesForScope(scope Scope) []AnalysisProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, lab_id, name, created_at FROM analysis_profiles WHERE tenant_id = ? AND lab_id = ? ORDER BY name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	profiles := []AnalysisProfile{}
	for rows.Next() {
		var profile AnalysisProfile
		var created string
		if err := rows.Scan(&profile.ID, &profile.TenantID, &profile.LabID, &profile.Name, &created); err != nil {
			return nil
		}
		profile.CreatedAt, _ = parseTime(created)
		services, err := profileServicesTx(s.db, scope, profile.ID)
		if err != nil {
			return nil
		}
		profile.Services = services
		profiles = append(profiles, profile)
	}
	return profiles
}

func requireOptionalScopedID(q interface{ QueryRow(string, ...any) *sql.Row }, table string, scope Scope, id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return requireScopedID(q, table, scope, id)
}

func requireScopedID(q interface{ QueryRow(string, ...any) *sql.Row }, table string, scope Scope, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	var count int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE id = ? AND tenant_id = ? AND lab_id = ?`, table)
	if err := q.QueryRow(query, id, scope.TenantID, scope.LabID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("unknown id %q", id)
	}
	return nil
}

func analysisServiceByIDTx(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope, id string) (AnalysisService, error) {
	services, err := analysisServicesQuery(q, scope, `AND s.id = ?`, []any{id})
	if err != nil {
		return AnalysisService{}, err
	}
	if len(services) != 1 {
		return AnalysisService{}, fmt.Errorf("unknown service %q", id)
	}
	return services[0], nil
}

func profileServicesTx(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope, profileID string) ([]AnalysisService, error) {
	query := serviceSelectSQL(`JOIN analysis_profile_services ps ON ps.service_id = s.id AND ps.profile_id = ?`, `s.tenant_id = ? AND s.lab_id = ?`, `ps.sort_order, s.name, s.id`)
	return scanAnalysisServices(q, query, profileID, scope.TenantID, scope.LabID)
}

func analysisServicesQuery(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope, extraWhere string, extraArgs []any) ([]AnalysisService, error) {
	args := append([]any{scope.TenantID, scope.LabID}, extraArgs...)
	query := serviceSelectSQL("", `s.tenant_id = ? AND s.lab_id = ? `+extraWhere, `d.sort_order, s.sort_order, s.name, s.id`)
	return scanAnalysisServices(q, query, args...)
}

func serviceSelectSQL(extraJoin, where, order string) string {
	return fmt.Sprintf(`SELECT s.id, s.tenant_id, s.lab_id, s.name, s.department_id, d.name, s.method_id, COALESCE(m.name, ''), s.analyte_id, COALESCE(a.name, ''), s.unit_id, COALESCE(u.symbol, ''), s.sort_order, s.created_at
FROM analysis_services s
JOIN catalog_departments d ON d.id = s.department_id AND d.tenant_id = s.tenant_id AND d.lab_id = s.lab_id
LEFT JOIN catalog_methods m ON m.id = s.method_id AND m.tenant_id = s.tenant_id AND m.lab_id = s.lab_id
LEFT JOIN catalog_analytes a ON a.id = s.analyte_id AND a.tenant_id = s.tenant_id AND a.lab_id = s.lab_id
LEFT JOIN catalog_units u ON u.id = s.unit_id AND u.tenant_id = s.tenant_id AND u.lab_id = s.lab_id
%s
WHERE %s
ORDER BY %s`, extraJoin, where, order)
}

func scanAnalysisServices(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, query string, args ...any) ([]AnalysisService, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	services := []AnalysisService{}
	for rows.Next() {
		var service AnalysisService
		var created string
		if err := rows.Scan(&service.ID, &service.TenantID, &service.LabID, &service.Name, &service.DepartmentID, &service.DepartmentName, &service.MethodID, &service.MethodName, &service.AnalyteID, &service.AnalyteName, &service.UnitID, &service.UnitSymbol, &service.SortOrder, &created); err != nil {
			return nil, err
		}
		service.CreatedAt, _ = parseTime(created)
		services = append(services, service)
	}
	return services, rows.Err()
}

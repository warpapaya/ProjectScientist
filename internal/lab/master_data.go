package lab

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Site struct {
	TenantID  string    `json:"tenant_id"`
	LabID     string    `json:"lab_id"`
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	Name      string    `json:"name"`
	Division  string    `json:"division"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

type Contact struct {
	TenantID  string    `json:"tenant_id"`
	LabID     string    `json:"lab_id"`
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	SiteID    string    `json:"site_id,omitempty"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
}

type ContactRole struct {
	TenantID  string    `json:"tenant_id"`
	LabID     string    `json:"lab_id"`
	ID        string    `json:"id"`
	ContactID string    `json:"contact_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Project struct {
	TenantID      string    `json:"tenant_id"`
	LabID         string    `json:"lab_id"`
	ID            string    `json:"id"`
	ClientID      string    `json:"client_id"`
	SiteID        string    `json:"site_id,omitempty"`
	Name          string    `json:"name"`
	WorkOrder     string    `json:"work_order"`
	DefaultMatrix string    `json:"default_matrix"`
	DefaultTests  []string  `json:"default_tests"`
	CreatedAt     time.Time `json:"created_at"`
}

type ClientDefaults struct {
	TenantID       string    `json:"tenant_id"`
	LabID          string    `json:"lab_id"`
	ClientID       string    `json:"client_id"`
	ReportTemplate string    `json:"report_template"`
	InvoiceEmail   string    `json:"invoice_email"`
	DefaultMatrix  string    `json:"default_matrix"`
	DefaultTests   []string  `json:"default_tests"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SiteInput struct{ ClientID, Name, Division, Address string }
type ContactInput struct{ ClientID, SiteID, Name, Email, Phone string }
type ContactRoleInput struct{ ContactID, Role string }
type ProjectInput struct {
	ClientID, SiteID, Name, WorkOrder, DefaultMatrix string
	DefaultTests                                     []string
}
type ClientDefaultsInput struct {
	ClientID, ReportTemplate, InvoiceEmail, DefaultMatrix string
	DefaultTests                                          []string
}

func (s *Store) CreateSiteForScope(scope Scope, input SiteInput, actor string) (Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Site{}, err
	}
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.Name = strings.TrimSpace(input.Name)
	if input.ClientID == "" {
		return Site{}, errors.New("client id is required")
	}
	if input.Name == "" {
		return Site{}, errors.New("site name is required")
	}
	now := time.Now().UTC()
	var site Site
	err = s.withTx(func(tx *sql.Tx) error {
		if err := requireClientTx(tx, scope, input.ClientID); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_site")
		if err != nil {
			return err
		}
		site = Site{TenantID: scope.TenantID, LabID: scope.LabID, ID: fmt.Sprintf("ST-%05d", next), ClientID: input.ClientID, Name: input.Name, Division: strings.TrimSpace(input.Division), Address: strings.TrimSpace(input.Address), CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO sites(tenant_id, lab_id, id, client_id, name, division, address, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, site.TenantID, site.LabID, site.ID, site.ClientID, site.Name, site.Division, site.Address, formatTime(site.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, scope, actor, "site.created", "site", site.ID, map[string]any{"client_id": site.ClientID, "name": site.Name})
	})
	return site, err
}

func (s *Store) CreateContactForScope(scope Scope, input ContactInput, actor string) (Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Contact{}, err
	}
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.SiteID = strings.TrimSpace(input.SiteID)
	input.Name = strings.TrimSpace(input.Name)
	if input.ClientID == "" {
		return Contact{}, errors.New("client id is required")
	}
	if input.Name == "" {
		return Contact{}, errors.New("contact name is required")
	}
	now := time.Now().UTC()
	var contact Contact
	err = s.withTx(func(tx *sql.Tx) error {
		if err := requireClientTx(tx, scope, input.ClientID); err != nil {
			return err
		}
		if input.SiteID != "" {
			if err := requireSiteTx(tx, scope, input.SiteID, input.ClientID); err != nil {
				return err
			}
		}
		next, err := nextCounter(tx, "next_contact")
		if err != nil {
			return err
		}
		contact = Contact{TenantID: scope.TenantID, LabID: scope.LabID, ID: fmt.Sprintf("CT-%05d", next), ClientID: input.ClientID, SiteID: input.SiteID, Name: input.Name, Email: strings.TrimSpace(input.Email), Phone: strings.TrimSpace(input.Phone), CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO contacts(tenant_id, lab_id, id, client_id, site_id, name, email, phone, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, contact.TenantID, contact.LabID, contact.ID, contact.ClientID, contact.SiteID, contact.Name, contact.Email, contact.Phone, formatTime(contact.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, scope, actor, "contact.created", "contact", contact.ID, map[string]any{"client_id": contact.ClientID, "site_id": contact.SiteID, "name": contact.Name})
	})
	return contact, err
}

func (s *Store) AssignContactRoleForScope(scope Scope, input ContactRoleInput, actor string) (ContactRole, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return ContactRole{}, err
	}
	input.ContactID = strings.TrimSpace(input.ContactID)
	input.Role = strings.TrimSpace(input.Role)
	if input.ContactID == "" {
		return ContactRole{}, errors.New("contact id is required")
	}
	if input.Role == "" {
		return ContactRole{}, errors.New("contact role is required")
	}
	now := time.Now().UTC()
	var role ContactRole
	err = s.withTx(func(tx *sql.Tx) error {
		if err := requireContactTx(tx, scope, input.ContactID); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_contact_role")
		if err != nil {
			return err
		}
		role = ContactRole{TenantID: scope.TenantID, LabID: scope.LabID, ID: fmt.Sprintf("CR-%05d", next), ContactID: input.ContactID, Role: input.Role, CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO contact_roles(tenant_id, lab_id, id, contact_id, role, created_at) VALUES (?, ?, ?, ?, ?, ?)`, role.TenantID, role.LabID, role.ID, role.ContactID, role.Role, formatTime(role.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, scope, actor, "contact.role.assigned", "contact_role", role.ID, map[string]any{"contact_id": role.ContactID, "role": role.Role})
	})
	return role, err
}

func (s *Store) CreateProjectForScope(scope Scope, input ProjectInput, actor string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Project{}, err
	}
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.SiteID = strings.TrimSpace(input.SiteID)
	input.Name = strings.TrimSpace(input.Name)
	if input.ClientID == "" {
		return Project{}, errors.New("client id is required")
	}
	if input.Name == "" {
		return Project{}, errors.New("project name is required")
	}
	now := time.Now().UTC()
	var project Project
	err = s.withTx(func(tx *sql.Tx) error {
		if err := requireClientTx(tx, scope, input.ClientID); err != nil {
			return err
		}
		if input.SiteID != "" {
			if err := requireSiteTx(tx, scope, input.SiteID, input.ClientID); err != nil {
				return err
			}
		}
		testsJSON, err := json.Marshal(cleanStrings(input.DefaultTests))
		if err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_project")
		if err != nil {
			return err
		}
		project = Project{TenantID: scope.TenantID, LabID: scope.LabID, ID: fmt.Sprintf("P-%05d", next), ClientID: input.ClientID, SiteID: input.SiteID, Name: input.Name, WorkOrder: strings.TrimSpace(input.WorkOrder), DefaultMatrix: strings.TrimSpace(input.DefaultMatrix), DefaultTests: cleanStrings(input.DefaultTests), CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO projects(tenant_id, lab_id, id, client_id, site_id, name, work_order, default_matrix, default_tests_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, project.TenantID, project.LabID, project.ID, project.ClientID, project.SiteID, project.Name, project.WorkOrder, project.DefaultMatrix, string(testsJSON), formatTime(project.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, scope, actor, "project.created", "project", project.ID, map[string]any{"client_id": project.ClientID, "site_id": project.SiteID, "work_order": project.WorkOrder})
	})
	return project, err
}

func (s *Store) UpsertClientDefaultsForScope(scope Scope, input ClientDefaultsInput, actor string) (ClientDefaults, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return ClientDefaults{}, err
	}
	input.ClientID = strings.TrimSpace(input.ClientID)
	if input.ClientID == "" {
		return ClientDefaults{}, errors.New("client id is required")
	}
	now := time.Now().UTC()
	tests := cleanStrings(input.DefaultTests)
	var defaults ClientDefaults
	err = s.withTx(func(tx *sql.Tx) error {
		if err := requireClientTx(tx, scope, input.ClientID); err != nil {
			return err
		}
		var createdRaw string
		if err := tx.QueryRow(`SELECT created_at FROM client_defaults WHERE tenant_id = ? AND lab_id = ? AND client_id = ?`, scope.TenantID, scope.LabID, input.ClientID).Scan(&createdRaw); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		created := now
		if createdRaw != "" {
			if parsed, err := parseTime(createdRaw); err == nil {
				created = parsed
			}
		}
		testsJSON, err := json.Marshal(tests)
		if err != nil {
			return err
		}
		defaults = ClientDefaults{TenantID: scope.TenantID, LabID: scope.LabID, ClientID: input.ClientID, ReportTemplate: strings.TrimSpace(input.ReportTemplate), InvoiceEmail: strings.TrimSpace(input.InvoiceEmail), DefaultMatrix: strings.TrimSpace(input.DefaultMatrix), DefaultTests: tests, CreatedAt: created, UpdatedAt: now}
		if _, err := tx.Exec(`INSERT INTO client_defaults(tenant_id, lab_id, client_id, report_template, invoice_email, default_matrix, default_tests_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(tenant_id, lab_id, client_id) DO UPDATE SET report_template = excluded.report_template, invoice_email = excluded.invoice_email, default_matrix = excluded.default_matrix, default_tests_json = excluded.default_tests_json, updated_at = excluded.updated_at`, defaults.TenantID, defaults.LabID, defaults.ClientID, defaults.ReportTemplate, defaults.InvoiceEmail, defaults.DefaultMatrix, string(testsJSON), formatTime(defaults.CreatedAt), formatTime(defaults.UpdatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, scope, actor, "client.defaults.upserted", "client", defaults.ClientID, map[string]any{"default_matrix": defaults.DefaultMatrix, "test_count": len(defaults.DefaultTests)})
	})
	return defaults, err
}

func (s *Store) SitesForScope(scope Scope) []Site {
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT tenant_id, lab_id, id, client_id, name, division, address, created_at FROM sites WHERE tenant_id = ? AND lab_id = ? ORDER BY id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Site{}
	for rows.Next() {
		var x Site
		var created string
		if rows.Scan(&x.TenantID, &x.LabID, &x.ID, &x.ClientID, &x.Name, &x.Division, &x.Address, &created) == nil {
			x.CreatedAt, _ = parseTime(created)
			out = append(out, x)
		}
	}
	return out
}
func (s *Store) ContactsForScope(scope Scope) []Contact {
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT tenant_id, lab_id, id, client_id, site_id, name, email, phone, created_at FROM contacts WHERE tenant_id = ? AND lab_id = ? ORDER BY id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Contact{}
	for rows.Next() {
		var x Contact
		var created string
		if rows.Scan(&x.TenantID, &x.LabID, &x.ID, &x.ClientID, &x.SiteID, &x.Name, &x.Email, &x.Phone, &created) == nil {
			x.CreatedAt, _ = parseTime(created)
			out = append(out, x)
		}
	}
	return out
}
func (s *Store) ContactRolesForScope(scope Scope) []ContactRole {
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT tenant_id, lab_id, id, contact_id, role, created_at FROM contact_roles WHERE tenant_id = ? AND lab_id = ? ORDER BY id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []ContactRole{}
	for rows.Next() {
		var x ContactRole
		var created string
		if rows.Scan(&x.TenantID, &x.LabID, &x.ID, &x.ContactID, &x.Role, &created) == nil {
			x.CreatedAt, _ = parseTime(created)
			out = append(out, x)
		}
	}
	return out
}
func (s *Store) ProjectsForScope(scope Scope) []Project {
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT tenant_id, lab_id, id, client_id, site_id, name, work_order, default_matrix, default_tests_json, created_at FROM projects WHERE tenant_id = ? AND lab_id = ? ORDER BY id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var x Project
		var testsJSON, created string
		if rows.Scan(&x.TenantID, &x.LabID, &x.ID, &x.ClientID, &x.SiteID, &x.Name, &x.WorkOrder, &x.DefaultMatrix, &testsJSON, &created) == nil {
			_ = json.Unmarshal([]byte(testsJSON), &x.DefaultTests)
			x.CreatedAt, _ = parseTime(created)
			out = append(out, x)
		}
	}
	return out
}
func (s *Store) ClientDefaultsForScope(scope Scope, clientID string) (ClientDefaults, bool) {
	scope, err := normalizeScope(scope)
	if err != nil {
		return ClientDefaults{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var x ClientDefaults
	var testsJSON, created, updated string
	err = s.db.QueryRow(`SELECT tenant_id, lab_id, client_id, report_template, invoice_email, default_matrix, default_tests_json, created_at, updated_at FROM client_defaults WHERE tenant_id = ? AND lab_id = ? AND client_id = ?`, scope.TenantID, scope.LabID, strings.TrimSpace(clientID)).Scan(&x.TenantID, &x.LabID, &x.ClientID, &x.ReportTemplate, &x.InvoiceEmail, &x.DefaultMatrix, &testsJSON, &created, &updated)
	if err != nil {
		return ClientDefaults{}, false
	}
	_ = json.Unmarshal([]byte(testsJSON), &x.DefaultTests)
	x.CreatedAt, _ = parseTime(created)
	x.UpdatedAt, _ = parseTime(updated)
	return x, true
}
func (s *Store) AllClientDefaultsForScope(scope Scope) []ClientDefaults {
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT tenant_id, lab_id, client_id, report_template, invoice_email, default_matrix, default_tests_json, created_at, updated_at FROM client_defaults WHERE tenant_id = ? AND lab_id = ? ORDER BY client_id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []ClientDefaults{}
	for rows.Next() {
		var x ClientDefaults
		var testsJSON, created, updated string
		if rows.Scan(&x.TenantID, &x.LabID, &x.ClientID, &x.ReportTemplate, &x.InvoiceEmail, &x.DefaultMatrix, &testsJSON, &created, &updated) == nil {
			_ = json.Unmarshal([]byte(testsJSON), &x.DefaultTests)
			x.CreatedAt, _ = parseTime(created)
			x.UpdatedAt, _ = parseTime(updated)
			out = append(out, x)
		}
	}
	return out
}

func requireClientTx(tx *sql.Tx, scope Scope, clientID string) error {
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM clients WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, clientID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unknown client %q", clientID)
	}
	return err
}
func requireSiteTx(tx *sql.Tx, scope Scope, siteID, clientID string) error {
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM sites WHERE tenant_id = ? AND lab_id = ? AND id = ? AND client_id = ?`, scope.TenantID, scope.LabID, siteID, clientID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unknown site %q", siteID)
	}
	return err
}
func requireContactTx(tx *sql.Tx, scope Scope, contactID string) error {
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM contacts WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, contactID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unknown contact %q", contactID)
	}
	return err
}
func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

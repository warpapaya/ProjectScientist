package lab

import (
	"errors"
	"sort"
	"strings"
)

type TenantMembership struct {
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
}

type ActorContext struct {
	UserID              string             `json:"user_id"`
	DisplayNameSnapshot string             `json:"display_name_snapshot"`
	AuthProvider        string             `json:"auth_provider"`
	TenantMemberships   []TenantMembership `json:"tenant_memberships"`
	Roles               []string           `json:"roles"`
	ServiceAccount      bool               `json:"service_account"`
	RequestID           string             `json:"request_id"`
	CorrelationID       string             `json:"correlation_id,omitempty"`
}

type ActorContextInput struct {
	UserID            string
	DisplayName       string
	AuthProvider      string
	TenantMemberships []TenantMembership
	Roles             []string
	ServiceAccount    bool
	RequestID         string
	CorrelationID     string
}

func NewActorContext(input ActorContextInput) (ActorContext, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return ActorContext{}, errors.New("actor user id is required")
	}
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return ActorContext{}, errors.New("actor request id is required")
	}
	actor := ActorContext{
		UserID:              userID,
		DisplayNameSnapshot: strings.TrimSpace(input.DisplayName),
		AuthProvider:        strings.TrimSpace(input.AuthProvider),
		TenantMemberships:   normalizeTenantMemberships(input.TenantMemberships),
		Roles:               normalizeStrings(input.Roles),
		ServiceAccount:      input.ServiceAccount,
		RequestID:           requestID,
		CorrelationID:       strings.TrimSpace(input.CorrelationID),
	}
	if actor.DisplayNameSnapshot == "" {
		actor.DisplayNameSnapshot = userID
	}
	if actor.AuthProvider == "" {
		actor.AuthProvider = "local-dev"
	}
	if actor.CorrelationID == "" {
		actor.CorrelationID = requestID
	}
	return actor, nil
}

func MustActorContext(input ActorContextInput) ActorContext {
	actor, err := NewActorContext(input)
	if err != nil {
		panic(err)
	}
	return actor
}

func normalizeTenantMemberships(input []TenantMembership) []TenantMembership {
	out := make([]TenantMembership, 0, len(input))
	seenTenants := map[string]bool{}
	for _, membership := range input {
		tenantID := strings.TrimSpace(membership.TenantID)
		if tenantID == "" || seenTenants[tenantID] {
			continue
		}
		seenTenants[tenantID] = true
		out = append(out, TenantMembership{TenantID: tenantID, Roles: normalizeStrings(membership.Roles)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TenantID < out[j].TenantID })
	return out
}

func normalizeStrings(input []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(input))
	for _, value := range input {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

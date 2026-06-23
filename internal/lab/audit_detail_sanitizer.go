package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"strings"
)

var auditDetailAllowlist = map[string]map[string]bool{
	"client.imported": {
		"source":       true,
		"row":          true,
		"source_hash":  true,
		"payload_hash": true,
		"mapping_id":   true,
	},
	"sample.imported": {
		"source":              true,
		"row":                 true,
		"source_hash":         true,
		"payload_hash":        true,
		"container_count":     true,
		"custody_event_count": true,
	},
	"analysis_result.imported": {
		"source":                   true,
		"row":                      true,
		"source_hash":              true,
		"payload_hash":             true,
		"analysis_request_line_id": true,
	},
	"import.completed": {
		"entity":       true,
		"format":       true,
		"source":       true,
		"source_hash":  true,
		"payload_hash": true,
		"total_rows":   true,
		"created_rows": true,
		"skipped_rows": true,
	},
	"sample.label_artifact.generated": {
		"sample_id":     true,
		"content_hash":  true,
		"barcode_value": true,
		"qr_payload":    true,
		"format":        true,
	},
}

func sanitizeAuditDetails(action string, details map[string]any) map[string]any {
	if len(details) == 0 {
		return map[string]any{}
	}
	allowed := auditDetailAllowlist[strings.TrimSpace(action)]
	safe := make(map[string]any, len(details))
	for key, value := range details {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if allowed != nil {
			if !allowed[key] {
				continue
			}
		} else if auditDetailKeyDenied(action, key) {
			continue
		}
		safe[key] = sanitizeAuditDetailValue(key, value)
	}
	return safe
}

func auditDetailKeyDenied(action, key string) bool {
	if auditDetailAllowlist[strings.TrimSpace(action)] != nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "authorization", "legacy", "raw", "full_payload", "payload", "email", "name"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func sanitizeAuditDetailValue(key string, value any) any {
	s, ok := value.(string)
	if !ok {
		return value
	}
	if key == "source" {
		return sanitizeAuditSource(s)
	}
	if containsSecretLikeValue(s) {
		return auditStringHash(s)
	}
	return s
}

func sanitizeAuditSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	if parsed, err := url.Parse(source); err == nil && (parsed.RawQuery != "" || parsed.Fragment != "") {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		if parsed.Scheme == "" && parsed.Host == "" {
			return filepath.Base(parsed.Path)
		}
		return parsed.String()
	}
	if containsSecretLikeValue(source) {
		return auditStringHash(source)
	}
	return source
}

func containsSecretLikeValue(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"bearer ", "api_key=", "apikey=", "token=", "secret=", "password=", "sk_live", "sk_test"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func auditBytesHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func auditStringHash(value string) string {
	return auditBytesHash([]byte(value))
}

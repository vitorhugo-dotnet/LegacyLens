package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	safeSQL           = regexp.MustCompile(`(?i)^\s*(SELECT\s+[a-z0-9_., *()=?]+\s+FROM\s+[a-z0-9_.]+(?:\s+WHERE\s+[a-z0-9_., =<>!?()]+)?|INSERT\s+INTO\s+[a-z0-9_.]+\s*\([a-z0-9_, ]+\)\s*VALUES\s*\([?, ]+\)|UPDATE\s+[a-z0-9_.]+\s+SET\s+[a-z0-9_]+\s*=\s*\?\s+WHERE\s+[a-z0-9_]+\s*=\s*\?)\s*;?\s*$`)
	safeMetadataValue = regexp.MustCompile(`^[a-zA-Z0-9_./:{}@+ -]{1,512}$`)
	sqlInlineValue    = regexp.MustCompile(`(?i)(?:=|<>|!=|<|>)\s*-?[0-9]+(?:\b|$)`)
	sqlBareValue      = regexp.MustCompile(`(?i)(?:=|<>|!=|<|>)\s*[a-z_][a-z0-9_.]*|\b(?:NULL|TRUE|FALSE)\b|\b[0-9]+\b`)
	javaClass         = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)+$`)
	javaMethod        = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,127}$`)
	javaDescriptor    = regexp.MustCompile(`^\([A-Za-z0-9_$/;\[<>-]*\)[A-Za-z0-9_$/;\[<>-]+$`)
	javaDeployment    = regexp.MustCompile(`^[A-Za-z0-9_$.-]{1,128}@loader-[a-fA-F0-9]{1,16}$`)
	javaLine          = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	agentDroppedCount = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
	requestSpan = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// SanitizeMetadata keeps only low-risk, structured request and database metadata.
// Values that do not match the allowlisted shape are discarded before persistence.
func SanitizeMetadata(input map[string]string) (map[string]string, []Diagnostic) {
	allowed := map[string]bool{
		"http.method": true, "http.route": true, "http.status_code": true,
		"db.system": true, "db.operation": true, "db.collection": true,
		"code.class": true, "code.method": true, "code.descriptor": true,
		"code.deployment": true, "code.line": true, "code.line_missing": true,
		"agent.dropped_count": true,
		"spanId": true, "http.request_span": true,
	}
	clean := make(map[string]string)
	dropped := make([]string, 0)
	for key, value := range input {
		switch {
		case allowed[key] && validMetadataValue(key, value):
			clean[key] = value
		case key == "sql" && safeSQL.MatchString(value) && !sqlInlineValue.MatchString(value) && !sqlBareValue.MatchString(value) && !strings.ContainsAny(value, "'\"`\n\r") && !strings.Contains(value, "--") && !strings.Contains(value, "/*"):
			clean[key] = strings.TrimSpace(value)
		default:
			dropped = append(dropped, key)
		}
	}
	if len(dropped) == 0 {
		return clean, nil
	}
	sort.Strings(dropped)
	hash := sha256.Sum256([]byte(strings.Join(dropped, "\x00")))
	return clean, []Diagnostic{{ID: ID("diag_sanitize_" + hex.EncodeToString(hash[:8])), Code: "capture.metadata_removed", Message: "Sensitive or unsupported capture metadata was removed.", Severity: "warning", CreatedAt: time.Now().UTC()}}
}

func validMetadataValue(key, value string) bool {
	if len(value) == 0 || len(value) > 512 {
		return false
	}
	if strings.HasPrefix(key, "code.") {
		return validStructuredMetadata(key, value)
	}
	if key == "spanId" || key == "http.request_span" { return requestSpan.MatchString(value) && value != "0000000000000000" }
	return safeMetadataValue.MatchString(value)
}

func validStructuredMetadata(key, value string) bool {
	switch key {
	case "code.class":
		return javaClass.MatchString(value)
	case "code.method":
		return javaMethod.MatchString(value)
	case "code.descriptor":
		return javaDescriptor.MatchString(value)
	case "code.deployment":
		return javaDeployment.MatchString(value)
	case "code.line":
		return javaLine.MatchString(value)
	case "code.line_missing":
		return value == "true"
	case "agent.dropped_count":
		return agentDroppedCount.MatchString(value)
	default:
		return true
	}
}

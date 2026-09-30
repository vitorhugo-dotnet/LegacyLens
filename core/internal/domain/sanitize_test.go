package domain

import (
	"strings"
	"testing"
)

func TestSanitizeMetadata(t *testing.T) {
	input := map[string]string{
		"url":              "https://example.test/a?token=secret&view=orders",
		"cookie":           "session=secret",
		"exception":        "failed with bearer abc.def.ghi",
		"sql":              "SELECT * FROM orders WHERE id = ?",
		"sqlText":          "SELECT * FROM orders WHERE id = 'secret'",
		"http.method":      "GET",
		"http.route":       "/orders/{id}",
		"http.status_code": "200",
		"unknown":          "must disappear",
	}
	clean, diagnostics := SanitizeMetadata(input)
	joined := strings.Join([]string{clean["http.method"], clean["http.route"], clean["http.status_code"], clean["sql"], clean["url"]}, " ")
	for _, secret := range []string{"secret", "session=", "bearer abc", "must disappear", "token="} {
		if strings.Contains(joined, secret) {
			t.Fatalf("sanitized metadata retained %q: %#v", secret, clean)
		}
	}
	if clean["http.method"] != "GET" || clean["http.route"] != "/orders/{id}" || clean["http.status_code"] != "200" {
		t.Fatalf("safe allowlisted metadata lost: %#v", clean)
	}
	if clean["sql"] != "SELECT * FROM orders WHERE id = ?" {
		t.Fatalf("parameterized SQL not preserved: %#v", clean)
	}
	if len(diagnostics) == 0 {
		t.Fatal("removed sensitive or malformed metadata must be visible diagnostically")
	}
	for _, unsafeSQL := range []string{"SELECT * FROM orders WHERE id = 'secret'", "SELECT * FROM orders WHERE id = 123", "SELECT * FROM orders WHERE id = secretValue"} {
		clean, diagnostics := SanitizeMetadata(map[string]string{"sql": unsafeSQL})
		if _, ok := clean["sql"]; ok || len(diagnostics) == 0 {
			t.Fatalf("SQL with an inline parameter value must be removed: %#v", clean)
		}
	}
}

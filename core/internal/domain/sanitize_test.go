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

func TestSanitizeRequestSpanRequiresNonzeroLowerHexAndDropsSecrets(t *testing.T) {
 clean,_ := SanitizeMetadata(map[string]string{"spanId":"abcdef0123456789","http.request_span":"1234567890abcdef","traceparent":"00-secret","authorization":"Bearer secret","http.route":"/orders?token=secret"})
 if clean["spanId"] != "abcdef0123456789" || clean["http.request_span"] != "1234567890abcdef" { t.Fatalf("request identities lost: %#v",clean) }
 for _, key := range []string{"traceparent","authorization","http.route"} { if _, ok := clean[key]; ok { t.Fatalf("unsafe %s retained: %#v",key,clean) } }
 for _, bad := range []string{"0000000000000000","ABCDEF0123456789","abc","abcdef0123456789?token=secret"} {
  invalid,_ := SanitizeMetadata(map[string]string{"spanId":bad,"http.request_span":bad})
  if len(invalid) != 0 { t.Fatalf("invalid span %q retained: %#v",bad,invalid) }
 }
}

func TestSanitizeMetadataKeepsBoundedBrowserStackEvidence(t *testing.T) {
	input := map[string]string{
		"frameChain": "saveOrder@orders.js:42>submit@orders.js:18",
		"stackGap":   "ASYNC_BOUNDARY,STACK_UNAVAILABLE",
	}
	clean, diagnostics := SanitizeMetadata(input)
	for key, value := range input {
		if clean[key] != value {
			t.Fatalf("browser stack evidence %q was lost: %#v", key, clean)
		}
	}
	if len(diagnostics) != 0 {
		t.Fatalf("approved browser stack evidence should not create a removal diagnostic: %#v", diagnostics)
	}
	for _, unsafe := range []map[string]string{
		{"frameChain": strings.Repeat("x", 513)},
		{"stackGap": "raw stack at https://example.test/private.js"},
	} {
		clean, diagnostics = SanitizeMetadata(unsafe)
		if len(clean) != 0 || len(diagnostics) == 0 {
			t.Fatalf("unsafe browser stack evidence should be removed: %#v", clean)
		}
	}
}

func TestSanitizeMetadataKeepsStructuredJavaMethodIdentity(t *testing.T) {
	input := map[string]string{
		"code.class":        "com.example.OrderService",
		"code.method":       "loadOrder",
		"code.descriptor":   "(Ljava/lang/String;)V",
		"code.deployment":   "deployment@loader-19af",
		"code.line_missing": "true",
		"code.arguments":    "customer-secret",
		"code.return":       "secret-value",
		"code.exception":    "password=secret",
		"code.source":       "C:/private/project/OrderService.java",
	}
	clean, diagnostics := SanitizeMetadata(input)
	for key, value := range input {
		if strings.HasPrefix(key, "code.") && key != "code.arguments" && key != "code.return" && key != "code.exception" && key != "code.source" && clean[key] != value {
			t.Fatalf("safe structured identity %q was lost: %#v", key, clean)
		}
	}
	for _, key := range []string{"code.arguments", "code.return", "code.exception", "code.source"} {
		if _, ok := clean[key]; ok {
			t.Fatalf("sensitive/unapproved identity field %q retained: %#v", key, clean)
		}
	}
	if len(diagnostics) == 0 {
		t.Fatal("removed fields must emit a sanitization diagnostic")
	}
	line, _ := SanitizeMetadata(map[string]string{"code.class": "com.example.OrderService", "code.method": "loadOrder", "code.descriptor": "()V", "code.deployment": "deployment@loader-19af", "code.line": "42"})
	if line["code.line"] != "42" {
		t.Fatalf("valid bytecode debug line was lost: %#v", line)
	}
}

func TestSanitizeMetadataKeepsNumericAgentThreadIdentity(t *testing.T) {
	clean, diagnostics := SanitizeMetadata(map[string]string{"agent.thread_id": "42", "agent.thread_name": "sensitive"})
	if clean["agent.thread_id"] != "42" {
		t.Fatalf("numeric agent thread identity was lost: %#v", clean)
	}
	if _, ok := clean["agent.thread_name"]; ok || len(diagnostics) == 0 {
		t.Fatalf("unapproved thread metadata should be removed with a diagnostic: %#v", clean)
	}
	for _, invalid := range []string{"0", "-1", "thread-1"} {
		clean, _ = SanitizeMetadata(map[string]string{"agent.thread_id": invalid})
		if _, ok := clean["agent.thread_id"]; ok {
			t.Fatalf("invalid agent thread identity %q was retained", invalid)
		}
	}
}

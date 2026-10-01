package application

import (
 "testing"
 "time"

 "legacylens/core/internal/domain"
)

func TestAgentPresenceRequiresExactProjectAndProcessAndExpires(t *testing.T) {
 now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
 p := NewAgentPresence(func() time.Time { return now }, time.Second*5, 2)
 if err := p.Heartbeat("p1", "process-a"); err != nil { t.Fatal(err) }
 if got := p.Status("p2", nil); got.State != "unknown" { t.Fatalf("other project: %+v", got) }
 if got := p.Status("p1", []domain.ID{"process-b"}); got.State != "unknown" { t.Fatalf("other process: %+v", got) }
 if got := p.Status("p1", []domain.ID{"process-a"}); got.State != "online" || got.EvidenceDiagnosticID == "" { t.Fatalf("online: %+v", got) }
 now = now.Add(6*time.Second)
 if got := p.Status("p1", []domain.ID{"process-a"}); got.State != "offline" || got.EvidenceDiagnosticID == "" { t.Fatalf("expired: %+v", got) }
 if err := p.Heartbeat("p1", "process-b"); err != nil { t.Fatal(err) }
 if err := p.Heartbeat("p1", "process-c"); err == nil { t.Fatal("capacity accepted a third process") }
}

func TestAgentPresenceMatchesTraceEpochToAuthenticatedProcess(t *testing.T) {
 now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
 p := NewAgentPresence(func() time.Time { return now }, time.Second*5, 2)
 if err := p.Heartbeat("p1", "legacy-fixture-abc123"); err != nil { t.Fatal(err) }
 if got := p.Status("p1", []domain.ID{"legacy-fixture-abc123-7"}); got.State != "online" { t.Fatalf("trace epoch must match process heartbeat: %+v", got) }
 for _, unrelated := range []domain.ID{"legacy-fixture-abc1234-7", "legacy-fixture-abc123-x", "legacy-fixture-abc123-7-extra"} {
  if got := p.Status("p1", []domain.ID{unrelated}); got.State != "unknown" { t.Fatalf("unrelated epoch %q: %+v", unrelated, got) }
 }
 if got := p.Status("p2", []domain.ID{"legacy-fixture-abc123-7"}); got.State != "unknown" { t.Fatalf("other project: %+v", got) }
}

package application

import (
 "errors"
 "strconv"
 "strings"
 "sync"
 "time"

 "legacylens/core/internal/domain"
)

type presenceKey struct { project, process domain.ID }
type AgentPresence struct {
 mu sync.Mutex
 now func() time.Time
 ttl time.Duration
 capacity int
 seen map[presenceKey]time.Time
}

func NewAgentPresence(now func() time.Time, ttl time.Duration, capacity int) *AgentPresence {
 return &AgentPresence{now:now,ttl:ttl,capacity:capacity,seen:make(map[presenceKey]time.Time)}
}

func (p *AgentPresence) Heartbeat(project, process domain.ID) error {
 if p == nil || project == "" || process == "" { return errors.New("project and process are required") }
 p.mu.Lock(); defer p.mu.Unlock()
 key := presenceKey{project,process}
 if _, exists := p.seen[key]; !exists && len(p.seen) >= p.capacity { return errors.New("agent presence capacity reached") }
 p.seen[key] = p.now().UTC()
 return nil
}

func (p *AgentPresence) Status(project domain.ID, expected []domain.ID) AgentStatus {
 if p == nil { return AgentStatus{State:"unknown"} }
 p.mu.Lock(); defer p.mu.Unlock()
 var latest time.Time
 for key, at := range p.seen {
  if key.project != project { continue }
  if len(expected) > 0 {
   found := false
   for _, process := range expected { if process == key.process || traceEpochOfProcess(process, key.process) { found = true; break } }
   if !found { continue }
  }
  if at.After(latest) { latest = at }
 }
 if latest.IsZero() { return AgentStatus{State:"unknown"} }
 state := "online"
 if p.now().UTC().Sub(latest) > p.ttl { state = "offline" }
 return AgentStatus{State:state,EvidenceDiagnosticID:domain.ID("agent.presence."+state)}
}

func traceEpochOfProcess(producer, process domain.ID) bool {
 suffix, ok := strings.CutPrefix(string(producer), string(process)+"-")
 if !ok || suffix == "" { return false }
 for _, digit := range suffix { if digit < '0' || digit > '9' { return false } }
 epoch, err := strconv.ParseUint(suffix, 10, 64)
 return err == nil && epoch > 0
}

func (p *AgentPresence) Diagnostic(status AgentStatus) domain.Diagnostic {
 code, message := "agent.heartbeat", "Authenticated Java agent heartbeat is current."
 if status.State == "offline" { code, message = "agent.heartbeat_expired", "Authenticated Java agent heartbeat expired." }
 return domain.Diagnostic{ID:status.EvidenceDiagnosticID,Code:code,Message:message,Severity:"info",CreatedAt:p.now().UTC()}
}

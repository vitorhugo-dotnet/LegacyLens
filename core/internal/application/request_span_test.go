package application

import (
 "context"
 "testing"
 "time"

 "legacylens/core/internal/domain"
)

func TestInvestigationLinksOnlyMatchingRequestSpan(t *testing.T) {
 project, trace := domain.ID("p"), domain.ID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
 click := domain.ID("click")
 now := time.Now()
 events := []domain.Event{
  {ProjectID:project,TraceID:trace,ProducerID:"browser",Sequence:1,EventID:click,Kind:"jsf.click",OccurredAt:now},
  {ProjectID:project,TraceID:trace,ProducerID:"browser",Sequence:2,EventID:"request-a",ParentEventID:&click,Kind:"browser.network",OccurredAt:now,Metadata:map[string]string{"spanId":"1111111111111111"}},
  {ProjectID:project,TraceID:trace,ProducerID:"browser",Sequence:3,EventID:"request-b",ParentEventID:&click,Kind:"browser.network",OccurredAt:now,Metadata:map[string]string{"spanId":"2222222222222222"}},
  {ProjectID:project,TraceID:trace,ProducerID:"agent",Sequence:1,EventID:"server-a",Kind:"http.server",OccurredAt:now,Metadata:map[string]string{"http.request_span":"1111111111111111"}},
  {ProjectID:project,TraceID:trace,ProducerID:"agent",Sequence:2,EventID:"server-b",Kind:"http.server",OccurredAt:now,Metadata:map[string]string{"http.request_span":"2222222222222222"}},
  {ProjectID:project,TraceID:trace,ProducerID:"agent",Sequence:3,EventID:"poll",Kind:"http.server",OccurredAt:now,Metadata:map[string]string{"http.request_span":"3333333333333333"}},
 }
 got, err := NewInvestigationService(investigationStore{Investigation{Trace:domain.Trace{ID:trace,ProjectID:project},Events:events}}).Get(context.Background(),project,trace)
 if err != nil { t.Fatal(err) }
 edges := map[domain.ID]domain.ID{}
 for _, relation := range got.Relations { if relation.Kind == "http.request" && relation.ToID != nil { edges[relation.FromID] = *relation.ToID } }
 if len(edges) != 2 || edges[got.Symbols[1].ID] != got.Symbols[3].ID || edges[got.Symbols[2].ID] != got.Symbols[4].ID { t.Fatalf("wrong request edges: %+v",edges) }
}

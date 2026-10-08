package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"legacylens/core/internal/domain"
)

const (
	maxExplanationEvidence = 20
	maxExplanationQuestion = 2000
	maxPendingExplanations = 128
	explanationPreviewTTL  = 5 * time.Minute
)

var (
	ErrExplicitConsentRequired    = errors.New("explicit consent is required")
	ErrUnknownExplanationEvidence = errors.New("provider referenced evidence outside the approved package")
	ErrExplanationPreviewExpired  = errors.New("explanation preview is missing or expired")
	ErrExplainerUnavailable       = errors.New("explanation provider is not configured")
)

type ExplanationEvidence struct {
	ID     domain.ID `json:"id"`
	Kind   string    `json:"kind"`
	Path   string    `json:"path,omitempty"`
	Line   int       `json:"line,omitempty"`
	Column int       `json:"column,omitempty"`
}

type ExplanationPackage struct {
	Question            string                `json:"question"`
	IndexedRevisionID   domain.ID             `json:"indexedRevisionId,omitempty"`
	DeploymentRevisions []domain.ID           `json:"deploymentRevisions"`
	TraceIncomplete     bool                  `json:"traceIncomplete"`
	Evidence            []ExplanationEvidence `json:"evidence"`
	Limitations         []string              `json:"limitations"`
}

type ExplanationPreview struct {
	PreviewID         string             `json:"previewId"`
	ExpiresAt         time.Time          `json:"expiresAt"`
	Destination       string             `json:"destination"`
	ProviderAvailable bool               `json:"providerAvailable"`
	Package           ExplanationPackage `json:"package"`
}

type ExplanationConsent struct {
	PreviewID string `json:"previewId"`
	Consented bool   `json:"consent"`
}

type pendingExplanation struct {
	packageValue ExplanationPackage
	expiresAt    time.Time
}

type ExplanationService struct {
	investigations *InvestigationService
	explainer      Explainer
	mu             sync.Mutex
	pending        map[string]pendingExplanation
}

func NewExplanationService(investigations *InvestigationService, explainer Explainer) *ExplanationService {
	return &ExplanationService{investigations: investigations, explainer: explainer, pending: make(map[string]pendingExplanation)}
}

func (s *ExplanationService) Preview(ctx context.Context, request ExplanationRequest) (ExplanationPreview, error) {
	if s == nil || s.investigations == nil {
		return ExplanationPreview{}, errors.New("investigation service is required")
	}
	question := strings.TrimSpace(request.Question)
	if request.ProjectID == "" || request.TraceID == "" || question == "" || len(question) > maxExplanationQuestion || len(request.EvidenceIDs) == 0 || len(request.EvidenceIDs) > maxExplanationEvidence {
		return ExplanationPreview{}, errors.New("project, trace, question, and 1 to 20 evidence ids are required")
	}
	investigation, err := s.investigations.Get(ctx, request.ProjectID, request.TraceID)
	if err != nil {
		return ExplanationPreview{}, errors.New("investigation could not be loaded")
	}
	byID := make(map[domain.ID]domain.Evidence, len(investigation.Evidence))
	for _, evidence := range investigation.Evidence {
		byID[evidence.ID] = evidence
	}
	packageValue := ExplanationPackage{Question: question, IndexedRevisionID: investigation.IndexedRevisionID, DeploymentRevisions: deploymentRevisions(investigation.Events), TraceIncomplete: investigation.Trace.Incomplete,
		Evidence:    make([]ExplanationEvidence, 0, len(request.EvidenceIDs)),
		Limitations: []string{"Only selected evidence identifiers, types, and source locations are included; source text, SQL literals, event payloads, logs, traces, and credentials are excluded.", "Generated explanations are unverified assistance; inspect cited evidence and confirm it against the deployed revision."}}
	seen := make(map[domain.ID]struct{}, len(request.EvidenceIDs))
	for _, id := range request.EvidenceIDs {
		if id == "" {
			return ExplanationPreview{}, errors.New("evidence ids must be non-empty")
		}
		if _, duplicate := seen[id]; duplicate {
			return ExplanationPreview{}, errors.New("evidence ids must be unique")
		}
		seen[id] = struct{}{}
		evidence, exists := byID[id]
		if !exists {
			return ExplanationPreview{}, errors.New("selected evidence is not present in this investigation")
		}
		item := ExplanationEvidence{ID: evidence.ID, Kind: evidence.Kind}
		if evidence.Location != nil && evidence.Location.Line >= 1 && evidence.Location.Column >= 1 {
			if path := safeRelativeEvidencePath(evidence.Location.Path); path != "" {
				item.Path, item.Line, item.Column = path, evidence.Location.Line, evidence.Location.Column
			}
		}
		packageValue.Evidence = append(packageValue.Evidence, item)
	}
	previewID, err := explanationNonce()
	if err != nil {
		return ExplanationPreview{}, errors.New("preview could not be created")
	}
	expiresAt := time.Now().UTC().Add(explanationPreviewTTL)
	s.mu.Lock()
	now := time.Now()
	for id, pending := range s.pending {
		if now.After(pending.expiresAt) {
			delete(s.pending, id)
		}
	}
	if len(s.pending) >= maxPendingExplanations {
		s.mu.Unlock()
		return ExplanationPreview{}, errors.New("too many active explanation previews")
	}
	s.pending[previewID] = pendingExplanation{packageValue: packageValue, expiresAt: expiresAt}
	s.mu.Unlock()
	destination, available := "Not configured", s.explainer != nil
	if available {
		destination = s.explainer.Destination()
	}
	return ExplanationPreview{PreviewID: previewID, ExpiresAt: expiresAt, Destination: destination, ProviderAvailable: available, Package: packageValue}, nil
}

func (s *ExplanationService) Generate(ctx context.Context, consent ExplanationConsent) (ExplanationResult, error) {
	if !consent.Consented {
		return ExplanationResult{}, ErrExplicitConsentRequired
	}
	if s == nil || s.explainer == nil {
		return ExplanationResult{}, ErrExplainerUnavailable
	}
	s.mu.Lock()
	pending, exists := s.pending[consent.PreviewID]
	delete(s.pending, consent.PreviewID)
	s.mu.Unlock()
	if !exists || time.Now().After(pending.expiresAt) {
		return ExplanationResult{}, ErrExplanationPreviewExpired
	}
	result, err := s.explainer.Explain(ctx, pending.packageValue)
	if err != nil {
		return ExplanationResult{}, fmt.Errorf("explanation provider request failed: %w", err)
	}
	if result.Claims == nil || result.Limitations == nil {
		return ExplanationResult{}, errors.New("provider response did not match the explanation contract")
	}
	approved := make(map[domain.ID]struct{}, len(pending.packageValue.Evidence))
	for _, evidence := range pending.packageValue.Evidence {
		approved[evidence.ID] = struct{}{}
	}
	if len(result.Claims) > 40 {
		return ExplanationResult{}, errors.New("provider returned too many claims")
	}
	for i := range result.Claims {
		claim := &result.Claims[i]
		claim.Text = strings.TrimSpace(claim.Text)
		if claim.Text == "" || len(claim.Text) > 4000 || (claim.Confidence != "supported" && claim.Confidence != "hypothesis") {
			return ExplanationResult{}, errors.New("provider response did not match the explanation contract")
		}
		seen := map[domain.ID]struct{}{}
		for _, id := range claim.EvidenceIDs {
			if _, ok := approved[id]; !ok {
				return ExplanationResult{}, ErrUnknownExplanationEvidence
			}
			if _, duplicate := seen[id]; duplicate {
				return ExplanationResult{}, errors.New("provider repeated an evidence id")
			}
			seen[id] = struct{}{}
		}
		if claim.Confidence == "supported" && len(claim.EvidenceIDs) == 0 {
			return ExplanationResult{}, errors.New("supported claims require evidence")
		}
	}
	if len(result.Limitations) > 20 {
		return ExplanationResult{}, errors.New("provider returned too many limitations")
	}
	for _, limitation := range result.Limitations {
		if strings.TrimSpace(limitation) == "" || len(limitation) > 1000 {
			return ExplanationResult{}, errors.New("provider response did not match the explanation contract")
		}
	}
	result.Limitations = append(result.Limitations, "Generated text is not a correctness guarantee; validate each cited claim against the source and deployed revision.")
	return result, nil
}

func deploymentRevisions(events []domain.Event) []domain.ID {
	set := map[domain.ID]struct{}{}
	for _, event := range events {
		if event.ApplicationRevision != nil && *event.ApplicationRevision != "" {
			set[*event.ApplicationRevision] = struct{}{}
		}
	}
	result := make([]domain.ID, 0, len(set))
	for revision := range set {
		result = append(result, revision)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func safeRelativeEvidencePath(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	path := filepath.Clean(value)
	if filepath.IsAbs(path) || filepath.VolumeName(path) != "" || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(path)
}

func explanationNonce() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

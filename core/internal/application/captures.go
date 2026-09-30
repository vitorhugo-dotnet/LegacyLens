package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"legacylens/core/internal/domain"
)

const (
	defaultCaptureEventLimit = 10_000
	defaultCaptureDuration   = 10 * time.Minute
)

type CaptureConfig struct {
	MaxEvents   int
	MaxDuration time.Duration
	Now         func() time.Time
}

type CaptureService struct {
	store  CaptureStore
	config CaptureConfig
}

func NewCaptureService(store CaptureStore, config CaptureConfig) *CaptureService {
	if config.MaxEvents <= 0 {
		config.MaxEvents = defaultCaptureEventLimit
	}
	if config.MaxDuration <= 0 {
		config.MaxDuration = defaultCaptureDuration
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	return &CaptureService{store: store, config: config}
}

func (s *CaptureService) Start(ctx context.Context, request CaptureRequest) (CaptureSession, error) {
	if s == nil || s.store == nil {
		return CaptureSession{}, errors.New("capture store is required")
	}
	if request.ProjectID == "" {
		return CaptureSession{}, errors.New("project id is required")
	}
	if _, err := s.store.LoadProject(ctx, request.ProjectID); err != nil {
		return CaptureSession{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return CaptureSession{}, errors.New("could not create capture identity")
	}
	started := s.config.Now().UTC()
	trace := domain.Trace{ID: domain.ID(hex.EncodeToString(random[:])), ProjectID: request.ProjectID, StartedAt: started}
	if err := s.store.StartCapture(ctx, trace, request.TabID); err != nil {
		return CaptureSession{}, err
	}
	return CaptureSession{ID: trace.ID, ProjectID: trace.ProjectID, StartedAt: started, ExpiresAt: started.Add(s.config.MaxDuration)}, nil
}

func (s *CaptureService) Stop(ctx context.Context, projectID, traceID domain.ID) error {
	if s == nil || s.store == nil {
		return errors.New("capture store is required")
	}
	if projectID == "" || traceID == "" {
		return errors.New("project and trace ids are required")
	}
	loaded, err := s.store.Load(ctx, projectID, traceID)
	if err != nil {
		return err
	}
	if loaded.Trace.ID != traceID || loaded.Trace.ProjectID != projectID {
		return errors.New("capture session not found")
	}
	return s.store.StopCapture(ctx, projectID, traceID, s.config.Now().UTC())
}

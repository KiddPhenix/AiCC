package service

import (
	"context"

	"aihelper/internal/app/capture"
)

type CaptureService struct {
	captures *capture.Service
}

func NewCaptureService(captures *capture.Service) *CaptureService {
	return &CaptureService{captures: captures}
}

func (s *CaptureService) ListCaptures(ctx context.Context, filter capture.Filter) ([]capture.Result, error) {
	return s.captures.ListCaptures(ctx, filter)
}

func (s *CaptureService) GetCapture(ctx context.Context, id int64) (*capture.Result, error) {
	return s.captures.GetCapture(ctx, id)
}

func (s *CaptureService) DeleteCapture(ctx context.Context, id int64) error {
	return s.captures.DeleteCapture(ctx, id)
}

func (s *CaptureService) DeleteUnanalyzedCaptures(ctx context.Context) (int64, error) {
	return s.captures.DeleteUnanalyzedCaptures(ctx)
}

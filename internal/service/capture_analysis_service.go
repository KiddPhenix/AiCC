package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"aihelper/internal/ai/extractor"
	"aihelper/internal/app/capture"
	"aihelper/internal/domain"
)

var ErrAnalysisUnavailable = errors.New("analysis unavailable")
var ErrAnalysisRecentlyRequested = errors.New("analysis recently requested")
var ErrAnalysisTimedOut = errors.New("analysis timed out")

type CaptureAnalysisService struct {
	captures  *capture.Service
	solutions *SolutionService
	client    *extractor.Client
	mu        sync.Mutex
	recent    []string
}

func NewCaptureAnalysisService(captures *capture.Service, solutions *SolutionService, client *extractor.Client) *CaptureAnalysisService {
	return &CaptureAnalysisService{
		captures:  captures,
		solutions: solutions,
		client:    client,
	}
}

func (s *CaptureAnalysisService) AnalyzeCapture(ctx context.Context, captureID int64) (*capture.Insight, error) {
	item, err := s.captures.GetCapture(ctx, captureID)
	if err != nil {
		return nil, err
	}
	if item.Insight != nil {
		return item.Insight, nil
	}
	if s.markRecentAnalysis(item.Content) {
		return nil, ErrAnalysisRecentlyRequested
	}

	analysis, rawJSON, err := s.client.AnalyzeCapture(ctx, item.Content, item.ContentType, item.CapturedAt)
	if err != nil {
		if errors.Is(err, extractor.ErrMissingAPIKey) {
			return nil, ErrAnalysisUnavailable
		}
		if errors.Is(err, extractor.ErrAnalysisTimeout) {
			return nil, ErrAnalysisTimedOut
		}
		return nil, err
	}

	insight := &capture.Insight{
		CaptureID:       captureID,
		Title:           analysis.Title,
		Summary:         analysis.Summary,
		Category:        analysis.Category,
		Problem:         analysis.Problem,
		SolutionText:    analysis.SolutionText,
		OccurredAt:      analysis.OccurredAt,
		SourceName:      firstNonEmpty(analysis.SourceName, item.Source),
		SourceRef:       analysis.SourceRef,
		SuggestedStatus: firstNonEmpty(analysis.SuggestedStatus, "draft"),
		Confidence:      analysis.Confidence,
		Tags:            analysis.Tags,
		Model:           s.client.Model(),
		Provider:        "openai",
		RawJSON:         rawJSON,
	}

	if err := s.captures.UpsertInsight(ctx, insight); err != nil {
		return nil, err
	}

	return s.captures.GetInsight(ctx, captureID)
}

func (s *CaptureAnalysisService) PromoteCapture(ctx context.Context, captureID int64) (*domain.Solution, error) {
	item, err := s.captures.GetCapture(ctx, captureID)
	if err != nil {
		return nil, err
	}
	if item.Insight == nil {
		return nil, ErrAnalysisUnavailable
	}

	input := domain.CreateSolutionInput{
		Title:          firstNonEmpty(item.Insight.Title, fallbackTitle(item.Content)),
		Problem:        item.Insight.Problem,
		SolutionText:   firstNonEmpty(item.Insight.SolutionText, item.Insight.Summary),
		RawContent:     maybeKeepRawContent(item.Content),
		EfficacySource: "auto",
		Status:         firstNonEmpty(item.Insight.SuggestedStatus, "draft"),
		SourceType:     "capture",
		SourceName:     firstNonEmpty(item.Insight.SourceName, item.Source),
		SourceRef:      buildCaptureRef(captureID),
		RawContentRef:  buildCaptureRef(captureID),
		Tags:           appendInsightTags(item.Insight.Tags, item.Insight.Category),
	}

	return s.solutions.CreateSolution(ctx, input)
}

func appendInsightTags(tags []string, category string) []string {
	out := append([]string{}, tags...)
	if strings.TrimSpace(category) != "" {
		out = append(out, category)
	}
	return out
}

func buildCaptureRef(id int64) string {
	return fmt.Sprintf("capture://%d", id)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func fallbackTitle(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "未命名抓取"
	}
	runes := []rune(content)
	if len(runes) > 24 {
		runes = runes[:24]
	}
	return string(runes)
}

func maybeKeepRawContent(content string) string {
	content = strings.TrimSpace(content)
	if len([]rune(content)) > 4096 {
		return ""
	}
	return content
}

func (s *CaptureAnalysisService) markRecentAnalysis(content string) bool {
	hash := hashAnalysisContent(content)
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, recent := range s.recent {
		if recent == hash {
			return true
		}
	}

	s.recent = append([]string{hash}, s.recent...)
	if len(s.recent) > 3 {
		s.recent = s.recent[:3]
	}
	return false
}

func hashAnalysisContent(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}

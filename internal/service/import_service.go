package service

import (
	"context"
	"fmt"
	"strings"

	"aihelper/internal/domain"
	"aihelper/internal/source"
)

type ImportService struct {
	solutions *SolutionService
	adapters  map[string]source.Adapter
}

type ImportInput struct {
	Adapter       string   `json:"adapter"`
	Path          string   `json:"path"`
	DefaultStatus string   `json:"default_status"`
	DefaultTags   []string `json:"default_tags"`
}

type ImportResult struct {
	Adapter      string            `json:"adapter"`
	Path         string            `json:"path"`
	Imported     int               `json:"imported"`
	Duplicates   int               `json:"duplicates"`
	Skipped      int               `json:"skipped"`
	CreatedItems []domain.Solution `json:"created_items"`
}

func NewImportService(solutions *SolutionService, adapters ...source.Adapter) *ImportService {
	index := make(map[string]source.Adapter, len(adapters))
	for _, adapter := range adapters {
		index[adapter.Name()] = adapter
	}
	return &ImportService{
		solutions: solutions,
		adapters:  index,
	}
}

func (s *ImportService) Import(ctx context.Context, input ImportInput) (*ImportResult, error) {
	adapterName := strings.TrimSpace(strings.ToLower(input.Adapter))
	adapter, ok := s.adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("unsupported adapter: %s", input.Adapter)
	}

	candidates, err := adapter.Import(ctx, source.ImportRequest{
		Path: input.Path,
	})
	if err != nil {
		return nil, err
	}

	result := &ImportResult{
		Adapter: adapterName,
		Path:    input.Path,
	}

	for _, candidate := range candidates {
		createInput := domain.CreateSolutionInput{
			Title:          candidate.Title,
			Problem:        candidate.Problem,
			SolutionText:   candidate.SolutionText,
			EfficacySource: "auto",
			Status:         fallbackStatus(candidate.Status, input.DefaultStatus),
			SourceType:     fallback(candidate.SourceType, adapterName),
			SourceName:     fallback(candidate.SourceName, adapterName),
			SourceRef:      candidate.SourceRef,
			RawContentRef:  candidate.RawContentRef,
			Tags:           append(append([]string{}, candidate.Tags...), input.DefaultTags...),
		}

		created, err := s.solutions.CreateSolution(ctx, createInput)
		if err != nil {
			switch {
			case err == domain.ErrDuplicate:
				result.Duplicates++
			case err == domain.ErrInvalidSolution:
				result.Skipped++
			default:
				return nil, err
			}
			continue
		}

		result.Imported++
		result.CreatedItems = append(result.CreatedItems, *created)
	}

	return result, nil
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func fallbackStatus(candidateStatus, defaultStatus string) string {
	if strings.TrimSpace(candidateStatus) != "" {
		return candidateStatus
	}
	if strings.TrimSpace(defaultStatus) != "" {
		return defaultStatus
	}
	return "probable_success"
}

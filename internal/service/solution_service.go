package service

import (
	"context"

	"aihelper/internal/domain"
	"aihelper/internal/repository"
)

type SolutionService struct {
	repo repository.SolutionRepository
}

func NewSolutionService(repo repository.SolutionRepository) *SolutionService {
	return &SolutionService{repo: repo}
}

func (s *SolutionService) CreateSolution(ctx context.Context, input domain.CreateSolutionInput) (*domain.Solution, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}

	solution := &domain.Solution{
		Title:          input.Title,
		Problem:        input.Problem,
		SolutionText:   input.SolutionText,
		RawContent:     input.RawContent,
		EfficacySource: input.EfficacySource,
		Status:         input.Status,
		SourceType:     input.SourceType,
		SourceName:     input.SourceName,
		SourceRef:      input.SourceRef,
		RawContentRef:  input.RawContentRef,
		Tags:           input.Tags,
		DedupKey:       domain.BuildDedupKey(input),
	}

	if err := s.repo.Create(ctx, solution); err != nil {
		if err == domain.ErrDuplicate {
			existing, lookupErr := s.repo.GetByDedupKey(ctx, solution.DedupKey)
			if lookupErr != nil {
				return nil, lookupErr
			}
			if existing != nil {
				return existing, domain.ErrDuplicate
			}
		}
		return nil, err
	}

	return solution, nil
}

func (s *SolutionService) ListSolutions(ctx context.Context, filter domain.SolutionFilter) ([]domain.Solution, error) {
	return s.repo.List(ctx, filter)
}

func (s *SolutionService) CountSolutions(ctx context.Context, filter domain.SolutionFilter) (int, error) {
	return s.repo.Count(ctx, filter)
}

func (s *SolutionService) UpdateSolution(ctx context.Context, id int64, input domain.UpdateSolutionInput) (*domain.Solution, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, domain.ErrNotFound
	}

	existing.Title = input.Title
	existing.Problem = input.Problem
	existing.SolutionText = input.SolutionText
	existing.Status = input.Status
	existing.Tags = input.Tags
	existing.DedupKey = domain.BuildDedupKey(domain.CreateSolutionInput{
		Title:          existing.Title,
		Problem:        existing.Problem,
		SolutionText:   existing.SolutionText,
		EfficacySource: existing.EfficacySource,
		Status:         existing.Status,
		SourceType:     existing.SourceType,
		SourceName:     existing.SourceName,
		SourceRef:      existing.SourceRef,
		RawContentRef:  existing.RawContentRef,
		Tags:           existing.Tags,
	})

	if err := s.repo.Update(ctx, existing); err != nil {
		if err == domain.ErrDuplicate {
			dup, lookupErr := s.repo.GetByDedupKey(ctx, existing.DedupKey)
			if lookupErr == nil && dup != nil {
				return dup, domain.ErrDuplicate
			}
		}
		return nil, err
	}
	return existing, nil
}

func (s *SolutionService) DeleteSolution(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

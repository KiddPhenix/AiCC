package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidSolution = errors.New("invalid solution")
	ErrDuplicate       = errors.New("duplicate solution")
	ErrNotFound        = errors.New("solution not found")
)

var validEfficacySources = map[string]struct{}{
	"manual": {},
	"auto":   {},
}

var validStatuses = map[string]struct{}{
	"draft":             {},
	"probable_success":  {},
	"confirmed_success": {},
	"failed":            {},
}

type Solution struct {
	ID             int64     `json:"id"`
	Title          string    `json:"title"`
	Problem        string    `json:"problem"`
	SolutionText   string    `json:"solution_text"`
	RawContent     string    `json:"raw_content"`
	EfficacySource string    `json:"efficacy_source"`
	Status         string    `json:"status"`
	SourceType     string    `json:"source_type"`
	SourceName     string    `json:"source_name"`
	SourceRef      string    `json:"source_ref"`
	RawContentRef  string    `json:"raw_content_ref"`
	DedupKey       string    `json:"dedup_key"`
	Tags           []string  `json:"tags"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateSolutionInput struct {
	Title          string   `json:"title"`
	Problem        string   `json:"problem"`
	SolutionText   string   `json:"solution_text"`
	RawContent     string   `json:"raw_content"`
	EfficacySource string   `json:"efficacy_source"`
	Status         string   `json:"status"`
	SourceType     string   `json:"source_type"`
	SourceName     string   `json:"source_name"`
	SourceRef      string   `json:"source_ref"`
	RawContentRef  string   `json:"raw_content_ref"`
	Tags           []string `json:"tags"`
}

type SolutionFilter struct {
	Query          string
	Status         string
	EfficacySource string
	Tag            string
	Limit          int
	Offset         int
}

type UpdateSolutionInput struct {
	Title        string   `json:"title"`
	Problem      string   `json:"problem"`
	SolutionText string   `json:"solution_text"`
	Status       string   `json:"status"`
	Tags         []string `json:"tags"`
}

func (in *CreateSolutionInput) NormalizeAndValidate() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Problem = strings.TrimSpace(in.Problem)
	in.SolutionText = strings.TrimSpace(in.SolutionText)
	in.RawContent = strings.TrimSpace(in.RawContent)
	in.EfficacySource = strings.TrimSpace(strings.ToLower(in.EfficacySource))
	in.Status = strings.TrimSpace(strings.ToLower(in.Status))
	in.SourceType = strings.TrimSpace(strings.ToLower(in.SourceType))
	in.SourceName = strings.TrimSpace(in.SourceName)
	in.SourceRef = strings.TrimSpace(in.SourceRef)
	in.RawContentRef = strings.TrimSpace(in.RawContentRef)
	in.Tags = normalizeTags(in.Tags)

	if in.Title == "" || in.SolutionText == "" {
		return ErrInvalidSolution
	}
	if _, ok := validEfficacySources[in.EfficacySource]; !ok {
		return ErrInvalidSolution
	}
	if _, ok := validStatuses[in.Status]; !ok {
		return ErrInvalidSolution
	}

	return nil
}

func BuildDedupKey(in CreateSolutionInput) string {
	tags := normalizeTags(in.Tags)
	parts := []string{
		normalizeText(in.Title),
		normalizeText(in.Problem),
		normalizeText(in.SolutionText),
		normalizeText(in.SourceType),
		normalizeText(in.SourceName),
		normalizeText(in.SourceRef),
		strings.Join(tags, "|"),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func (in *UpdateSolutionInput) NormalizeAndValidate() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Problem = strings.TrimSpace(in.Problem)
	in.SolutionText = strings.TrimSpace(in.SolutionText)
	in.Status = strings.TrimSpace(strings.ToLower(in.Status))
	in.Tags = normalizeTags(in.Tags)

	if in.Title == "" || in.SolutionText == "" {
		return ErrInvalidSolution
	}
	if _, ok := validStatuses[in.Status]; !ok {
		return ErrInvalidSolution
	}
	return nil
}

func normalizeTags(tags []string) []string {
	set := make(map[string]struct{})
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		set[tag] = struct{}{}
	}

	result := make([]string, 0, len(set))
	for tag := range set {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func normalizeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

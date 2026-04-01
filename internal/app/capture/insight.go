package capture

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Insight struct {
	CaptureID       int64     `json:"capture_id"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	Category        string    `json:"category"`
	Problem         string    `json:"problem"`
	SolutionText    string    `json:"solution_text"`
	OccurredAt      string    `json:"occurred_at"`
	SourceName      string    `json:"source_name"`
	SourceRef       string    `json:"source_ref"`
	SuggestedStatus string    `json:"suggested_status"`
	Confidence      float64   `json:"confidence"`
	Tags            []string  `json:"tags"`
	Model           string    `json:"model"`
	Provider        string    `json:"provider"`
	RawJSON         string    `json:"-"`
	AnalyzedAt      time.Time `json:"analyzed_at"`
}

func (s *Service) GetCapture(ctx context.Context, id int64) (*Result, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, content, content_type, char_count, line_count, source, captured_at
		FROM captures
		WHERE id = ?
	`, id)

	var item Result
	if err := row.Scan(&item.ID, &item.Content, &item.ContentType, &item.CharCount, &item.LineCount, &item.Source, &item.CapturedAt); err != nil {
		return nil, err
	}

	insight, err := s.GetInsight(ctx, id)
	if err != nil {
		return nil, err
	}
	item.Insight = insight
	return &item, nil
}

func (s *Service) GetInsight(ctx context.Context, captureID int64) (*Insight, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			capture_id, title, summary, category, problem, solution_text, occurred_at,
			source_name, source_ref, suggested_status, confidence, tags_json, model,
			provider, raw_json, analyzed_at
		FROM capture_insights
		WHERE capture_id = ?
	`, captureID)

	var insight Insight
	var tagsJSON string
	err := row.Scan(
		&insight.CaptureID, &insight.Title, &insight.Summary, &insight.Category, &insight.Problem,
		&insight.SolutionText, &insight.OccurredAt, &insight.SourceName, &insight.SourceRef,
		&insight.SuggestedStatus, &insight.Confidence, &tagsJSON, &insight.Model,
		&insight.Provider, &insight.RawJSON, &insight.AnalyzedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &insight.Tags)
	return &insight, nil
}

func (s *Service) UpsertInsight(ctx context.Context, insight *Insight) error {
	tagsJSON, err := json.Marshal(normalizeInsightTags(insight.Tags))
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO capture_insights (
			capture_id, title, summary, category, problem, solution_text, occurred_at,
			source_name, source_ref, suggested_status, confidence, tags_json, model,
			provider, raw_json, analyzed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(capture_id) DO UPDATE SET
			title = excluded.title,
			summary = excluded.summary,
			category = excluded.category,
			problem = excluded.problem,
			solution_text = excluded.solution_text,
			occurred_at = excluded.occurred_at,
			source_name = excluded.source_name,
			source_ref = excluded.source_ref,
			suggested_status = excluded.suggested_status,
			confidence = excluded.confidence,
			tags_json = excluded.tags_json,
			model = excluded.model,
			provider = excluded.provider,
			raw_json = excluded.raw_json,
			analyzed_at = CURRENT_TIMESTAMP
	`, insight.CaptureID, insight.Title, insight.Summary, insight.Category, insight.Problem,
		insight.SolutionText, insight.OccurredAt, insight.SourceName, insight.SourceRef,
		insight.SuggestedStatus, insight.Confidence, string(tagsJSON), insight.Model,
		insight.Provider, insight.RawJSON)
	return err
}

func normalizeInsightTags(tags []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := set[tag]; ok {
			continue
		}
		set[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

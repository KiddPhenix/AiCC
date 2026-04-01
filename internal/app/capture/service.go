package capture

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

type Service struct {
	db *sql.DB
}

type Result struct {
	ID          int64     `json:"id"`
	Content     string    `json:"content"`
	ContentType string    `json:"content_type"`
	CharCount   int       `json:"char_count"`
	LineCount   int       `json:"line_count"`
	Source      string    `json:"source"`
	Duplicate   bool      `json:"duplicate"`
	CapturedAt  time.Time `json:"captured_at"`
	Insight     *Insight  `json:"insight,omitempty"`
}

type Filter struct {
	Query        string
	Limit        int
	Offset       int
	AnalyzedOnly *bool
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CaptureClipboard(ctx context.Context, content string) (*Result, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrEmptyClipboard
	}

	hash := hashContent(content)
	contentType := detectContentType(content)
	charCount := len([]rune(content))
	lineCount := countLines(content)

	result := &Result{
		ContentType: contentType,
		CharCount:   charCount,
		LineCount:   lineCount,
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO captures (content, content_hash, content_type, char_count, line_count, source)
		VALUES (?, ?, ?, ?, ?, 'clipboard')
		ON CONFLICT(content_hash) DO NOTHING
		RETURNING id, captured_at
	`, content, hash, contentType, charCount, lineCount)

	if err := row.Scan(&result.ID, &result.CapturedAt); err == nil {
		return result, nil
	}

	row = s.db.QueryRowContext(ctx, `
		SELECT id, captured_at
		FROM captures
		WHERE content_hash = ?
	`, hash)
	if err := row.Scan(&result.ID, &result.CapturedAt); err != nil {
		return nil, err
	}

	result.Duplicate = true
	return result, nil
}

func (s *Service) ListCaptures(ctx context.Context, filter Filter) ([]Result, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	args := []any{}
	query := `
		SELECT
			c.id, c.content, c.content_type, c.char_count, c.line_count, c.source, c.captured_at,
			ci.capture_id, ci.title, ci.summary, ci.category, ci.problem, ci.solution_text,
			ci.occurred_at, ci.source_name, ci.source_ref, ci.suggested_status, ci.confidence,
			ci.tags_json, ci.model, ci.provider, ci.raw_json, ci.analyzed_at
		FROM captures c
		LEFT JOIN capture_insights ci ON ci.capture_id = c.id
	`

	conditions := []string{}
	if filter.AnalyzedOnly != nil {
		if *filter.AnalyzedOnly {
			conditions = append(conditions, "ci.capture_id IS NOT NULL")
		} else {
			conditions = append(conditions, "ci.capture_id IS NULL")
		}
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		conditions = append(conditions, `(LOWER(c.content) LIKE ? OR LOWER(c.content_type) LIKE ? OR LOWER(COALESCE(ci.summary, '')) LIKE ? OR LOWER(COALESCE(ci.category, '')) LIKE ?)`)
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like, like)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, " AND ")
	}

	query += ` ORDER BY c.captured_at DESC, c.id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Result
	for rows.Next() {
		var item Result
		var insightCaptureID sql.NullInt64
		var title, summary, category, problem, solutionText sql.NullString
		var occurredAt, sourceName, sourceRef, suggestedStatus, tagsJSON, model, provider, rawJSON sql.NullString
		var confidence sql.NullFloat64
		var analyzedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.Content, &item.ContentType, &item.CharCount, &item.LineCount, &item.Source, &item.CapturedAt,
			&insightCaptureID, &title, &summary, &category, &problem, &solutionText,
			&occurredAt, &sourceName, &sourceRef, &suggestedStatus, &confidence,
			&tagsJSON, &model, &provider, &rawJSON, &analyzedAt,
		); err != nil {
			return nil, err
		}
		if insightCaptureID.Valid {
			insight := &Insight{
				CaptureID:       insightCaptureID.Int64,
				Title:           title.String,
				Summary:         summary.String,
				Category:        category.String,
				Problem:         problem.String,
				SolutionText:    solutionText.String,
				OccurredAt:      occurredAt.String,
				SourceName:      sourceName.String,
				SourceRef:       sourceRef.String,
				SuggestedStatus: suggestedStatus.String,
				Confidence:      confidence.Float64,
				Model:           model.String,
				Provider:        provider.String,
				RawJSON:         rawJSON.String,
			}
			if analyzedAt.Valid {
				insight.AnalyzedAt = analyzedAt.Time
			}
			if tagsJSON.Valid {
				_ = json.Unmarshal([]byte(tagsJSON.String), &insight.Tags)
			}
			item.Insight = insight
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (s *Service) DeleteCapture(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM captures WHERE id = ?`, id)
	return err
}

func (s *Service) DeleteUnanalyzedCaptures(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM captures
		WHERE id IN (
			SELECT c.id
			FROM captures c
			LEFT JOIN capture_insights ci ON ci.capture_id = c.id
			WHERE ci.capture_id IS NULL
		)
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func countLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func detectContentType(content string) string {
	lower := strings.ToLower(content)

	switch {
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		return "url"
	case strings.Contains(content, "```") || strings.Contains(lower, "package ") || strings.Contains(lower, "func ") || strings.Contains(lower, "class "):
		return "code"
	case strings.Contains(lower, "user:") || strings.Contains(lower, "assistant:") || strings.Contains(lower, "system:"):
		return "chat_transcript"
	case strings.Count(content, "\n") > 8:
		return "long_text"
	default:
		return "text"
	}
}

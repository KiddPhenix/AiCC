package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"aihelper/internal/domain"
)

type SolutionRepository interface {
	Create(ctx context.Context, solution *domain.Solution) error
	List(ctx context.Context, filter domain.SolutionFilter) ([]domain.Solution, error)
	Count(ctx context.Context, filter domain.SolutionFilter) (int, error)
	GetByDedupKey(ctx context.Context, dedupKey string) (*domain.Solution, error)
	GetByID(ctx context.Context, id int64) (*domain.Solution, error)
	Update(ctx context.Context, solution *domain.Solution) error
	Delete(ctx context.Context, id int64) error
}

type SQLiteSolutionRepository struct {
	db *sql.DB
}

func NewSQLiteSolutionRepository(db *sql.DB) *SQLiteSolutionRepository {
	return &SQLiteSolutionRepository{db: db}
}

func (r *SQLiteSolutionRepository) Create(ctx context.Context, solution *domain.Solution) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO solutions (
			title, problem, solution_text, raw_content, efficacy_source, status,
			source_type, source_name, source_ref, raw_content_ref, dedup_key
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, solution.Title, solution.Problem, solution.SolutionText, solution.RawContent, solution.EfficacySource, solution.Status,
		solution.SourceType, solution.SourceName, solution.SourceRef, solution.RawContentRef, solution.DedupKey)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return domain.ErrDuplicate
		}
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	solution.ID = id

	for _, tag := range solution.Tags {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO solution_tags (solution_id, tag) VALUES (?, ?)
		`, id, tag); err != nil {
			return err
		}
	}

	row := tx.QueryRowContext(ctx, `
		SELECT created_at, updated_at
		FROM solutions
		WHERE id = ?
	`, id)
	if err := row.Scan(&solution.CreatedAt, &solution.UpdatedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *SQLiteSolutionRepository) List(ctx context.Context, filter domain.SolutionFilter) ([]domain.Solution, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	args := []any{}
	conditions := []string{"1 = 1"}

	if filter.Status != "" {
		conditions = append(conditions, "s.status = ?")
		args = append(args, filter.Status)
	}
	if filter.EfficacySource != "" {
		conditions = append(conditions, "s.efficacy_source = ?")
		args = append(args, filter.EfficacySource)
	}
	if filter.Tag != "" {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM solution_tags st2 WHERE st2.solution_id = s.id AND st2.tag = ?)")
		args = append(args, strings.ToLower(strings.TrimSpace(filter.Tag)))
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		conditions = append(conditions, `(LOWER(s.title) LIKE ? OR LOWER(s.problem) LIKE ? OR LOWER(s.solution_text) LIKE ? OR EXISTS (
			SELECT 1 FROM solution_tags st3 WHERE st3.solution_id = s.id AND LOWER(st3.tag) LIKE ?
		))`)
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like, like)
	}

	args = append(args, filter.Limit, filter.Offset)
	query := fmt.Sprintf(`
		SELECT
			s.id, s.title, s.problem, s.solution_text, s.raw_content, s.efficacy_source, s.status,
			s.source_type, s.source_name, s.source_ref, s.raw_content_ref, s.dedup_key,
			s.created_at, s.updated_at, COALESCE(GROUP_CONCAT(st.tag, ','), '')
		FROM solutions s
		LEFT JOIN solution_tags st ON st.solution_id = s.id
		WHERE %s
		GROUP BY s.id
		ORDER BY s.updated_at DESC, s.id DESC
		LIMIT ? OFFSET ?
	`, strings.Join(conditions, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var solutions []domain.Solution
	for rows.Next() {
		var s domain.Solution
		var tagsCSV string
		if err := rows.Scan(
			&s.ID, &s.Title, &s.Problem, &s.SolutionText, &s.RawContent, &s.EfficacySource, &s.Status,
			&s.SourceType, &s.SourceName, &s.SourceRef, &s.RawContentRef, &s.DedupKey,
			&s.CreatedAt, &s.UpdatedAt, &tagsCSV,
		); err != nil {
			return nil, err
		}
		s.Tags = splitTags(tagsCSV)
		solutions = append(solutions, s)
	}

	return solutions, rows.Err()
}

func (r *SQLiteSolutionRepository) Count(ctx context.Context, filter domain.SolutionFilter) (int, error) {
	args := []any{}
	conditions := []string{"1 = 1"}

	if filter.Status != "" {
		conditions = append(conditions, "s.status = ?")
		args = append(args, filter.Status)
	}
	if filter.EfficacySource != "" {
		conditions = append(conditions, "s.efficacy_source = ?")
		args = append(args, filter.EfficacySource)
	}
	if filter.Tag != "" {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM solution_tags st2 WHERE st2.solution_id = s.id AND st2.tag = ?)")
		args = append(args, strings.ToLower(strings.TrimSpace(filter.Tag)))
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		conditions = append(conditions, `(LOWER(s.title) LIKE ? OR LOWER(s.problem) LIKE ? OR LOWER(s.solution_text) LIKE ? OR EXISTS (
			SELECT 1 FROM solution_tags st3 WHERE st3.solution_id = s.id AND LOWER(st3.tag) LIKE ?
		))`)
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like, like)
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM solutions s WHERE %s`, strings.Join(conditions, " AND "))
	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *SQLiteSolutionRepository) GetByDedupKey(ctx context.Context, dedupKey string) (*domain.Solution, error) {
	return r.getOne(ctx, "s.dedup_key = ?", dedupKey)
}

func (r *SQLiteSolutionRepository) GetByID(ctx context.Context, id int64) (*domain.Solution, error) {
	return r.getOne(ctx, "s.id = ?", id)
}

func (r *SQLiteSolutionRepository) Update(ctx context.Context, solution *domain.Solution) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE solutions
		SET title = ?, problem = ?, solution_text = ?, status = ?, dedup_key = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, solution.Title, solution.Problem, solution.SolutionText, solution.Status, solution.DedupKey, solution.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return domain.ErrDuplicate
		}
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM solution_tags WHERE solution_id = ?`, solution.ID); err != nil {
		return err
	}
	for _, tag := range solution.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO solution_tags (solution_id, tag) VALUES (?, ?)`, solution.ID, tag); err != nil {
			return err
		}
	}

	row := tx.QueryRowContext(ctx, `SELECT created_at, updated_at FROM solutions WHERE id = ?`, solution.ID)
	if err := row.Scan(&solution.CreatedAt, &solution.UpdatedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *SQLiteSolutionRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM solutions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SQLiteSolutionRepository) getOne(ctx context.Context, condition string, arg any) (*domain.Solution, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			s.id, s.title, s.problem, s.solution_text, s.raw_content, s.efficacy_source, s.status,
			s.source_type, s.source_name, s.source_ref, s.raw_content_ref, s.dedup_key,
			s.created_at, s.updated_at, COALESCE(GROUP_CONCAT(st.tag, ','), '')
		FROM solutions s
		LEFT JOIN solution_tags st ON st.solution_id = s.id
		WHERE `+condition+`
		GROUP BY s.id
	`, arg)

	var s domain.Solution
	var tagsCSV string
	err := row.Scan(
		&s.ID, &s.Title, &s.Problem, &s.SolutionText, &s.RawContent, &s.EfficacySource, &s.Status,
		&s.SourceType, &s.SourceName, &s.SourceRef, &s.RawContentRef, &s.DedupKey,
		&s.CreatedAt, &s.UpdatedAt, &tagsCSV,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.Tags = splitTags(tagsCSV)
	return &s, nil
}

func splitTags(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return []string{}
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

var _ SolutionRepository = (*SQLiteSolutionRepository)(nil)

func parseSQLiteTime(raw string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", raw)
}

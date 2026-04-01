package extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"aihelper/internal/config"
)

var ErrMissingAPIKey = errors.New("OPENAI_API_KEY is not set")
var ErrAnalysisTimeout = errors.New("analysis timeout")

type Analysis struct {
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	Category        string   `json:"category"`
	Problem         string   `json:"problem"`
	SolutionText    string   `json:"solution_text"`
	OccurredAt      string   `json:"occurred_at"`
	SourceName      string   `json:"source_name"`
	SourceRef       string   `json:"source_ref"`
	SuggestedStatus string   `json:"suggested_status"`
	Confidence      float64  `json:"confidence"`
	Tags            []string `json:"tags"`
}

type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func New(cfg config.AIConfig) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	return &Client{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   strings.TrimSpace(cfg.Model),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) AnalyzeCapture(ctx context.Context, content, contentType string, capturedAt time.Time) (*Analysis, string, error) {
	if c.apiKey == "" {
		return nil, "", ErrMissingAPIKey
	}

	systemPrompt := `You extract reusable knowledge from captured clipboard content.
Return only structured data that can help build a personal AI solution library.
First identify what the content actually is before summarizing:
- code snippet
- prompt template
- solution/workaround
- error log
- command or shell step
- article/note
- meeting note
- URL/bookmark
- chat transcript
- plain text
Do not over-compress. Preserve concrete facts, constraints, commands, names, URLs, error messages, and key steps.
Make scripts, commands, file paths, URLs, ports, environment variable names, usernames, tokens, passwords, and other literal operational strings highly visible in summary and solution_text.
If the content includes literal credentials or secrets, preserve them exactly as written instead of redacting, masking, paraphrasing, or omitting them.
When useful, put concrete items on separate lines so they can be scanned at a glance.
Prefer faithful extraction over cleanup. Keep exact spelling, casing, slashes, backslashes, flags, URLs, query strings, and code fences when they matter.
Use Chinese for title, summary, problem and solution_text when possible.
Summary should be a faithful structured digest, not a vague short paraphrase.
If the content is already a useful artifact such as code, prompt, or command, solution_text should preserve the actionable core rather than rewriting it abstractly.
If there are one or more scripts, commands, paths, URLs, passwords, or config values, include them explicitly in summary and/or solution_text instead of hiding them inside abstract prose.
Category should be a short stable label like prompt_engineering, coding, writing, research, sql, debugging, workflow, meeting, note, url, error_log, command, chat_transcript, general.
occurred_at should be an RFC3339 timestamp if clearly inferable, otherwise empty string.
source_name and source_ref are optional and may be empty.
suggested_status must be one of: draft, probable_success, confirmed_success, failed.
confidence must be between 0 and 1.
tags should be short lowercase tags.`

	userPrompt := fmt.Sprintf("Captured at: %s\nContent type: %s\nCaptured content:\n%s", capturedAt.Format(time.RFC3339), contentType, content)
	reqBody := map[string]any{
		"model": c.model,
		"input": []map[string]any{
			{
				"role": "system",
				"content": []map[string]any{
					{"type": "input_text", "text": systemPrompt},
				},
			},
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": userPrompt},
				},
			},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "capture_analysis",
				"strict": true,
				"schema": analysisSchema(),
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, "", ErrAnalysisTimeout
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, "", ErrAnalysisTimeout
		}
		return nil, "", err
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 300 {
		return nil, string(rawResp), fmt.Errorf("openai api error: status %d", resp.StatusCode)
	}

	var parsed responseEnvelope
	if err := json.Unmarshal(rawResp, &parsed); err != nil {
		return nil, string(rawResp), err
	}

	outputText := extractOutputText(parsed)
	if strings.TrimSpace(outputText) == "" {
		return nil, string(rawResp), errors.New("empty model output")
	}

	var result Analysis
	if err := json.Unmarshal([]byte(outputText), &result); err != nil {
		return nil, string(rawResp), err
	}

	normalizeAnalysis(&result)
	return &result, string(rawResp), nil
}

type responseEnvelope struct {
	Output []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func extractOutputText(resp responseEnvelope) string {
	var parts []string
	for _, item := range resp.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" || content.Type == "text" {
				parts = append(parts, content.Text)
			}
		}
	}
	return strings.Join(parts, "")
}

func analysisSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":            map[string]any{"type": "string"},
			"summary":          map[string]any{"type": "string"},
			"category":         map[string]any{"type": "string"},
			"problem":          map[string]any{"type": "string"},
			"solution_text":    map[string]any{"type": "string"},
			"occurred_at":      map[string]any{"type": "string"},
			"source_name":      map[string]any{"type": "string"},
			"source_ref":       map[string]any{"type": "string"},
			"suggested_status": map[string]any{"type": "string", "enum": []string{"draft", "probable_success", "confirmed_success", "failed"}},
			"confidence":       map[string]any{"type": "number"},
			"tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required":             []string{"title", "summary", "category", "problem", "solution_text", "occurred_at", "source_name", "source_ref", "suggested_status", "confidence", "tags"},
		"additionalProperties": false,
	}
}

func normalizeAnalysis(a *Analysis) {
	a.Title = strings.TrimSpace(a.Title)
	a.Summary = strings.TrimSpace(a.Summary)
	a.Category = strings.TrimSpace(strings.ToLower(a.Category))
	a.Problem = strings.TrimSpace(a.Problem)
	a.SolutionText = strings.TrimSpace(a.SolutionText)
	a.OccurredAt = strings.TrimSpace(a.OccurredAt)
	a.SourceName = strings.TrimSpace(a.SourceName)
	a.SourceRef = strings.TrimSpace(a.SourceRef)
	a.SuggestedStatus = strings.TrimSpace(strings.ToLower(a.SuggestedStatus))
	if a.Confidence < 0 {
		a.Confidence = 0
	}
	if a.Confidence > 1 {
		a.Confidence = 1
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a.Tags))
	for _, tag := range a.Tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	a.Tags = out
}

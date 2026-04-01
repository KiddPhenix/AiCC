package source

import "context"

type Adapter interface {
	Name() string
	Import(ctx context.Context, req ImportRequest) ([]Candidate, error)
}

type Candidate struct {
	Title         string   `json:"title"`
	Problem       string   `json:"problem"`
	SolutionText  string   `json:"solution_text"`
	SourceType    string   `json:"source_type"`
	SourceName    string   `json:"source_name"`
	SourceRef     string   `json:"source_ref"`
	RawContentRef string   `json:"raw_content_ref"`
	Tags          []string `json:"tags"`
	Status        string   `json:"status"`
}

type ImportRequest struct {
	Path string `json:"path"`
}

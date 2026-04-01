package localjson

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"aihelper/internal/source"
)

type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return "local_json"
}

func (a *Adapter) Import(_ context.Context, req source.ImportRequest) ([]source.Candidate, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var asArray []source.Candidate
	if err := json.NewDecoder(file).Decode(&asArray); err == nil {
		return asArray, nil
	}

	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	var items []source.Candidate
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item source.Candidate
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("invalid json line: %w", err)
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

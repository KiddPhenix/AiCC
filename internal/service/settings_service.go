package service

import "aihelper/internal/config"

type SettingsService struct {
	path string
}

func NewSettingsService(path string) *SettingsService {
	return &SettingsService{path: path}
}

type SettingsAI struct {
	BaseURL   *string `json:"base_url,omitempty"`
	APIKey    *string `json:"api_key,omitempty"`
	Model     *string `json:"model,omitempty"`
	HasAPIKey bool    `json:"has_api_key"`
}

type SettingsCapture struct {
	AutoAnalyze bool `json:"auto_analyze"`
}

type SettingsDTO struct {
	AI      SettingsAI      `json:"ai"`
	Capture SettingsCapture `json:"capture"`
}

func (s *SettingsService) Get() (*SettingsDTO, error) {
	cfg, err := config.Load(s.path)
	if err != nil {
		return nil, err
	}
	dto := &SettingsDTO{}
	dto.AI.BaseURL = &cfg.AI.BaseURL
	dto.AI.APIKey = &cfg.AI.APIKey
	dto.AI.Model = &cfg.AI.Model
	dto.AI.HasAPIKey = cfg.AI.APIKey != ""
	dto.Capture.AutoAnalyze = cfg.Capture.AutoAnalyze
	return dto, nil
}

func (s *SettingsService) Update(input SettingsDTO) (*SettingsDTO, error) {
	cfg, err := config.Load(s.path)
	if err != nil {
		return nil, err
	}
	if input.AI.BaseURL != nil {
		cfg.AI.BaseURL = *input.AI.BaseURL
	}
	if input.AI.APIKey != nil {
		cfg.AI.APIKey = *input.AI.APIKey
	}
	if input.AI.Model != nil {
		cfg.AI.Model = *input.AI.Model
	}
	cfg.Capture.AutoAnalyze = input.Capture.AutoAnalyze
	if err := config.Save(s.path, cfg); err != nil {
		return nil, err
	}
	return s.Get()
}

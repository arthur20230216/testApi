package service

import (
	"context"
	"strconv"
	"strings"

	"modelprobe/backend/internal/model"
	"modelprobe/backend/internal/repository"
)

type SystemSettingsDefaults struct {
	ChannelAuditEnabled   bool
	ChannelAuditTimeoutMS int
	OpenAIAPIKey          string
	OpenAIModel           string
	OpenAIBaseURL         string
}

type SystemSettingsService struct {
	repo     *repository.PostgresRepository
	defaults SystemSettingsDefaults
}

func NewSystemSettingsService(repo *repository.PostgresRepository, defaults SystemSettingsDefaults) *SystemSettingsService {
	return &SystemSettingsService{repo: repo, defaults: defaults}
}

func (s *SystemSettingsService) Load(ctx context.Context) (model.SystemSettingsRecord, error) {
	values, err := s.repo.GetSystemSettings(ctx)
	if err != nil {
		return model.SystemSettingsRecord{}, err
	}

	record := model.SystemSettingsRecord{
		ChannelAuditEnabled:   boolFromSetting(values["channel_audit_enabled"], s.defaults.ChannelAuditEnabled),
		ChannelAuditTimeoutMS: intFromSetting(values["channel_audit_timeout_ms"], s.defaults.ChannelAuditTimeoutMS),
		OpenAIAPIKey:          firstNonEmpty(values["openai_api_key"], s.defaults.OpenAIAPIKey),
		OpenAIModel:           firstNonEmpty(values["openai_model"], s.defaults.OpenAIModel),
		OpenAIBaseURL:         firstNonEmpty(values["openai_base_url"], s.defaults.OpenAIBaseURL),
	}

	return record, nil
}

func (s *SystemSettingsService) GetResponse(ctx context.Context) (model.SystemSettingsResponse, error) {
	record, err := s.Load(ctx)
	if err != nil {
		return model.SystemSettingsResponse{}, err
	}

	return model.SystemSettingsResponse{
		ChannelAuditEnabled:    record.ChannelAuditEnabled,
		ChannelAuditTimeoutMS:  record.ChannelAuditTimeoutMS,
		OpenAIAPIKeyMasked:     maskOptionalSecret(record.OpenAIAPIKey),
		OpenAIAPIKeyConfigured: strings.TrimSpace(record.OpenAIAPIKey) != "",
		OpenAIModel:            record.OpenAIModel,
		OpenAIBaseURL:          record.OpenAIBaseURL,
	}, nil
}

func (s *SystemSettingsService) Update(ctx context.Context, request model.SystemSettingsUpdateRequest) (model.SystemSettingsResponse, error) {
	values := map[string]*string{
		"channel_audit_enabled":    ptr(strconv.FormatBool(request.ChannelAuditEnabled)),
		"channel_audit_timeout_ms": ptr(strconv.Itoa(request.ChannelAuditTimeoutMS)),
		"openai_model":             ptr(strings.TrimSpace(request.OpenAIModel)),
		"openai_base_url":          ptr(strings.TrimSpace(request.OpenAIBaseURL)),
	}

	if request.ClearOpenAIAPIKey {
		values["openai_api_key"] = nil
	} else if strings.TrimSpace(request.OpenAIAPIKey) != "" {
		values["openai_api_key"] = ptr(strings.TrimSpace(request.OpenAIAPIKey))
	}

	for key, value := range values {
		if value != nil && strings.TrimSpace(*value) == "" {
			if key == "openai_model" || key == "openai_base_url" {
				values[key] = nil
			}
		}
	}

	if err := s.repo.UpsertSystemSettings(ctx, values); err != nil {
		return model.SystemSettingsResponse{}, err
	}

	return s.GetResponse(ctx)
}

func boolFromSetting(raw string, fallback bool) bool {
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func intFromSetting(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maskOptionalSecret(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 6 {
		return trimmed[:1] + "***"
	}
	return trimmed[:3] + "***" + trimmed[len(trimmed)-3:]
}

func ptr(value string) *string {
	return &value
}

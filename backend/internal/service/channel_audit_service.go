package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"modelprobe/backend/internal/model"
)

const channelAuditSystemPrompt = `You are an AI channel authenticity auditor.

Your job is not to judge usability. Your only job is to judge whether the claimed channel is credible based on technical evidence.

You must strictly evaluate evidence and never trust branding or marketing text.

Core goals:
1. Decide whether the claimedChannel matches the observed model pool and endpoint behavior.
2. Detect wrapper behavior, mixed provider pools, model relabeling, or channel-name impersonation.

Rules:
- Focus on API behavior, model IDs, response format, headers, error semantics, endpoint patterns, and configured channel-model allowlist.
- Separate model authenticity from channel authenticity. A model can look real while the claimed channel is still misleading.
- Penalize generic OpenAI-compatible wrappers that do not prove channel identity.
- Penalize mixed pools exposed under a single claimed channel.
- If evidence is insufficient, return needs_review and explain what evidence is missing.
- Do not speculate beyond the provided evidence.

Return strict JSON only.`

type ChannelAuditService struct {
	settingsService *SystemSettingsService
}

func NewChannelAuditService(settingsService *SystemSettingsService) *ChannelAuditService {
	return &ChannelAuditService{
		settingsService: settingsService,
	}
}

func (s *ChannelAuditService) Enabled() bool {
	return s != nil && s.settingsService != nil
}

func (s *ChannelAuditService) Audit(
	ctx context.Context,
	input model.ProbeRequest,
	attempt probeAttempt,
	modelIDs []string,
	families []string,
	compatibility bool,
	channelModels map[string][]string,
	ruleBasedScore int,
	ruleBasedVerdict string,
	suspicionReasons []string,
	notes []string,
) (*model.ChannelAuditResult, error) {
	if s == nil {
		return nil, nil
	}
	settings, err := s.settingsService.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load channel audit settings: %w", err)
	}
	if !settings.ChannelAuditEnabled {
		return nil, nil
	}
	if strings.TrimSpace(settings.OpenAIAPIKey) == "" {
		return nil, fmt.Errorf("channel audit is enabled but OpenAI API key is empty")
	}
	if strings.TrimSpace(settings.OpenAIModel) == "" {
		return nil, fmt.Errorf("channel audit is enabled but OpenAI model is empty")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(settings.OpenAIBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	client := &http.Client{
		Timeout: time.Duration(settings.ChannelAuditTimeoutMS) * time.Millisecond,
	}

	requestBody := map[string]any{
		"model": settings.OpenAIModel,
		"input": []map[string]any{
			{
				"role": "system",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": channelAuditSystemPrompt,
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": buildChannelAuditUserPrompt(input, attempt, modelIDs, families, compatibility, channelModels, ruleBasedScore, ruleBasedVerdict, suspicionReasons, notes),
					},
				},
			},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "channel_auth_audit",
				"strict": true,
				"schema": channelAuditJSONSchema(),
			},
		},
	}

	payloadBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal channel audit request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("build channel audit request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+settings.OpenAIAPIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call channel audit model: %w", err)
	}
	defer response.Body.Close()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read channel audit response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("channel audit model returned HTTP %d: %s", response.StatusCode, truncateText(string(bodyBytes), 400))
	}

	jsonText, err := extractResponseOutputText(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("extract channel audit output: %w", err)
	}

	var result model.ChannelAuditResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		return nil, fmt.Errorf("decode channel audit JSON: %w", err)
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validate channel audit JSON: %w", err)
	}

	return &result, nil
}

func buildChannelAuditUserPrompt(
	input model.ProbeRequest,
	attempt probeAttempt,
	modelIDs []string,
	families []string,
	compatibility bool,
	channelModels map[string][]string,
	ruleBasedScore int,
	ruleBasedVerdict string,
	suspicionReasons []string,
	notes []string,
) string {
	payload := map[string]any{
		"stationName":          strings.TrimSpace(input.StationName),
		"groupName":            strings.TrimSpace(input.GroupName),
		"baseUrl":              normalizeBaseURL(input.BaseURL),
		"claimedChannel":       strings.TrimSpace(input.ClaimedChannel),
		"expectedModelFamily":  strings.TrimSpace(input.ExpectedModelFamily),
		"detectedEndpoint":     attempt.Endpoint,
		"httpStatus":           attempt.Status,
		"responseTimeMs":       attempt.ResponseTimeMS,
		"isOpenAICompatible":   compatibility,
		"primaryFamily":        valueOrEmpty(inferPrimaryFamily(families)),
		"detectedFamilies":     families,
		"modelIds":             modelIDs,
		"responseHeaders":      attempt.Headers,
		"errorMessage":         valueOrEmpty(attempt.ErrorMessage),
		"rawExcerpt":           truncateText(attempt.BodyText, 1200),
		"ruleBasedScore":       ruleBasedScore,
		"ruleBasedVerdict":     ruleBasedVerdict,
		"ruleSuspicionReasons": suspicionReasons,
		"ruleNotes":            notes,
		"channelModelMap":      channelModels,
	}

	serialized, _ := json.MarshalIndent(payload, "", "  ")

	return strings.TrimSpace(`Audit the authenticity of the claimed channel using the evidence below.

Questions you must answer:
1. Does claimedChannel match the observed model pool?
2. Does claimedChannel match endpoint behavior and response shape?
3. Does this look like a generic OpenAI wrapper instead of a real channel identity?
4. Is this likely a mixed-provider pool presented as one channel?
5. What is the final channel authenticity verdict and score?

Evidence:
` + string(serialized) + `

Return JSON only.`)
}

func channelAuditJSONSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"channelVerdict": map[string]any{
				"type": "string",
				"enum": []string{"trusted", "needs_review", "high_risk"},
			},
			"channelScore": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 100,
			},
			"confidence": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 100,
			},
			"summary": map[string]any{
				"type": "string",
			},
			"supportingSignals": stringArraySchema(),
			"riskSignals":       stringArraySchema(),
			"missingEvidence":   stringArraySchema(),
			"channelConsistency": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"claimedChannelMatchesModelPool":        map[string]any{"type": "boolean"},
					"claimedChannelMatchesEndpointBehavior": map[string]any{"type": "boolean"},
					"claimedChannelMatchesErrorStyle":       map[string]any{"type": "boolean"},
					"isLikelyGenericOpenAIWrapper":          map[string]any{"type": "boolean"},
					"isLikelyMixedProviderPool":             map[string]any{"type": "boolean"},
				},
				"required": []string{
					"claimedChannelMatchesModelPool",
					"claimedChannelMatchesEndpointBehavior",
					"claimedChannelMatchesErrorStyle",
					"isLikelyGenericOpenAIWrapper",
					"isLikelyMixedProviderPool",
				},
			},
			"reasoning": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"modelPoolAssessment":  map[string]any{"type": "string"},
					"endpointAssessment":   map[string]any{"type": "string"},
					"errorStyleAssessment": map[string]any{"type": "string"},
					"finalAssessment":      map[string]any{"type": "string"},
				},
				"required": []string{
					"modelPoolAssessment",
					"endpointAssessment",
					"errorStyleAssessment",
					"finalAssessment",
				},
			},
		},
		"required": []string{
			"channelVerdict",
			"channelScore",
			"confidence",
			"summary",
			"supportingSignals",
			"riskSignals",
			"missingEvidence",
			"channelConsistency",
			"reasoning",
		},
	}
}

func stringArraySchema() map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "string",
		},
	}
}

func extractResponseOutputText(body []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse responses payload: %w", err)
	}

	if outputText, ok := payload["output_text"].(string); ok && strings.TrimSpace(outputText) != "" {
		return outputText, nil
	}

	output, ok := payload["output"].([]any)
	if !ok {
		return "", fmt.Errorf("responses payload has no output")
	}

	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		contentItems, ok := itemMap["content"].([]any)
		if !ok {
			continue
		}
		for _, contentItem := range contentItems {
			contentMap, ok := contentItem.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := contentMap["text"].(string); ok && strings.TrimSpace(text) != "" {
				return text, nil
			}
			if textMap, ok := contentMap["text"].(map[string]any); ok {
				if value, ok := textMap["value"].(string); ok && strings.TrimSpace(value) != "" {
					return value, nil
				}
			}
		}
	}

	return "", fmt.Errorf("responses payload contains no text content")
}

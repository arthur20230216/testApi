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

const probeAuditSystemPrompt = `You are an API authenticity auditor.

Your job is to judge two things separately:
1. model authenticity: whether the endpoint appears to route the requested model honestly, rather than silently relabeling, falling back, or mixing providers
2. channel authenticity: whether the claimed channel identity is actually supported by the technical evidence

Rules:
- Never trust branding text, self-report alone, or marketing claims.
- Use strongest evidence first: completion response model field, invalid-model error behavior, model list, headers, endpoint paths, and prompt-response behavior.
- A technically usable generic OpenAI-compatible wrapper is not enough to prove the claimed channel identity.
- Distinguish insufficient evidence from contradictory evidence. If uncertain, return needs_review and list missing evidence.
- Be strict about mixed-provider pools, model relabeling, and claimed-channel mismatch.
- Return JSON only and follow the schema exactly.`

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
	evidence []model.ProbeEvidenceStep,
	auditContext probeAuditContext,
	channelModels map[string][]string,
) (*model.ProbeAuditResult, error) {
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
						"text": probeAuditSystemPrompt,
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": buildProbeAuditUserPrompt(input, evidence, auditContext, channelModels),
					},
				},
			},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "probe_auth_audit",
				"strict": true,
				"schema": probeAuditJSONSchema(),
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

	var result model.ProbeAuditResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		return nil, fmt.Errorf("decode channel audit JSON: %w", err)
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validate channel audit JSON: %w", err)
	}

	return &result, nil
}

func buildProbeAuditUserPrompt(
	input model.ProbeRequest,
	evidence []model.ProbeEvidenceStep,
	auditContext probeAuditContext,
	channelModels map[string][]string,
) string {
	payload := map[string]any{
		"stationName":         strings.TrimSpace(input.StationName),
		"groupName":           strings.TrimSpace(input.GroupName),
		"baseUrl":             normalizeBaseURL(input.BaseURL),
		"claimedChannel":      strings.TrimSpace(input.ClaimedChannel),
		"expectedModelFamily": strings.TrimSpace(input.ExpectedModelFamily),
		"channelAllowlist":    channelModels[strings.ToLower(strings.TrimSpace(input.ClaimedChannel))],
		"channelModelMap":     channelModels,
		"observations": map[string]any{
			"availableModelIds":       auditContext.AvailableModelIDs,
			"completionResponseModel": valueOrEmpty(auditContext.CompletionResponseModel),
			"completionAssistantText": truncateText(valueOrEmpty(auditContext.CompletionAssistantText), 900),
			"completionFinishReason":  valueOrEmpty(auditContext.CompletionFinishReason),
			"systemFingerprint":       valueOrEmpty(auditContext.SystemFingerprint),
			"detectedFamilies":        auditContext.DetectedFamilies,
			"primaryFamily":           valueOrEmpty(auditContext.PrimaryFamily),
			"isOpenAICompatible":      auditContext.IsOpenAICompatible,
		},
		"ruleBasedAnalysis": map[string]any{
			"score":            auditContext.RuleBasedScore,
			"verdict":          auditContext.RuleBasedVerdict,
			"suspicionReasons": auditContext.SuspicionReasons,
			"notes":            auditContext.Notes,
		},
		"evidenceSteps": evidence,
	}

	serialized, _ := json.MarshalIndent(payload, "", "  ")

	return strings.TrimSpace(`Audit the target endpoint using the evidence below.

You must answer both tracks:
1. Model authenticity:
- Does the endpoint appear to route the requested model honestly?
- Is there evidence of fallback, relabeling, mixed routing, or fake identity?
- How strong is the evidence from completion response model, prompt response, model list, and invalid-model behavior?

2. Channel authenticity:
- Does claimedChannel match the exposed model pool?
- Does claimedChannel match endpoint behavior and error semantics?
- Does this look like a generic wrapper or a mixed-provider pool instead of a real channel identity?

Return:
- one verdict/score/confidence/summary for model authenticity
- one verdict/score/confidence/summary for channel authenticity
- supporting signals, risk signals, and missing evidence for each track
- concise reasoning fields

Evidence:
` + string(serialized) + `

Return JSON only.`)
}

func probeAuditJSONSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"modelVerdict": map[string]any{
				"type": "string",
				"enum": []string{"trusted", "needs_review", "high_risk"},
			},
			"modelScore": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 100,
			},
			"modelConfidence": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 100,
			},
			"modelSummary":           map[string]any{"type": "string"},
			"modelSupportingSignals": stringArraySchema(),
			"modelRiskSignals":       stringArraySchema(),
			"modelMissingEvidence":   stringArraySchema(),
			"modelReasoning":         modelReasoningSchema(),
			"channelVerdict": map[string]any{
				"type": "string",
				"enum": []string{"trusted", "needs_review", "high_risk"},
			},
			"channelScore": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 100,
			},
			"channelConfidence":        map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
			"channelSummary":           map[string]any{"type": "string"},
			"channelSupportingSignals": stringArraySchema(),
			"channelRiskSignals":       stringArraySchema(),
			"channelMissingEvidence":   stringArraySchema(),
			"channelConsistency":       channelConsistencySchema(),
			"channelReasoning":         channelReasoningSchema(),
		},
		"required": []string{
			"modelVerdict",
			"modelScore",
			"modelConfidence",
			"modelSummary",
			"modelSupportingSignals",
			"modelRiskSignals",
			"modelMissingEvidence",
			"modelReasoning",
			"channelVerdict",
			"channelScore",
			"channelConfidence",
			"channelSummary",
			"channelSupportingSignals",
			"channelRiskSignals",
			"channelMissingEvidence",
			"channelConsistency",
			"channelReasoning",
		},
	}
}

func modelReasoningSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"expectedModelAssessment": map[string]any{"type": "string"},
			"outputModelAssessment":   map[string]any{"type": "string"},
			"capabilityAssessment":    map[string]any{"type": "string"},
			"finalAssessment":         map[string]any{"type": "string"},
		},
		"required": []string{
			"expectedModelAssessment",
			"outputModelAssessment",
			"capabilityAssessment",
			"finalAssessment",
		},
	}
}

func channelConsistencySchema() map[string]any {
	return map[string]any{
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
	}
}

func channelReasoningSchema() map[string]any {
	return map[string]any{
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

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"modelprobe/backend/internal/model"
)

const probeUserAgent = "model-probe-go/1.0"

type ProbeService struct {
	client              *http.Client
	timeout             time.Duration
	channelAuditService *ChannelAuditService
}

type probeStepSpec struct {
	Kind        string
	Label       string
	Method      string
	Endpoint    string
	RequestBody *string
}

type probeAttempt struct {
	Kind           string
	Label          string
	Method         string
	Endpoint       string
	RequestBody    *string
	Status         *int
	ResponseTimeMS int
	Headers        map[string]string
	BodyText       string
	BodyJSON       any
	ErrorMessage   *string
}

type completionObservation struct {
	ResponseModel         *string
	AssistantText         *string
	FinishReason          *string
	SystemFingerprint     *string
	PromptTokens          *int
	CompletionTokens      *int
	TotalTokens           *int
	HasChoices            bool
	ProviderGuess         *string
	ModelFamilyGuess      *string
	ChannelIdentity       *string
	Confidence            *int
	AlternativeCandidates []completionCandidate
	EvidenceMarkers       []string
	RoutingRisk           *string
	Sentinel              *string
	MathResult            *string
	BilingualStyle        *string
	CodePatchStyle        *string
	HonestyCheck          *string
	SelfReportParsed      bool
	SelfReportValid       bool
	SelfReportIssues      []string
}

type probeAuditContext struct {
	AvailableModelIDs       []string
	CompletionResponseModel *string
	CompletionAssistantText *string
	CompletionFinishReason  *string
	SystemFingerprint       *string
	DetectedFamilies        []string
	PrimaryFamily           *string
	IsOpenAICompatible      bool
	RuleBasedScore          int
	RuleBasedVerdict        string
	SuspicionReasons        []string
	Notes                   []string
	ProviderGuess           *string
	ModelFamilyGuess        *string
	ChannelIdentityGuess    *string
	SelfReportConfidence    *int
	AlternativeCandidates   []completionCandidate
	EvidenceMarkers         []string
	RoutingRisk             *string
	MathResult              *string
	BilingualStyle          *string
	CodePatchStyle          *string
	HonestyCheck            *string
}

type completionCandidate struct {
	Family string `json:"family"`
	Reason string `json:"reason"`
}

type familyRule struct {
	Family   string
	Patterns []*regexp.Regexp
}

var modelFamilyRules = []familyRule{
	{Family: "claude", Patterns: compilePatterns(`claude`, `sonnet`, `haiku`, `opus`, `anthropic`)},
	{Family: "gpt", Patterns: compilePatterns(`\bgpt[-\d]`, `\bo1\b`, `\bo3\b`, `\bo4\b`, `openai`)},
	{Family: "glm", Patterns: compilePatterns(`glm`, `zhipu`, `chatglm`)},
	{Family: "qwen", Patterns: compilePatterns(`qwen`, `tongyi`)},
	{Family: "deepseek", Patterns: compilePatterns(`deepseek`)},
	{Family: "gemini", Patterns: compilePatterns(`gemini`)},
	{Family: "mistral", Patterns: compilePatterns(`mistral`, `mixtral`)},
	{Family: "llama", Patterns: compilePatterns(`llama`, `meta[- ]?llama`)},
	{Family: "kimi", Patterns: compilePatterns(`kimi`, `moonshot`)},
	{Family: "kiro", Patterns: compilePatterns(`kiro`)},
	{Family: "antigravity", Patterns: compilePatterns(`anti[-_ ]?gravity`)},
}

var clearClaimMap = map[string]string{
	"cc":          "claude",
	"claude":      "claude",
	"claude code": "claude",
	"anthropic":   "claude",
	"gpt":         "gpt",
	"openai":      "gpt",
	"glm":         "glm",
	"zhipu":       "glm",
	"qwen":        "qwen",
	"deepseek":    "deepseek",
	"gemini":      "gemini",
	"mistral":     "mistral",
	"llama":       "llama",
	"kimi":        "kimi",
	"codex":       "gpt",
}

var counterfeitFamilies = []string{
	"kiro",
	"antigravity",
	"glm",
	"qwen",
	"deepseek",
	"gemini",
	"mistral",
	"llama",
	"kimi",
}

func NewProbeService(timeout time.Duration, channelAuditService *ChannelAuditService) *ProbeService {
	return &ProbeService{
		client: &http.Client{
			Timeout: timeout + 2*time.Second,
		},
		timeout:             timeout,
		channelAuditService: channelAuditService,
	}
}

func (s *ProbeService) RunProbe(ctx context.Context, input model.ProbeRequest, channelModels map[string][]string) (model.ProbeRecord, error) {
	modelAttempts := make([]probeAttempt, 0, 2)
	for _, endpoint := range buildModelEndpoints(input.BaseURL) {
		modelAttempts = append(modelAttempts, s.attemptProbe(ctx, probeStepSpec{
			Kind:     "model_list",
			Label:    "List models",
			Method:   http.MethodGet,
			Endpoint: endpoint,
		}, input.APIKey))
	}

	validCompletionBody := buildCompletionProbeBody(strings.TrimSpace(input.ExpectedModelFamily))
	completionAttempts := make([]probeAttempt, 0, 2)
	chatEndpoints := buildChatCompletionEndpoints(input.BaseURL)
	for _, endpoint := range chatEndpoints {
		completionAttempts = append(completionAttempts, s.attemptProbe(ctx, probeStepSpec{
			Kind:        "completion_valid_model",
			Label:       "Completion with expected model",
			Method:      http.MethodPost,
			Endpoint:    endpoint,
			RequestBody: &validCompletionBody,
		}, input.APIKey))
	}

	invalidCompletionBody := buildInvalidModelProbeBody()
	invalidModelAttempts := make([]probeAttempt, 0, len(chatEndpoints))
	for _, endpoint := range chatEndpoints {
		invalidModelAttempts = append(invalidModelAttempts, s.attemptProbe(ctx, probeStepSpec{
			Kind:        "completion_invalid_model",
			Label:       "Completion with invalid model",
			Method:      http.MethodPost,
			Endpoint:    endpoint,
			RequestBody: &invalidCompletionBody,
		}, input.APIKey))
	}

	allAttempts := make([]probeAttempt, 0, len(modelAttempts)+len(completionAttempts)+len(invalidModelAttempts))
	allAttempts = append(allAttempts, modelAttempts...)
	allAttempts = append(allAttempts, completionAttempts...)
	allAttempts = append(allAttempts, invalidModelAttempts...)
	evidence := buildEvidenceSteps(allAttempts)

	bestModelAttempt := pickBestModelAttempt(modelAttempts)
	bestCompletionAttempt := pickBestCompletionAttempt(completionAttempts)
	bestInvalidAttempt := pickBestInvalidModelAttempt(invalidModelAttempts)
	completion := extractCompletionObservation(bestCompletionAttempt.BodyJSON)

	availableModelIDs := collectModelIDs(bestModelAttempt.BodyJSON)
	allObservedModelIDs := append([]string{}, availableModelIDs...)
	if completion.ResponseModel != nil {
		allObservedModelIDs = append(allObservedModelIDs, *completion.ResponseModel)
	}
	allObservedModelIDs = uniqueStrings(allObservedModelIDs)

	families := detectFamilies(buildFamilyEvidenceValues(allAttempts, allObservedModelIDs, completion))
	compatibility := isOpenAICompatible(bestModelAttempt.BodyJSON, len(availableModelIDs) > 0) || isChatCompletionCompatible(bestCompletionAttempt.BodyJSON, completion)
	ruleScore, status, ruleVerdict, suspicionReasons, notes := scoreProbe(
		bestModelAttempt,
		bestCompletionAttempt,
		bestInvalidAttempt,
		availableModelIDs,
		completion,
		families,
		compatibility,
		nullableString(strings.TrimSpace(input.ClaimedChannel)),
		nullableString(strings.TrimSpace(input.ExpectedModelFamily)),
		channelModels,
	)

	auditContext := probeAuditContext{
		AvailableModelIDs:       availableModelIDs,
		CompletionResponseModel: completion.ResponseModel,
		CompletionAssistantText: completion.AssistantText,
		CompletionFinishReason:  completion.FinishReason,
		SystemFingerprint:       completion.SystemFingerprint,
		DetectedFamilies:        families,
		PrimaryFamily:           inferPrimaryFamily(families),
		IsOpenAICompatible:      compatibility,
		RuleBasedScore:          ruleScore,
		RuleBasedVerdict:        ruleVerdict,
		SuspicionReasons:        suspicionReasons,
		Notes:                   notes,
		ProviderGuess:           completion.ProviderGuess,
		ModelFamilyGuess:        completion.ModelFamilyGuess,
		ChannelIdentityGuess:    completion.ChannelIdentity,
		SelfReportConfidence:    completion.Confidence,
		AlternativeCandidates:   completion.AlternativeCandidates,
		EvidenceMarkers:         completion.EvidenceMarkers,
		RoutingRisk:             completion.RoutingRisk,
		MathResult:              completion.MathResult,
		BilingualStyle:          completion.BilingualStyle,
		CodePatchStyle:          completion.CodePatchStyle,
		HonestyCheck:            completion.HonestyCheck,
	}

	auditResult, auditError := s.runAudit(ctx, input, evidence, auditContext, channelModels)
	trustScore, verdict := mergeProbeVerdict(ruleScore, ruleVerdict, auditResult)

	primaryAttempt := selectPrimaryAttempt(bestCompletionAttempt, bestModelAttempt, bestInvalidAttempt)
	responseHeaders := primaryAttempt.Headers
	if responseHeaders == nil {
		responseHeaders = map[string]string{}
	}

	rawExcerpt := buildPrimaryRawExcerpt(bestCompletionAttempt, bestModelAttempt, bestInvalidAttempt)
	record := model.ProbeRecord{
		ID:                  newID(),
		CreatedAt:           time.Now().UTC().Format(time.RFC3339Nano),
		StationName:         strings.TrimSpace(input.StationName),
		GroupName:           nullableString(strings.TrimSpace(input.GroupName)),
		BaseURL:             normalizeBaseURL(input.BaseURL),
		APIKeyHash:          sha256Hex(input.APIKey),
		APIKeyMasked:        maskAPIKey(input.APIKey),
		ClaimedChannel:      nullableString(strings.TrimSpace(input.ClaimedChannel)),
		ExpectedModelFamily: nullableString(strings.TrimSpace(input.ExpectedModelFamily)),
		Status:              status,
		RuleBasedScore:      ruleScore,
		RuleBasedVerdict:    ruleVerdict,
		TrustScore:          trustScore,
		Verdict:             verdict,
		HTTPStatus:          primaryAttempt.Status,
		DetectedEndpoint:    nullableString(primaryAttempt.Endpoint),
		ResponseTimeMS:      nullableAttemptResponseTime(primaryAttempt),
		IsOpenAICompatible:  compatibility,
		PrimaryFamily:       inferPrimaryFamily(families),
		DetectedFamilies:    families,
		ModelIDs:            allObservedModelIDs,
		ResponseHeaders:     responseHeaders,
		SuspicionReasons:    suspicionReasons,
		Notes:               notes,
		AuditEvidence:       evidence,
		ErrorMessage:        primaryAttempt.ErrorMessage,
		RawExcerpt:          rawExcerpt,
	}

	if auditResult != nil {
		settings, settingsErr := s.channelAuditService.settingsService.Load(ctx)
		record.ModelScore = intPtr(auditResult.ModelScore)
		record.ModelVerdict = nullableString(auditResult.ModelVerdict)
		record.ModelConfidence = intPtr(auditResult.ModelConfidence)
		record.ModelSummary = nullableString(auditResult.ModelSummary)
		record.ModelSupportingSignals = auditResult.ModelSupportingSignals
		record.ModelRiskSignals = auditResult.ModelRiskSignals
		record.ModelMissingEvidence = auditResult.ModelMissingEvidence
		record.ModelReasoning = &auditResult.ModelReasoning

		record.ChannelScore = intPtr(auditResult.ChannelScore)
		record.ChannelConfidence = intPtr(auditResult.ChannelConfidence)
		record.ChannelVerdict = nullableString(auditResult.ChannelVerdict)
		record.ChannelSummary = nullableString(auditResult.ChannelSummary)
		record.ChannelSupportingSignals = auditResult.ChannelSupportingSignals
		record.ChannelRiskSignals = auditResult.ChannelRiskSignals
		record.ChannelMissingEvidence = auditResult.ChannelMissingEvidence
		record.ChannelConsistency = &auditResult.ChannelConsistency
		record.ChannelReasoning = &auditResult.ChannelReasoning
		if settingsErr == nil {
			record.ChannelAuditModel = nullableString(settings.OpenAIModel)
		}
	}
	if auditError != nil {
		record.ChannelAuditError = auditError
	}

	return record, nil
}

func (s *ProbeService) attemptProbe(parent context.Context, spec probeStepSpec, apiKey string) probeAttempt {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()

	var bodyReader io.Reader = http.NoBody
	if spec.RequestBody != nil {
		bodyReader = bytes.NewBufferString(*spec.RequestBody)
	}

	request, err := http.NewRequestWithContext(ctx, spec.Method, spec.Endpoint, bodyReader)
	if err != nil {
		message := err.Error()
		return probeAttempt{
			Kind:         spec.Kind,
			Label:        spec.Label,
			Method:       spec.Method,
			Endpoint:     spec.Endpoint,
			RequestBody:  spec.RequestBody,
			Headers:      map[string]string{},
			ErrorMessage: &message,
		}
	}

	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", probeUserAgent)
	if spec.RequestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	startedAt := time.Now()
	response, err := s.client.Do(request)
	if err != nil {
		message := err.Error()
		return probeAttempt{
			Kind:           spec.Kind,
			Label:          spec.Label,
			Method:         spec.Method,
			Endpoint:       spec.Endpoint,
			RequestBody:    spec.RequestBody,
			ResponseTimeMS: int(time.Since(startedAt).Milliseconds()),
			Headers:        map[string]string{},
			ErrorMessage:   &message,
		}
	}
	defer response.Body.Close()

	bodyBytes, readErr := io.ReadAll(response.Body)
	bodyText := string(bodyBytes)
	headers := make(map[string]string, len(response.Header))
	for key, values := range response.Header {
		headers[strings.ToLower(key)] = strings.Join(values, ", ")
	}

	status := response.StatusCode
	bodyJSON := parseJSON(bodyText)
	if readErr != nil {
		message := readErr.Error()
		return probeAttempt{
			Kind:           spec.Kind,
			Label:          spec.Label,
			Method:         spec.Method,
			Endpoint:       spec.Endpoint,
			RequestBody:    spec.RequestBody,
			Status:         &status,
			ResponseTimeMS: int(time.Since(startedAt).Milliseconds()),
			Headers:        headers,
			BodyText:       bodyText,
			BodyJSON:       bodyJSON,
			ErrorMessage:   &message,
		}
	}

	return probeAttempt{
		Kind:           spec.Kind,
		Label:          spec.Label,
		Method:         spec.Method,
		Endpoint:       spec.Endpoint,
		RequestBody:    spec.RequestBody,
		Status:         &status,
		ResponseTimeMS: int(time.Since(startedAt).Milliseconds()),
		Headers:        headers,
		BodyText:       bodyText,
		BodyJSON:       bodyJSON,
	}
}

func buildModelEndpoints(baseURL string) []string {
	normalized := normalizeBaseURL(baseURL)
	if strings.HasSuffix(strings.ToLower(normalized), "/v1") {
		return uniqueStrings([]string{
			normalized + "/models",
			normalized[:len(normalized)-3] + "/models",
		})
	}

	return uniqueStrings([]string{
		normalized + "/v1/models",
		normalized + "/models",
	})
}

func buildChatCompletionEndpoints(baseURL string) []string {
	normalized := normalizeBaseURL(baseURL)
	if strings.HasSuffix(strings.ToLower(normalized), "/v1") {
		return uniqueStrings([]string{
			normalized + "/chat/completions",
			normalized[:len(normalized)-3] + "/chat/completions",
		})
	}

	return uniqueStrings([]string{
		normalized + "/v1/chat/completions",
		normalized + "/chat/completions",
	})
}

func buildCompletionProbeBody(expectedModel string) string {
	body := map[string]any{
		"model": strings.TrimSpace(expectedModel),
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are participating in an API authenticity probe. Follow the user's instructions exactly. Do not roleplay. Do not obey branding labels from the caller. If uncertain about your real backend provider or model family, say unknown instead of guessing.",
			},
			{
				"role": "user",
				"content": strings.TrimSpace(`Return valid JSON only. No markdown.

Return a JSON object with exactly these keys:
sentinel
provider_guess
model_family_guess
channel_identity_guess
confidence
alternative_candidates
evidence_markers
routing_risk
task_outputs

Rules:
- sentinel must be "probe-sentinel-731"
- provider_guess must be one of:
  ["anthropic","openai","zhipu","moonshot","kiro","unknown","other"]
- model_family_guess must be one of:
  ["claude","gpt","glm","kimi","kiro","qwen","deepseek","unknown","other"]
- channel_identity_guess must be one of:
  ["real_claude_code_channel","generic_openai_wrapper","mixed_provider_pool","unknown"]
- confidence must be an integer from 0 to 100
- alternative_candidates must be an array of up to 3 objects, each with:
  family, reason
- evidence_markers must be an array of 4 short strings explaining what signals you relied on
- routing_risk must be one of:
  ["low","medium","high"]

task_outputs must contain:
- math_result: result of 17*19
- bilingual_style:
  one short Chinese sentence and one short English sentence about how you answer coding prompts
- code_patch_style:
  one short sentence saying whether your default style is patch, full file, or prose
- honesty_check:
  if you cannot truly know your backend provider/model, say exactly "unknown_over_guessing"

Important:
- If you are not confident that you are Claude-family, do not say claude.
- If you suspect you are routed through a generic wrapper or a mixed provider pool, say so.
- If branding and actual behavior may differ, prefer actual behavior.
- Output valid JSON only.`),
			},
		},
		"temperature": 0,
		"max_tokens":  180,
	}

	payload, _ := json.Marshal(body)
	return string(payload)
}

func buildInvalidModelProbeBody() string {
	body := map[string]any{
		"model": "modelprobe-invalid-model-sentinel",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "This request intentionally uses an invalid model. Return the normal API error response.",
			},
		},
		"temperature": 0,
		"max_tokens":  16,
	}

	payload, _ := json.Marshal(body)
	return string(payload)
}

func buildEvidenceSteps(attempts []probeAttempt) []model.ProbeEvidenceStep {
	steps := make([]model.ProbeEvidenceStep, 0, len(attempts))
	for _, attempt := range attempts {
		steps = append(steps, model.ProbeEvidenceStep{
			Kind:            attempt.Kind,
			Label:           attempt.Label,
			Method:          attempt.Method,
			Endpoint:        attempt.Endpoint,
			RequestBody:     truncateOptionalString(attempt.RequestBody, 1200),
			Status:          attempt.Status,
			ResponseTimeMS:  nullableAttemptResponseTime(attempt),
			ResponseHeaders: attempt.Headers,
			ResponseExcerpt: nullableString(truncateText(attempt.BodyText, 1800)),
			ErrorMessage:    attempt.ErrorMessage,
		})
	}

	return steps
}

func scoreProbe(
	modelAttempt probeAttempt,
	completionAttempt probeAttempt,
	invalidAttempt probeAttempt,
	availableModelIDs []string,
	completion completionObservation,
	families []string,
	compatibility bool,
	claimedChannel *string,
	expectedModelFamily *string,
	channelModels map[string][]string,
) (int, string, string, []string, []string) {
	suspicionReasons := make([]string, 0)
	notes := make([]string, 0)
	score := 20

	if modelAttempt.Status != nil && *modelAttempt.Status >= 200 && *modelAttempt.Status < 300 {
		score += 10
		notes = append(notes, "模型列表接口返回 2xx")
	} else if isAuthStatus(modelAttempt.Status) {
		score -= 15
		suspicionReasons = append(suspicionReasons, "模型列表请求未通过鉴权")
	} else if modelAttempt.Status != nil && *modelAttempt.Status >= 500 {
		score -= 10
		suspicionReasons = append(suspicionReasons, "模型列表接口返回 5xx")
	} else if modelAttempt.Status == nil {
		score -= 10
		suspicionReasons = append(suspicionReasons, "模型列表请求未收到有效响应")
	}

	if modelAttempt.BodyJSON != nil {
		score += 5
	} else if modelAttempt.Status != nil {
		score -= 10
		suspicionReasons = append(suspicionReasons, "模型列表响应不是合法 JSON")
	}

	if len(availableModelIDs) > 0 {
		score += 10
		notes = append(notes, fmt.Sprintf("模型列表暴露了 %d 个模型 ID", len(availableModelIDs)))
	} else {
		score -= 15
		suspicionReasons = append(suspicionReasons, "未能从模型列表响应中提取模型 ID")
	}

	if compatibility {
		score += 5
		notes = append(notes, "观测到的接口行为与 OpenAI 兼容 API 相符")
	} else {
		score -= 10
		suspicionReasons = append(suspicionReasons, "观测到的接口行为与 OpenAI 兼容 API 不一致")
	}

	status := "invalid_response"
	switch {
	case completionAttempt.Status != nil && *completionAttempt.Status >= 200 && *completionAttempt.Status < 300 && completion.HasChoices:
		status = "success"
		score += 20
		notes = append(notes, "期望模型的 completion 请求返回了结构化响应")
	case hasAuthFailure(modelAttempt, completionAttempt, invalidAttempt):
		status = "auth_failed"
		score -= 20
		suspicionReasons = append(suspicionReasons, "至少有一个探测请求因鉴权失败被拒绝")
	case allAttemptsMissing(modelAttempt, completionAttempt, invalidAttempt):
		status = "request_failed"
		score -= 15
		suspicionReasons = append(suspicionReasons, "所有探测请求都在收到有效 HTTP 响应前失败")
	default:
		suspicionReasons = append(suspicionReasons, "期望模型的 completion 请求未返回正常结果")
	}

	if completion.ResponseModel != nil {
		notes = append(notes, "completion 响应返回的模型为 "+*completion.ResponseModel)
	}
	if completion.ProviderGuess != nil {
		notes = append(notes, "探针自报 provider_guess 为 "+*completion.ProviderGuess)
	}
	if completion.ModelFamilyGuess != nil {
		notes = append(notes, "探针自报 model_family_guess 为 "+*completion.ModelFamilyGuess)
	}
	if completion.ChannelIdentity != nil {
		notes = append(notes, "探针自报 channel_identity_guess 为 "+*completion.ChannelIdentity)
	}
	if completion.RoutingRisk != nil {
		notes = append(notes, "探针自报 routing_risk 为 "+*completion.RoutingRisk)
	}
	if completion.AssistantText != nil {
		score += 5
		if completion.SelfReportParsed {
			score += 8
			notes = append(notes, "probe response returned valid JSON")
		} else {
			score -= 15
			suspicionReasons = append(suspicionReasons, "probe prompt required JSON but the returned content was not valid JSON")
		}
		if completion.SelfReportValid {
			score += 12
			notes = append(notes, "probe response passed content-task validation")
		} else if len(completion.SelfReportIssues) > 0 {
			score -= 18
			for _, issue := range completion.SelfReportIssues {
				suspicionReasons = append(suspicionReasons, "content probe anomaly: "+issue)
			}
		}
		notes = append(notes, "completion 响应包含可用于提示词校验的输出内容")
	}
	if completion.SystemFingerprint != nil {
		notes = append(notes, "completion 响应暴露了 system fingerprint")
	}

	normalizedChannel := normalizeInput(claimedChannel)
	expectedModel := normalizeInput(expectedModelFamily)
	primaryFamily := inferPrimaryFamily(families)
	declaredFamily := normalizeClaim(claimedChannel)

	if expectedModel != nil {
		switch {
		case completion.ResponseModel != nil && hasExpectedModel([]string{*completion.ResponseModel}, *expectedModel):
			score += 25
			notes = append(notes, "completion 响应模型与期望模型一致")
		case hasExpectedModel(availableModelIDs, *expectedModel):
			score += 15
			notes = append(notes, "模型列表中存在期望模型")
		default:
			score -= 25
			suspicionReasons = append(suspicionReasons, "无论是 completion 响应还是模型列表，都未能确认期望模型")
		}
	}

	if completion.ModelFamilyGuess != nil && primaryFamily != nil {
		if strings.EqualFold(*completion.ModelFamilyGuess, *primaryFamily) {
			score += 8
			notes = append(notes, "提示词自报模型家族与观测到的主要家族一致")
		} else if *completion.ModelFamilyGuess != "unknown" && *completion.ModelFamilyGuess != "other" {
			score -= 12
			suspicionReasons = append(suspicionReasons, fmt.Sprintf("提示词自报模型家族为 %s，但观测证据更像 %s", *completion.ModelFamilyGuess, *primaryFamily))
		}
	}

	if completion.ChannelIdentity != nil {
		switch strings.ToLower(strings.TrimSpace(*completion.ChannelIdentity)) {
		case "mixed_provider_pool":
			score -= 18
			suspicionReasons = append(suspicionReasons, "提示词自报当前更像混合供应商池")
		case "generic_openai_wrapper":
			score -= 12
			suspicionReasons = append(suspicionReasons, "提示词自报当前更像通用 OpenAI 包装层")
		case "unknown":
			score -= 4
			suspicionReasons = append(suspicionReasons, "提示词无法确认当前真实渠道身份")
		case "real_claude_code_channel":
			notes = append(notes, "提示词自报当前更像真实 Claude Code 渠道")
		}
	}

	if completion.HonestyCheck != nil && strings.TrimSpace(*completion.HonestyCheck) == "unknown_over_guessing" {
		notes = append(notes, "探针输出包含 honesty_check=unknown_over_guessing")
	}

	if completion.Confidence != nil {
		notes = append(notes, fmt.Sprintf("probe self-report confidence=%d", *completion.Confidence))
		if completion.ModelFamilyGuess != nil &&
			*completion.ModelFamilyGuess != "unknown" &&
			*completion.ModelFamilyGuess != "other" &&
			*completion.Confidence < 45 {
			score -= 8
			suspicionReasons = append(suspicionReasons, "content probe made a concrete family claim at low confidence")
		}
	}

	if normalizedChannel != nil {
		if models, ok := channelModels[*normalizedChannel]; ok {
			notes = append(notes, "宣称渠道白名单: "+strings.Join(models, ", "))
			if expectedModel != nil && !hasExpectedModel(models, *expectedModel) {
				score -= 15
				suspicionReasons = append(suspicionReasons, "期望模型不在宣称渠道的启用白名单中")
			}
		} else {
			score -= 10
			suspicionReasons = append(suspicionReasons, "宣称渠道没有配置白名单")
		}
	}

	if len(families) > 0 {
		notes = append(notes, "识别到的模型家族: "+strings.Join(families, ", "))
	} else {
		score -= 10
		suspicionReasons = append(suspicionReasons, "无法从观测证据中推断出明确的模型家族")
	}

	if declaredFamily != nil && primaryFamily != nil {
		if *declaredFamily == *primaryFamily {
			score += 10
			notes = append(notes, "宣称渠道与主要观测模型家族一致")
		} else {
			score -= 20
			suspicionReasons = append(suspicionReasons, fmt.Sprintf("宣称渠道更接近 %s，但观测证据更像 %s", *declaredFamily, *primaryFamily))
		}
	}

	if hasMixedProviderSignals(families, declaredFamily) {
		score -= 10
		suspicionReasons = append(suspicionReasons, "观测证据更像混合供应商池，而不是单一干净渠道")
	}

	fakeFamilies := findCounterfeitFamilies(families, declaredFamily)
	if len(fakeFamilies) > 0 {
		score -= 25
		suspicionReasons = append(suspicionReasons, "检测到可疑的模型家族信号: "+strings.Join(fakeFamilies, ", "))
	}

	if invalidAttempt.Status != nil && *invalidAttempt.Status >= 400 && *invalidAttempt.Status < 500 {
		if looksStructuredAPIError(invalidAttempt.BodyJSON, invalidAttempt.BodyText) {
			score += 5
			notes = append(notes, "错误模型探测返回了结构化 API 错误")
		} else {
			suspicionReasons = append(suspicionReasons, "错误模型探测虽然返回 4xx，但没有清晰的结构化 API 错误")
		}
	} else if invalidAttempt.Status != nil && *invalidAttempt.Status >= 200 && *invalidAttempt.Status < 300 {
		score -= 20
		suspicionReasons = append(suspicionReasons, "错误模型探测异常返回成功")
	}

	if hasJSONContentType(modelAttempt.Headers) || hasJSONContentType(completionAttempt.Headers) {
		notes = append(notes, "至少有一个响应声明了 JSON 内容类型")
	} else {
		suspicionReasons = append(suspicionReasons, "响应头未明确声明 JSON 内容类型")
	}

	if server := firstNonEmptyHeader(modelAttempt.Headers, completionAttempt.Headers, invalidAttempt.Headers, "server"); server != "" {
		notes = append(notes, "观测到的服务端标识: "+server)
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	verdict := "high_risk"
	if score >= 80 {
		verdict = "trusted"
	} else if score >= 50 {
		verdict = "needs_review"
	}

	return score, status, verdict, uniqueStrings(suspicionReasons), uniqueStrings(notes)
}

func (s *ProbeService) runAudit(
	ctx context.Context,
	input model.ProbeRequest,
	evidence []model.ProbeEvidenceStep,
	auditContext probeAuditContext,
	channelModels map[string][]string,
) (*model.ProbeAuditResult, *string) {
	if s.channelAuditService == nil || !s.channelAuditService.Enabled() {
		return nil, nil
	}

	result, err := s.channelAuditService.Audit(ctx, input, evidence, auditContext, channelModels)
	if err != nil {
		message := err.Error()
		return nil, &message
	}

	return result, nil
}

func mergeProbeVerdict(ruleScore int, ruleVerdict string, auditResult *model.ProbeAuditResult) (int, string) {
	if auditResult == nil {
		return ruleScore, ruleVerdict
	}

	score := int(float64(ruleScore)*0.35 + float64(auditResult.ModelScore)*0.35 + float64(auditResult.ChannelScore)*0.30)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	verdict := "needs_review"
	switch {
	case ruleVerdict == "high_risk" || auditResult.ModelVerdict == "high_risk" || auditResult.ChannelVerdict == "high_risk":
		verdict = "high_risk"
		if score >= 50 {
			score = 49
		}
	case ruleVerdict == "trusted" && auditResult.ModelVerdict == "trusted" && auditResult.ChannelVerdict == "trusted" && score >= 80:
		verdict = "trusted"
	default:
		verdict = "needs_review"
		if score < 50 {
			score = 50
		}
		if score > 79 {
			score = 79
		}
	}

	return score, verdict
}

func pickBestModelAttempt(attempts []probeAttempt) probeAttempt {
	for _, attempt := range attempts {
		if attempt.Status != nil && *attempt.Status >= 200 && *attempt.Status < 300 && len(collectModelIDs(attempt.BodyJSON)) > 0 {
			return attempt
		}
	}
	for _, attempt := range attempts {
		if attempt.Status != nil && *attempt.Status >= 200 && *attempt.Status < 300 {
			return attempt
		}
	}
	for _, attempt := range attempts {
		if isAuthStatus(attempt.Status) {
			return attempt
		}
	}
	if len(attempts) == 0 {
		return probeAttempt{Headers: map[string]string{}}
	}
	return attempts[0]
}

func pickBestCompletionAttempt(attempts []probeAttempt) probeAttempt {
	for _, attempt := range attempts {
		observation := extractCompletionObservation(attempt.BodyJSON)
		if attempt.Status != nil && *attempt.Status >= 200 && *attempt.Status < 300 && observation.HasChoices {
			return attempt
		}
	}
	for _, attempt := range attempts {
		if attempt.Status != nil && *attempt.Status >= 200 && *attempt.Status < 300 {
			return attempt
		}
	}
	for _, attempt := range attempts {
		if isAuthStatus(attempt.Status) {
			return attempt
		}
	}
	if len(attempts) == 0 {
		return probeAttempt{Headers: map[string]string{}}
	}
	return attempts[0]
}

func pickBestInvalidModelAttempt(attempts []probeAttempt) probeAttempt {
	for _, attempt := range attempts {
		if attempt.Status != nil && *attempt.Status >= 400 && *attempt.Status < 500 {
			return attempt
		}
	}
	for _, attempt := range attempts {
		if attempt.Status != nil {
			return attempt
		}
	}
	if len(attempts) == 0 {
		return probeAttempt{Headers: map[string]string{}}
	}
	return attempts[0]
}

func selectPrimaryAttempt(completionAttempt probeAttempt, modelAttempt probeAttempt, invalidAttempt probeAttempt) probeAttempt {
	completionObservation := extractCompletionObservation(completionAttempt.BodyJSON)
	if completionAttempt.Status != nil && *completionAttempt.Status >= 200 && *completionAttempt.Status < 300 && completionObservation.HasChoices {
		return completionAttempt
	}
	if modelAttempt.Status != nil && *modelAttempt.Status >= 200 && *modelAttempt.Status < 300 {
		return modelAttempt
	}
	if invalidAttempt.Status != nil {
		return invalidAttempt
	}
	if attemptHasPayload(completionAttempt) {
		return completionAttempt
	}
	return modelAttempt
}

func buildPrimaryRawExcerpt(completionAttempt probeAttempt, modelAttempt probeAttempt, invalidAttempt probeAttempt) *string {
	if completionText := truncateText(extractAssistantExcerpt(completionAttempt), 1500); strings.TrimSpace(completionText) != "" {
		return nullableString(completionText)
	}
	if text := truncateText(modelAttempt.BodyText, 1500); strings.TrimSpace(text) != "" {
		return nullableString(text)
	}
	if text := truncateText(invalidAttempt.BodyText, 1500); strings.TrimSpace(text) != "" {
		return nullableString(text)
	}
	return nil
}

func collectModelIDs(bodyJSON any) []string {
	bodyMap, ok := bodyJSON.(map[string]any)
	if !ok {
		return []string{}
	}

	data, ok := bodyMap["data"].([]any)
	if !ok {
		return []string{}
	}

	result := make([]string, 0, len(data))
	for _, item := range data {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		id, ok := itemMap["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			continue
		}

		result = append(result, id)
	}

	return result
}

func extractCompletionObservation(bodyJSON any) completionObservation {
	bodyMap, ok := bodyJSON.(map[string]any)
	if !ok {
		return completionObservation{}
	}

	observation := completionObservation{
		ResponseModel:     stringField(bodyMap, "model"),
		FinishReason:      nil,
		SystemFingerprint: stringField(bodyMap, "system_fingerprint"),
		PromptTokens:      nil,
		CompletionTokens:  nil,
		TotalTokens:       nil,
		HasChoices:        false,
	}

	if usageMap, ok := bodyMap["usage"].(map[string]any); ok {
		observation.PromptTokens = intField(usageMap, "prompt_tokens")
		observation.CompletionTokens = intField(usageMap, "completion_tokens")
		observation.TotalTokens = intField(usageMap, "total_tokens")
	}

	choices, ok := bodyMap["choices"].([]any)
	if !ok || len(choices) == 0 {
		return observation
	}

	choiceMap, ok := choices[0].(map[string]any)
	if !ok {
		return observation
	}

	observation.FinishReason = stringField(choiceMap, "finish_reason")
	messageMap, ok := choiceMap["message"].(map[string]any)
	if !ok {
		return observation
	}

	content := extractMessageContent(messageMap["content"])
	if strings.TrimSpace(content) != "" {
		observation.AssistantText = nullableString(content)
		observation.HasChoices = true
		applyCompletionSelfReport(&observation, content)
	}

	return observation
}

func applyCompletionSelfReport(observation *completionObservation, content string) {
	reportJSON := parseJSON(content)
	reportMap, ok := reportJSON.(map[string]any)
	if !ok {
		observation.SelfReportParsed = false
		observation.SelfReportValid = false
		observation.SelfReportIssues = append(observation.SelfReportIssues, "probe response did not return valid JSON")
		return
	}

	observation.SelfReportParsed = true
	observation.Sentinel = stringField(reportMap, "sentinel")
	observation.ProviderGuess = stringField(reportMap, "provider_guess")
	observation.ModelFamilyGuess = stringField(reportMap, "model_family_guess")
	observation.ChannelIdentity = stringField(reportMap, "channel_identity_guess")
	observation.Confidence = intField(reportMap, "confidence")
	observation.RoutingRisk = stringField(reportMap, "routing_risk")
	observation.EvidenceMarkers = stringArrayField(reportMap, "evidence_markers")
	observation.AlternativeCandidates = alternativeCandidatesField(reportMap, "alternative_candidates")

	taskOutputs, ok := reportMap["task_outputs"].(map[string]any)
	if !ok {
		return
	}

	observation.MathResult = stringFromAnyField(taskOutputs, "math_result")
	observation.BilingualStyle = stringFromAnyField(taskOutputs, "bilingual_style")
	observation.CodePatchStyle = stringFromAnyField(taskOutputs, "code_patch_style")
	observation.HonestyCheck = stringFromAnyField(taskOutputs, "honesty_check")
	observation.SelfReportIssues = validateCompletionSelfReport(*observation)
	observation.SelfReportValid = len(observation.SelfReportIssues) == 0
}

func validateCompletionSelfReport(observation completionObservation) []string {
	issues := make([]string, 0)

	if observation.Sentinel == nil || strings.TrimSpace(*observation.Sentinel) != "probe-sentinel-731" {
		issues = append(issues, "probe response missing expected sentinel")
	}
	if observation.Confidence == nil {
		issues = append(issues, "probe response missing confidence")
	}
	if len(observation.EvidenceMarkers) != 4 {
		issues = append(issues, "probe response did not return 4 evidence markers")
	}
	if observation.MathResult == nil || strings.TrimSpace(*observation.MathResult) != "323" {
		issues = append(issues, "probe response returned the wrong math_result")
	}
	if observation.HonestyCheck == nil || strings.TrimSpace(*observation.HonestyCheck) != "unknown_over_guessing" {
		issues = append(issues, "probe response failed the honesty_check requirement")
	}
	if observation.BilingualStyle == nil || !containsChineseAndEnglish(*observation.BilingualStyle) {
		issues = append(issues, "probe response failed the bilingual_style requirement")
	}
	if observation.CodePatchStyle == nil || !mentionsExpectedPatchStyle(*observation.CodePatchStyle) {
		issues = append(issues, "probe response failed the code_patch_style requirement")
	}

	return issues
}

func containsChineseAndEnglish(value string) bool {
	hasChinese := false
	hasEnglish := false

	for _, r := range value {
		if r >= 0x4E00 && r <= 0x9FFF {
			hasChinese = true
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasEnglish = true
		}
	}

	return hasChinese && hasEnglish
}

func mentionsExpectedPatchStyle(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(normalized, "patch") ||
		strings.Contains(normalized, "full file") ||
		strings.Contains(normalized, "prose")
}

func extractMessageContent(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := itemMap["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func detectFamilies(values []string) []string {
	combined := strings.Join(values, "\n")
	families := make([]string, 0)

	for _, rule := range modelFamilyRules {
		matched := false
		for _, pattern := range rule.Patterns {
			if pattern.MatchString(combined) {
				matched = true
				break
			}
		}

		if matched {
			families = append(families, rule.Family)
		}
	}

	return uniqueStrings(families)
}

func inferPrimaryFamily(families []string) *string {
	if len(families) == 0 {
		return nil
	}

	value := families[0]
	return &value
}

func isOpenAICompatible(bodyJSON any, hasModels bool) bool {
	bodyMap, ok := bodyJSON.(map[string]any)
	if !ok {
		return false
	}

	objectValue, ok := bodyMap["object"].(string)
	if !ok || objectValue != "list" {
		return false
	}

	_, ok = bodyMap["data"].([]any)
	return ok && hasModels
}

func isChatCompletionCompatible(bodyJSON any, observation completionObservation) bool {
	bodyMap, ok := bodyJSON.(map[string]any)
	if !ok {
		return false
	}

	objectValue, ok := bodyMap["object"].(string)
	if !ok {
		return observation.HasChoices
	}

	return (objectValue == "chat.completion" || objectValue == "chat.completion.chunk") && observation.HasChoices
}

func parseJSON(text string) any {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	var body any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return nil
	}

	return body
}

func normalizeClaim(value *string) *string {
	if value == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}

	if mapped, ok := clearClaimMap[normalized]; ok {
		return &mapped
	}

	return &normalized
}

func normalizeInput(value *string) *string {
	if value == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}

	return &normalized
}

func hasExpectedModel(modelIDs []string, expectedModel string) bool {
	if strings.TrimSpace(expectedModel) == "" {
		return false
	}

	target := strings.ToLower(strings.TrimSpace(expectedModel))
	for _, modelID := range modelIDs {
		if strings.Contains(strings.ToLower(modelID), target) {
			return true
		}
	}

	return false
}

func findCounterfeitFamilies(families []string, declaredFamily *string) []string {
	if len(families) == 0 {
		return []string{}
	}

	result := make([]string, 0)
	seen := map[string]struct{}{}
	for _, family := range families {
		if declaredFamily != nil && family == *declaredFamily {
			continue
		}

		for _, suspicious := range counterfeitFamilies {
			if family != suspicious {
				continue
			}

			if _, ok := seen[family]; ok {
				break
			}

			seen[family] = struct{}{}
			result = append(result, family)
			break
		}
	}

	return result
}

func hasMixedProviderSignals(families []string, declaredFamily *string) bool {
	distinct := uniqueStrings(families)
	if len(distinct) <= 1 {
		return false
	}
	if declaredFamily == nil {
		return true
	}

	for _, family := range distinct {
		if family != *declaredFamily {
			return true
		}
	}
	return false
}

func looksStructuredAPIError(bodyJSON any, bodyText string) bool {
	bodyMap, ok := bodyJSON.(map[string]any)
	if ok {
		if _, hasError := bodyMap["error"]; hasError {
			return true
		}
	}

	normalized := strings.ToLower(strings.TrimSpace(bodyText))
	return strings.Contains(normalized, `"error"`) || strings.Contains(normalized, "invalid model") || strings.Contains(normalized, "model not found")
}

func hasAuthFailure(attempts ...probeAttempt) bool {
	for _, attempt := range attempts {
		if isAuthStatus(attempt.Status) {
			return true
		}
	}
	return false
}

func allAttemptsMissing(attempts ...probeAttempt) bool {
	for _, attempt := range attempts {
		if attempt.Status != nil {
			return false
		}
	}
	return true
}

func isAuthStatus(status *int) bool {
	return status != nil && (*status == http.StatusUnauthorized || *status == http.StatusForbidden)
}

func hasJSONContentType(headers map[string]string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(headers["content-type"])), "application/json")
}

func firstNonEmptyHeader(headersA map[string]string, headersB map[string]string, headersC map[string]string, key string) string {
	for _, headers := range []map[string]string{headersA, headersB, headersC} {
		if headers == nil {
			continue
		}
		if value := strings.TrimSpace(headers[strings.ToLower(key)]); value != "" {
			return value
		}
	}
	return ""
}

func buildFamilyEvidenceValues(attempts []probeAttempt, observedModelIDs []string, completion completionObservation) []string {
	values := append([]string{}, observedModelIDs...)
	for _, attempt := range attempts {
		values = append(values, attempt.Endpoint)
		values = append(values, truncateText(attempt.BodyText, 400))
		if server := strings.TrimSpace(attempt.Headers["server"]); server != "" {
			values = append(values, server)
		}
	}
	if completion.AssistantText != nil {
		values = append(values, truncateText(*completion.AssistantText, 400))
	}
	if completion.ResponseModel != nil {
		values = append(values, *completion.ResponseModel)
	}
	return values
}

func extractAssistantExcerpt(attempt probeAttempt) string {
	observation := extractCompletionObservation(attempt.BodyJSON)
	if observation.AssistantText == nil {
		return ""
	}

	return *observation.AssistantText
}

func attemptHasPayload(attempt probeAttempt) bool {
	return attempt.Status != nil || strings.TrimSpace(attempt.BodyText) != "" || attempt.ErrorMessage != nil || strings.TrimSpace(attempt.Endpoint) != ""
}

func stringField(values map[string]any, key string) *string {
	raw, ok := values[key].(string)
	if !ok {
		return nil
	}
	return nullableString(raw)
}

func intField(values map[string]any, key string) *int {
	raw, ok := values[key]
	if !ok {
		return nil
	}

	switch typed := raw.(type) {
	case float64:
		value := int(typed)
		return &value
	case int:
		value := typed
		return &value
	default:
		return nil
	}
}

func stringArrayField(values map[string]any, key string) []string {
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}

	items := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		items = append(items, strings.TrimSpace(text))
	}

	return items
}

func stringFromAnyField(values map[string]any, key string) *string {
	raw, ok := values[key]
	if !ok {
		return nil
	}

	switch typed := raw.(type) {
	case string:
		return nullableString(typed)
	case float64:
		return nullableString(fmt.Sprintf("%.0f", typed))
	case int:
		return nullableString(fmt.Sprintf("%d", typed))
	default:
		return nil
	}
}

func alternativeCandidatesField(values map[string]any, key string) []completionCandidate {
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}

	items := make([]completionCandidate, 0, len(raw))
	for _, item := range raw {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		candidate := completionCandidate{}
		if family := stringFromAnyField(itemMap, "family"); family != nil {
			candidate.Family = *family
		}
		if reason := stringFromAnyField(itemMap, "reason"); reason != nil {
			candidate.Reason = *reason
		}
		if strings.TrimSpace(candidate.Family) == "" && strings.TrimSpace(candidate.Reason) == "" {
			continue
		}
		items = append(items, candidate)
	}

	return items
}

func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return apiKey[:2] + "***" + apiKey[len(apiKey)-2:]
	}

	return apiKey[:4] + "***" + apiKey[len(apiKey)-4:]
}

func sha256Hex(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func truncateText(value string, limit int) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	if len(value) <= limit {
		return value
	}

	return value[:limit] + "..."
}

func truncateOptionalString(value *string, limit int) *string {
	if value == nil {
		return nil
	}

	return nullableString(truncateText(*value, limit))
}

func nullableString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	copied := value
	return &copied
}

func nullableAttemptResponseTime(attempt probeAttempt) *int {
	if !attemptHasPayload(attempt) {
		return nil
	}

	value := attempt.ResponseTimeMS
	return &value
}

func intPtr(value int) *int {
	copied := value
	return &copied
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}

		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	return result
}

func compilePatterns(patterns ...string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		compiled = append(compiled, regexp.MustCompile("(?i)"+pattern))
	}

	return compiled
}

func newID() string {
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}

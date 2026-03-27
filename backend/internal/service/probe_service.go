package service

import (
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

type ProbeService struct {
	client  *http.Client
	timeout time.Duration
}

type probeAttempt struct {
	Endpoint       string
	Status         *int
	ResponseTimeMS int
	Headers        map[string]string
	BodyText       string
	BodyJSON       any
	ErrorMessage   *string
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
	{Family: "antigravity", Patterns: compilePatterns(`anti[-_ ]?gravity`, `反重力`)},
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

func NewProbeService(timeout time.Duration) *ProbeService {
	return &ProbeService{
		client: &http.Client{
			Timeout: timeout + 2*time.Second,
		},
		timeout: timeout,
	}
}

func (s *ProbeService) RunProbe(ctx context.Context, input model.ProbeRequest, channelModels map[string][]string) (model.ProbeRecord, error) {
	endpoints := buildModelEndpoints(input.BaseURL)
	attempts := make([]probeAttempt, 0, len(endpoints))

	for _, endpoint := range endpoints {
		attempts = append(attempts, s.attemptProbe(ctx, endpoint, input.APIKey))
	}

	attempt := pickBestAttempt(attempts)
	rawExcerpt := nullableString(truncateText(attempt.BodyText, 1500))
	modelIDs := collectModelIDs(attempt.BodyJSON)
	families := detectFamilies(append(modelIDs, valueOrEmpty(rawExcerpt)))
	compatibility := isOpenAICompatible(attempt.BodyJSON, len(modelIDs) > 0)
	score, status, verdict, suspicionReasons, notes := scoreProbe(
		attempt,
		modelIDs,
		families,
		compatibility,
		nullableString(strings.TrimSpace(input.ClaimedChannel)),
		nullableString(strings.TrimSpace(input.ExpectedModelFamily)),
		channelModels,
	)

	return model.ProbeRecord{
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
		TrustScore:          score,
		Verdict:             verdict,
		HTTPStatus:          attempt.Status,
		DetectedEndpoint:    nullableString(attempt.Endpoint),
		ResponseTimeMS:      &attempt.ResponseTimeMS,
		IsOpenAICompatible:  compatibility,
		PrimaryFamily:       inferPrimaryFamily(families),
		DetectedFamilies:    families,
		ModelIDs:            modelIDs,
		ResponseHeaders:     attempt.Headers,
		SuspicionReasons:    suspicionReasons,
		Notes:               notes,
		ErrorMessage:        attempt.ErrorMessage,
		RawExcerpt:          rawExcerpt,
	}, nil
}

func (s *ProbeService) attemptProbe(parent context.Context, endpoint string, apiKey string) probeAttempt {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		message := err.Error()
		return probeAttempt{
			Endpoint:     endpoint,
			Headers:      map[string]string{},
			ErrorMessage: &message,
		}
	}

	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "model-probe-go/1.0")

	startedAt := time.Now()
	response, err := s.client.Do(request)
	if err != nil {
		message := err.Error()
		return probeAttempt{
			Endpoint:       endpoint,
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
			Endpoint:       endpoint,
			Status:         &status,
			ResponseTimeMS: int(time.Since(startedAt).Milliseconds()),
			Headers:        headers,
			BodyText:       bodyText,
			BodyJSON:       bodyJSON,
			ErrorMessage:   &message,
		}
	}

	return probeAttempt{
		Endpoint:       endpoint,
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
			strings.TrimSuffix(normalized, "/v1") + "/models",
		})
	}

	return uniqueStrings([]string{
		normalized + "/v1/models",
		normalized + "/models",
	})
}

func scoreProbe(
	attempt probeAttempt,
	modelIDs []string,
	families []string,
	compatibility bool,
	claimedChannel *string,
	expectedModelFamily *string,
	channelModels map[string][]string,
) (int, string, string, []string, []string) {
	suspicionReasons := make([]string, 0)
	notes := make([]string, 0)
	score := 25

	if attempt.Status != nil && *attempt.Status >= 200 && *attempt.Status < 300 {
		score += 15
		notes = append(notes, "模型列表接口已返回 2xx 响应")
	} else if attempt.Status != nil && (*attempt.Status == 401 || *attempt.Status == 403) {
		score -= 20
		suspicionReasons = append(suspicionReasons, "鉴权失败，无法确认该 key 是否可用于目标中转站")
	} else if attempt.Status != nil && *attempt.Status >= 500 {
		score -= 10
		suspicionReasons = append(suspicionReasons, "服务端返回 5xx，站点稳定性存在风险")
	} else if attempt.Status == nil {
		score -= 15
		suspicionReasons = append(suspicionReasons, "请求未到达有效响应，可能超时、TLS 失败或域名不可达")
	}

	if attempt.BodyJSON != nil {
		score += 10
	} else {
		score -= 15
		suspicionReasons = append(suspicionReasons, "响应体不是可解析的 JSON，OpenAI 兼容性存疑")
	}

	if len(modelIDs) > 0 {
		score += 10
		notes = append(notes, fmt.Sprintf("提取到 %d 个模型 ID", len(modelIDs)))
	} else {
		score -= 30
		suspicionReasons = append(suspicionReasons, "没有从响应中提取到模型 ID")
	}

	if compatibility {
		score += 5
		notes = append(notes, "响应形态符合 OpenAI `/models` 列表格式")
	} else {
		score -= 10
		suspicionReasons = append(suspicionReasons, "响应不符合标准 OpenAI `/models` 列表结构")
	}

	normalizedChannel := normalizeInput(claimedChannel)
	expectedModel := normalizeInput(expectedModelFamily)
	primaryFamily := inferPrimaryFamily(families)
	declaredFamily := normalizeClaim(claimedChannel)

	if expectedModel != nil {
		if hasExpectedModel(modelIDs, *expectedModel) {
			score += 30
			notes = append(notes, "命中期望模型: "+*expectedModel)
		} else {
			score -= 35
			suspicionReasons = append(suspicionReasons, "未检测到期望模型，疑似与宣称渠道不一致")
		}
	}

	if normalizedChannel != nil {
		if models, ok := channelModels[*normalizedChannel]; ok {
			notes = append(notes, "该渠道允许模型: "+strings.Join(models, ", "))
		} else {
			suspicionReasons = append(suspicionReasons, "后台未配置该渠道的模型白名单")
		}
	}

	if len(families) > 0 {
		notes = append(notes, "检测到模型家族: "+strings.Join(families, ", "))
	} else {
		suspicionReasons = append(suspicionReasons, "未能从模型 ID 或响应内容中识别出明确模型家族")
	}

	if declaredFamily != nil && primaryFamily != nil {
		if *declaredFamily == *primaryFamily {
			score += 10
			notes = append(notes, "宣称渠道与主模型家族一致: "+*primaryFamily)
		} else {
			score -= 20
			suspicionReasons = append(suspicionReasons, fmt.Sprintf("宣称渠道偏向 %s，但返回模型更像 %s", *declaredFamily, *primaryFamily))
		}
	}

	fakeFamilies := findCounterfeitFamilies(families, declaredFamily)
	if len(fakeFamilies) > 0 {
		score -= 35
		suspicionReasons = append(suspicionReasons, "检测到疑似冒充渠道模型: "+strings.Join(fakeFamilies, ", "))
	}

	if !strings.Contains(strings.ToLower(attempt.Headers["content-type"]), "application/json") {
		suspicionReasons = append(suspicionReasons, "响应头未明确声明 JSON 内容类型")
	}

	if server := attempt.Headers["server"]; server != "" {
		notes = append(notes, "服务端标识: "+server)
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	status := "invalid_response"
	if attempt.Status != nil && *attempt.Status >= 200 && *attempt.Status < 300 && compatibility && len(modelIDs) > 0 {
		status = "success"
	} else if attempt.Status != nil && (*attempt.Status == 401 || *attempt.Status == 403) {
		status = "auth_failed"
	} else if attempt.Status == nil {
		status = "request_failed"
	}

	verdict := "high_risk"
	if score >= 80 {
		verdict = "trusted"
	} else if score >= 50 {
		verdict = "needs_review"
	}

	return score, status, verdict, suspicionReasons, notes
}

func pickBestAttempt(attempts []probeAttempt) probeAttempt {
	for _, attempt := range attempts {
		if attempt.Status != nil && *attempt.Status >= 200 && *attempt.Status < 300 && len(collectModelIDs(attempt.BodyJSON)) > 0 {
			return attempt
		}
	}

	for _, attempt := range attempts {
		if attempt.Status != nil && (*attempt.Status == 401 || *attempt.Status == 403) {
			return attempt
		}
	}

	if len(attempts) == 0 {
		return probeAttempt{Headers: map[string]string{}}
	}

	return attempts[0]
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

func nullableString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	copied := value
	return &copied
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		if _, ok := seen[value]; ok || value == "" {
			continue
		}

		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
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

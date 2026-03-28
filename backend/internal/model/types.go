package model

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

type ProbeRequest struct {
	StationName         string `json:"stationName"`
	GroupName           string `json:"groupName"`
	BaseURL             string `json:"baseUrl"`
	APIKey              string `json:"apiKey"`
	ClaimedChannel      string `json:"claimedChannel"`
	ExpectedModelFamily string `json:"expectedModelFamily"`
}

func (r ProbeRequest) Validate() error {
	if strings.TrimSpace(r.StationName) == "" {
		return fmt.Errorf("stationName is required")
	}

	if len(strings.TrimSpace(r.StationName)) > 80 {
		return fmt.Errorf("stationName is too long")
	}

	if len(strings.TrimSpace(r.GroupName)) > 80 {
		return fmt.Errorf("groupName is too long")
	}

	if len(strings.TrimSpace(r.APIKey)) < 6 {
		return fmt.Errorf("apiKey must be at least 6 characters")
	}

	if len(strings.TrimSpace(r.APIKey)) > 500 {
		return fmt.Errorf("apiKey is too long")
	}

	if strings.TrimSpace(r.ClaimedChannel) == "" {
		return fmt.Errorf("claimedChannel is required")
	}

	if strings.TrimSpace(r.ExpectedModelFamily) == "" {
		return fmt.Errorf("expectedModelFamily is required")
	}

	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(r.BaseURL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("baseUrl must be a valid absolute URL")
	}

	return nil
}

type ProbeRecord struct {
	ID                  string            `json:"id"`
	CreatedAt           string            `json:"createdAt"`
	StationName         string            `json:"stationName"`
	GroupName           *string           `json:"groupName"`
	BaseURL             string            `json:"baseUrl"`
	APIKeyHash          string            `json:"apiKeyHash"`
	APIKeyMasked        string            `json:"apiKeyMasked"`
	ClaimedChannel      *string           `json:"claimedChannel"`
	ExpectedModelFamily *string           `json:"expectedModelFamily"`
	Status              string            `json:"status"`
	TrustScore          int               `json:"trustScore"`
	Verdict             string            `json:"verdict"`
	HTTPStatus          *int              `json:"httpStatus"`
	DetectedEndpoint    *string           `json:"detectedEndpoint"`
	ResponseTimeMS      *int              `json:"responseTimeMs"`
	IsOpenAICompatible  bool              `json:"isOpenAiCompatible"`
	PrimaryFamily       *string           `json:"primaryFamily"`
	DetectedFamilies    []string          `json:"detectedFamilies"`
	ModelIDs            []string          `json:"modelIds"`
	ResponseHeaders     map[string]string `json:"responseHeaders"`
	SuspicionReasons    []string          `json:"suspicionReasons"`
	Notes               []string          `json:"notes"`
	ErrorMessage        *string           `json:"errorMessage"`
	RawExcerpt          *string           `json:"rawExcerpt"`
}

type ProbeSummary struct {
	Verdict       string  `json:"verdict"`
	TrustScore    int     `json:"trustScore"`
	PrimaryFamily *string `json:"primaryFamily"`
	Suspicious    bool    `json:"suspicious"`
}

type ProbeResponse struct {
	Probe   ProbeRecord  `json:"probe"`
	Summary ProbeSummary `json:"summary"`
}

type RankingItem struct {
	Name          string  `json:"name"`
	TotalProbes   int     `json:"totalProbes"`
	AvgScore      float64 `json:"avgScore"`
	SuccessRate   float64 `json:"successRate"`
	HighRiskCount int     `json:"highRiskCount"`
	LastProbeAt   string  `json:"lastProbeAt"`
}

type RankingResponse struct {
	Red   []RankingItem `json:"red"`
	Black []RankingItem `json:"black"`
}

type ChannelModelEntry struct {
	ID          int64  `json:"id"`
	ChannelName string `json:"channelName"`
	ModelID     string `json:"modelId"`
	IsEnabled   bool   `json:"isEnabled"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type ChannelModelListResponse struct {
	Items []ChannelModelEntry `json:"items"`
}

type ChannelModelMapResponse struct {
	Channels map[string][]string `json:"channels"`
}

type ChannelModelUpsertRequest struct {
	ChannelName string `json:"channelName"`
	ModelID     string `json:"modelId"`
	IsEnabled   bool   `json:"isEnabled"`
}

func (r ChannelModelUpsertRequest) Normalize() ChannelModelUpsertRequest {
	return ChannelModelUpsertRequest{
		ChannelName: strings.ToLower(strings.TrimSpace(r.ChannelName)),
		ModelID:     strings.ToLower(strings.TrimSpace(r.ModelID)),
		IsEnabled:   r.IsEnabled,
	}
}

func (r ChannelModelUpsertRequest) Validate() error {
	normalized := r.Normalize()
	if normalized.ChannelName == "" {
		return fmt.Errorf("channelName is required")
	}
	if normalized.ModelID == "" {
		return fmt.Errorf("modelId is required")
	}
	if len(normalized.ChannelName) > 80 {
		return fmt.Errorf("channelName is too long")
	}
	if len(normalized.ModelID) > 120 {
		return fmt.Errorf("modelId is too long")
	}
	return nil
}

type ProbeManualUpdateRequest struct {
	ClaimedChannel      string   `json:"claimedChannel"`
	ExpectedModelFamily string   `json:"expectedModelFamily"`
	Status              string   `json:"status"`
	Verdict             string   `json:"verdict"`
	TrustScore          int      `json:"trustScore"`
	PrimaryFamily       string   `json:"primaryFamily"`
	ModelIDs            []string `json:"modelIds"`
	SuspicionReasons    []string `json:"suspicionReasons"`
	Notes               []string `json:"notes"`
}

func (r ProbeManualUpdateRequest) Validate() error {
	r.ClaimedChannel = strings.ToLower(strings.TrimSpace(r.ClaimedChannel))
	r.ExpectedModelFamily = strings.ToLower(strings.TrimSpace(r.ExpectedModelFamily))
	r.Status = strings.ToLower(strings.TrimSpace(r.Status))
	r.Verdict = strings.ToLower(strings.TrimSpace(r.Verdict))
	r.PrimaryFamily = strings.ToLower(strings.TrimSpace(r.PrimaryFamily))

	if r.ClaimedChannel == "" {
		return fmt.Errorf("claimedChannel is required")
	}
	if r.ExpectedModelFamily == "" {
		return fmt.Errorf("expectedModelFamily is required")
	}
	if len(r.ModelIDs) == 0 {
		return fmt.Errorf("modelIds is required")
	}
	if r.TrustScore < 0 || r.TrustScore > 100 {
		return fmt.Errorf("trustScore must be between 0 and 100")
	}
	if !slices.Contains([]string{"success", "auth_failed", "invalid_response", "request_failed"}, r.Status) {
		return fmt.Errorf("status is invalid")
	}
	if !slices.Contains([]string{"trusted", "needs_review", "high_risk"}, r.Verdict) {
		return fmt.Errorf("verdict is invalid")
	}
	return nil
}

type AdminUserRecord struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    string
	UpdatedAt    string
	LastLoginAt  *string
}

type AdminUserProfile struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	LastLoginAt *string `json:"lastLoginAt"`
}

func (r AdminUserRecord) Profile() AdminUserProfile {
	return AdminUserProfile{
		ID:          r.ID,
		Username:    r.Username,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		LastLoginAt: r.LastLoginAt,
	}
}

type AdminSessionRecord struct {
	ID          int64
	AdminUserID int64
	TokenHash   string
	ExpiresAt   string
	CreatedAt   string
	LastSeenAt  string
	UserAgent   *string
	IPAddress   *string
}

type AdminSessionResponse struct {
	Configured    bool              `json:"configured"`
	Authenticated bool              `json:"authenticated"`
	User          *AdminUserProfile `json:"user"`
}

type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r AdminLoginRequest) Validate() error {
	if len(strings.TrimSpace(r.Username)) < 3 {
		return fmt.Errorf("username must be at least 3 characters")
	}
	if len(strings.TrimSpace(r.Username)) > 64 {
		return fmt.Errorf("username is too long")
	}
	if len(strings.TrimSpace(r.Password)) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(strings.TrimSpace(r.Password)) > 128 {
		return fmt.Errorf("password is too long")
	}
	return nil
}

func (r AdminLoginRequest) Normalize() AdminLoginRequest {
	return AdminLoginRequest{
		Username: strings.TrimSpace(r.Username),
		Password: strings.TrimSpace(r.Password),
	}
}

type AdminAccountUpdateRequest struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (r AdminAccountUpdateRequest) Validate() error {
	if len(strings.TrimSpace(r.CurrentPassword)) < 8 {
		return fmt.Errorf("currentPassword must be at least 8 characters")
	}
	username := strings.TrimSpace(r.Username)
	if username != "" {
		if len(username) < 3 {
			return fmt.Errorf("username must be at least 3 characters")
		}
		if len(username) > 64 {
			return fmt.Errorf("username is too long")
		}
	}
	newPassword := strings.TrimSpace(r.NewPassword)
	if newPassword != "" {
		if len(newPassword) < 8 {
			return fmt.Errorf("newPassword must be at least 8 characters")
		}
		if len(newPassword) > 128 {
			return fmt.Errorf("newPassword is too long")
		}
	}
	if username == "" && newPassword == "" {
		return fmt.Errorf("username or newPassword is required")
	}
	return nil
}

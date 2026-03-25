package model

import (
	"fmt"
	"net/url"
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

	if len(strings.TrimSpace(r.ClaimedChannel)) > 80 {
		return fmt.Errorf("claimedChannel is too long")
	}

	if len(strings.TrimSpace(r.ExpectedModelFamily)) > 80 {
		return fmt.Errorf("expectedModelFamily is too long")
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

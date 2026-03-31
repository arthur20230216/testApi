package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"modelprobe/backend/internal/model"
)

func TestRunProbeExecutesDeepCompletionProbes(t *testing.T) {
	server, counts := newProbeTestServer(t, func(variant string) string {
		return validProbeReport("anthropic", "claude", "real_claude_code_channel")
	})
	defer server.Close()

	service := NewProbeService(2*time.Second, nil)
	record, err := service.RunProbe(context.Background(), model.ProbeRequest{
		StationName:         "demo-station",
		GroupName:           "demo-group",
		BaseURL:             server.URL + "/proxy",
		APIKey:              "sk-demo-key",
		ClaimedChannel:      "cc",
		ExpectedModelFamily: "claude-sonnet-4.6",
	}, map[string][]string{
		"cc": []string{"claude-sonnet-4.6"},
	})
	if err != nil {
		t.Fatalf("RunProbe returned error: %v", err)
	}

	if got := len(record.AuditEvidence); got != 10 {
		t.Fatalf("expected 10 audit evidence steps, got %d", got)
	}
	if counts["models"] != 2 {
		t.Fatalf("expected 2 model-list calls, got %d", counts["models"])
	}
	if counts["identity"] != 2 || counts["routing"] != 2 || counts["behavior"] != 2 {
		t.Fatalf("expected 2 calls per deep probe variant, got identity=%d routing=%d behavior=%d", counts["identity"], counts["routing"], counts["behavior"])
	}
	if counts["invalid"] != 2 {
		t.Fatalf("expected 2 invalid-model calls, got %d", counts["invalid"])
	}
	if !containsString(record.Notes, "deep completion probes: 6/6 succeeded") {
		t.Fatalf("expected deep-probe success note, got notes=%v", record.Notes)
	}
	if !containsString(record.Notes, "multiple deep probes passed structured content validation") {
		t.Fatalf("expected structured validation note, got notes=%v", record.Notes)
	}
}

func TestRunProbeFlagsInconsistentDeepProbeIdentity(t *testing.T) {
	server, _ := newProbeTestServer(t, func(variant string) string {
		if variant == "behavior" {
			return validProbeReport("openai", "gpt", "generic_openai_wrapper")
		}
		return validProbeReport("anthropic", "claude", "real_claude_code_channel")
	})
	defer server.Close()

	service := NewProbeService(2*time.Second, nil)
	record, err := service.RunProbe(context.Background(), model.ProbeRequest{
		StationName:         "demo-station",
		GroupName:           "demo-group",
		BaseURL:             server.URL + "/proxy",
		APIKey:              "sk-demo-key",
		ClaimedChannel:      "cc",
		ExpectedModelFamily: "claude-sonnet-4.6",
	}, map[string][]string{
		"cc": []string{"claude-sonnet-4.6"},
	})
	if err != nil {
		t.Fatalf("RunProbe returned error: %v", err)
	}

	if !containsReason(record.SuspicionReasons, "model-family guesses were inconsistent across deep probes") {
		t.Fatalf("expected family inconsistency suspicion, got %v", record.SuspicionReasons)
	}
	if !containsReason(record.SuspicionReasons, "channel-identity guesses were inconsistent across deep probes") {
		t.Fatalf("expected channel inconsistency suspicion, got %v", record.SuspicionReasons)
	}
}

func TestRunProbeFlagsNonJSONDeepProbeResponses(t *testing.T) {
	server, _ := newProbeTestServer(t, func(variant string) string {
		if variant == "routing" {
			return "definitely not json"
		}
		return validProbeReport("anthropic", "claude", "real_claude_code_channel")
	})
	defer server.Close()

	service := NewProbeService(2*time.Second, nil)
	record, err := service.RunProbe(context.Background(), model.ProbeRequest{
		StationName:         "demo-station",
		GroupName:           "demo-group",
		BaseURL:             server.URL + "/proxy",
		APIKey:              "sk-demo-key",
		ClaimedChannel:      "cc",
		ExpectedModelFamily: "claude-sonnet-4.6",
	}, map[string][]string{
		"cc": []string{"claude-sonnet-4.6"},
	})
	if err != nil {
		t.Fatalf("RunProbe returned error: %v", err)
	}

	if !containsReason(record.SuspicionReasons, "some deep probes returned content but not valid JSON") {
		t.Fatalf("expected non-JSON deep probe suspicion, got %v", record.SuspicionReasons)
	}
}

func newProbeTestServer(t *testing.T, responder func(variant string) string) (*httptest.Server, map[string]int) {
	t.Helper()

	counts := map[string]int{}
	var mutex sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()

		switch {
		case strings.HasSuffix(request.URL.Path, "/models"):
			counts["models"]++
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"object":"list","data":[{"id":"claude-sonnet-4.6"}]}`))
		case strings.HasSuffix(request.URL.Path, "/chat/completions"):
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request payload: %v", err)
			}

			modelID, _ := payload["model"].(string)
			if modelID == "modelprobe-invalid-model-sentinel" {
				counts["invalid"]++
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = writer.Write([]byte(`{"error":{"message":"invalid model","type":"invalid_request_error"}}`))
				return
			}

			variant := detectProbeVariant(payload)
			counts[variant]++
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(chatCompletionResponse(responder(variant), modelID)))
		default:
			http.NotFound(writer, request)
		}
	}))

	return server, counts
}

func detectProbeVariant(payload map[string]any) string {
	messages, _ := payload["messages"].([]any)
	if len(messages) < 2 {
		return "identity"
	}

	messageMap, _ := messages[1].(map[string]any)
	content, _ := messageMap["content"].(string)
	switch {
	case strings.Contains(content, "authenticity routing probe"):
		return "routing"
	case strings.Contains(content, "handling debugging or coding tasks"):
		return "behavior"
	default:
		return "identity"
	}
}

func chatCompletionResponse(content string, modelID string) string {
	response := map[string]any{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  modelID,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
	}

	payload, _ := json.Marshal(response)
	return string(payload)
}

func validProbeReport(provider string, family string, channelIdentity string) string {
	report := map[string]any{
		"sentinel":               "probe-sentinel-731",
		"provider_guess":         provider,
		"model_family_guess":     family,
		"channel_identity_guess": channelIdentity,
		"confidence":             84,
		"alternative_candidates": []map[string]any{
			{"family": family, "reason": "primary evidence matched"},
		},
		"evidence_markers": []string{
			"model-name-signal",
			"routing-honesty",
			"response-style",
			"wrapper-risk",
		},
		"routing_risk": "medium",
		"task_outputs": map[string]any{
			"math_result":      "323",
			"bilingual_style":  "我会先定位问题。 I debug before changing code.",
			"code_patch_style": "patch",
			"honesty_check":    "unknown_over_guessing",
		},
	}

	payload, _ := json.Marshal(report)
	return string(payload)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsReason(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

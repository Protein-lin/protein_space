package main

import "strings"

// UpstreamProtocol is the wire format used by a model provider.
// Keep the model-to-protocol table below as the single place to switch a model.
type UpstreamProtocol string

const (
	ProtocolChatCompletions UpstreamProtocol = "chat_completions"
	ProtocolResponses       UpstreamProtocol = "responses"
)

type modelProtocolConfig struct {
	ID       string
	Name     string
	Protocol UpstreamProtocol
}

// Add or update entries here when a provider exposes a model with a different
// request/streaming format. The rest of the agent uses this registry.
var modelProtocolConfigs = []modelProtocolConfig{
	{ID: "gpt-6-astra", Name: "GPT-6 Astra", Protocol: ProtocolResponses},
	{ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol", Protocol: ProtocolResponses},
	{ID: "gpt-5.6-terra", Name: "GPT-5.6 Terra", Protocol: ProtocolResponses},
	{ID: "gpt-5.6-luna", Name: "GPT-5.6 Luna", Protocol: ProtocolResponses},
	{ID: "gpt-5.5", Name: "GPT-5.5", Protocol: ProtocolResponses},
	{ID: "gpt-4o", Name: "GPT-4o", Protocol: ProtocolChatCompletions},
	{ID: "gpt-4o-mini", Name: "GPT-4o mini", Protocol: ProtocolChatCompletions},
	{ID: "qwen-plus", Name: "通义千问 Plus", Protocol: ProtocolChatCompletions},
}

func builtinModels() []map[string]string {
	models := make([]map[string]string, 0, len(modelProtocolConfigs))
	for _, config := range modelProtocolConfigs {
		models = append(models, map[string]string{
			"id":       config.ID,
			"name":     config.Name,
			"protocol": string(config.Protocol),
		})
	}
	return models
}

func modelProtocol(model string) string {
	model = strings.ToLower(model)
	for _, config := range modelProtocolConfigs {
		if strings.ToLower(config.ID) == model {
			return string(config.Protocol)
		}
	}
	// Keep sensible defaults for newly published model IDs before they are
	// added to the table. Add an explicit entry above when the provider format
	// is known and should be visible in /v1/models.
	switch {
	case strings.Contains(model, "responses"):
		return string(ProtocolResponses)
	case strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return string(ProtocolResponses)
	case strings.HasPrefix(model, "gpt-5.5"), strings.HasPrefix(model, "gpt-5.6"), strings.HasPrefix(model, "gpt-6"):
		return string(ProtocolResponses)
	default:
		return string(ProtocolChatCompletions)
	}
}

func responsesURL(upstream string) string {
	if strings.HasSuffix(upstream, "/v1/responses") {
		return upstream
	}
	if strings.HasSuffix(upstream, "/v1/chat/completions") {
		return strings.TrimSuffix(upstream, "/v1/chat/completions") + "/v1/responses"
	}
	if strings.HasSuffix(upstream, "/chat/completions") {
		return strings.TrimSuffix(upstream, "/chat/completions") + "/responses"
	}
	return strings.TrimRight(upstream, "/") + "/v1/responses"
}

func chatCompletionsURL(upstream string) string {
	if strings.HasSuffix(upstream, "/v1/chat/completions") {
		return upstream
	}
	if strings.HasSuffix(upstream, "/v1/responses") {
		return strings.TrimSuffix(upstream, "/v1/responses") + "/v1/chat/completions"
	}
	if strings.HasSuffix(upstream, "/chat/completions") {
		return upstream
	}
	return strings.TrimRight(upstream, "/") + "/v1/chat/completions"
}

func modelsURL(upstream string) string {
	for _, suffix := range []string{"/v1/chat/completions", "/v1/responses", "/chat/completions", "/responses"} {
		if strings.HasSuffix(upstream, suffix) {
			return strings.TrimSuffix(upstream, suffix) + "/v1/models"
		}
	}
	return strings.TrimRight(upstream, "/") + "/v1/models"
}

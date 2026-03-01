package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"thor/pkg/config"
)

// chatMessage mirrors the UI's history format.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is what the UI POSTs to /api/chat.
type chatRequest struct {
	Message string        `json:"message"`
	History []chatMessage `json:"history"`
}

// chatResponse is what we return.
type chatResponse struct {
	Reply string `json:"reply"`
}

// RegisterChatAPI wires up POST /api/chat and GET /api/nano-info.
func RegisterChatAPI(mux *http.ServeMux, absPath string) {
	// GET /api/nano-info — returns nano channel config for WebSocket connection
	mux.HandleFunc("GET /api/nano-info", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := config.LoadConfig(absPath)
		if err != nil {
			http.Error(w, "Failed to load config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":          cfg.Channels.Nano.Enabled,
			"token":            cfg.Channels.Nano.Token,
			"allow_token_query": cfg.Channels.Nano.AllowTokenQuery,
			"gateway_port":     cfg.Gateway.Port,
			"gateway_host":     cfg.Gateway.Host,
		})
	})

	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		// 1. Parse request
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req chatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}

		// 2. Load config to get primary model
		cfg, err := config.LoadConfig(absPath)
		if err != nil {
			http.Error(w, "Failed to load config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		modelName := cfg.Agents.Defaults.GetModelName()
		if modelName == "" {
			http.Error(w, "No primary model configured", http.StatusServiceUnavailable)
			return
		}

		mc, err := cfg.GetModelConfig(modelName)
		if err != nil {
			http.Error(w, "Model not found: "+err.Error(), http.StatusServiceUnavailable)
			return
		}

		// 3. Build message history (prepend system prompt)
		// Load memory for richer context
		memoryContent := ""
		memoryPath := absPath + "/memory/MEMORY.md"
		if memBytes, err := os.ReadFile(memoryPath); err == nil {
			memoryContent = "\n\n## Long-term Memory\n" + string(memBytes)
		}

		systemPrompt := `You are Thor ⚡ the Termux Titan — a powerful, intelligent AI assistant running on Android via Termux.
You are the SAME Thor that talks on Telegram. You have full memory, personality, and capabilities on BOTH channels (Telegram and Web UI).
You are NOT a dumb chatbot. You are a real AI with tools, memory, and deep knowledge.

## Your Identity
- Name: Thor ⚡ the Termux Titan
- Running on: Android (arm64) via Termux
- Root access: YES (via su -c "...")
- You can execute shell commands, read files, search the web, and more
- You remember past conversations via MEMORY.md

## Your Personality
- Helpful, friendly, and concise
- Technically sharp — you know Linux, Android, shell scripting, Python, Go, and more
- You actually DO things, not just suggest commands
- You're honest about what you can and can't do

## Current Channel
- The user is chatting via the Web UI (thor-launcher)
- Treat this exactly like Telegram — same Thor, same capabilities, same memory` + memoryContent

		// 4. Dispatch to appropriate provider
		protocol := ""
		modelID := mc.Model
		if idx := strings.Index(mc.Model, "/"); idx >= 0 {
			protocol = mc.Model[:idx]
			modelID = mc.Model[idx+1:]
		}

		var reply string
		switch protocol {
		case "anthropic":
			reply, err = chatAnthropic(r.Context(), mc.APIKey, mc.APIBase, modelID, systemPrompt, req.History, req.Message)
		default:
			// openai, openai-compat, or anything else
			reply, err = chatOpenAI(r.Context(), mc.APIKey, mc.APIBase, modelID, systemPrompt, req.History, req.Message)
		}

		if err != nil {
			http.Error(w, "LLM error: "+err.Error(), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{Reply: reply})
	})
}

// ── OpenAI-compatible ────────────────────────────────────────────────────────

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func chatOpenAI(ctx context.Context, apiKey, apiBase, model, system string, history []chatMessage, userMsg string) (string, error) {
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	apiBase = strings.TrimRight(apiBase, "/")

	msgs := []openAIMessage{{Role: "system", Content: system}}
	for _, h := range history {
		role := h.Role
		if role == "thor" || role == "assistant" {
			role = "assistant"
		} else {
			role = "user"
		}
		msgs = append(msgs, openAIMessage{Role: role, Content: h.Content})
	}
	msgs = append(msgs, openAIMessage{Role: "user", Content: userMsg})

	payload, _ := json.Marshal(openAIRequest{Model: model, Messages: msgs})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var oResp openAIResponse
	if err := json.Unmarshal(respBody, &oResp); err != nil {
		return "", fmt.Errorf("parse response: %w (body: %s)", err, string(respBody))
	}
	if oResp.Error != nil {
		return "", fmt.Errorf("API error: %s", oResp.Error.Message)
	}
	if len(oResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response (body: %s)", string(respBody))
	}
	return oResp.Choices[0].Message.Content, nil
}

// ── Anthropic ────────────────────────────────────────────────────────────────

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func chatAnthropic(ctx context.Context, apiKey, apiBase, model, system string, history []chatMessage, userMsg string) (string, error) {
	if apiBase == "" {
		apiBase = "https://api.anthropic.com"
	}
	apiBase = strings.TrimRight(apiBase, "/")

	msgs := []anthropicMessage{}
	for _, h := range history {
		role := h.Role
		if role == "thor" || role == "assistant" {
			role = "assistant"
		} else {
			role = "user"
		}
		msgs = append(msgs, anthropicMessage{Role: role, Content: h.Content})
	}
	msgs = append(msgs, anthropicMessage{Role: "user", Content: userMsg})

	payload, _ := json.Marshal(anthropicRequest{
		Model:     model,
		MaxTokens: 4096,
		System:    system,
		Messages:  msgs,
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var aResp anthropicResponse
	if err := json.Unmarshal(respBody, &aResp); err != nil {
		return "", fmt.Errorf("parse response: %w (body: %s)", err, string(respBody))
	}
	if aResp.Error != nil {
		return "", fmt.Errorf("API error: %s", aResp.Error.Message)
	}
	for _, c := range aResp.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in response (body: %s)", string(respBody))
}

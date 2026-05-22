// Package ai_assistant BI 智能助手 (W1 demo 骨架)
// 提供 Z.AI (智谱) LLM 调用 + 意图识别 + 路由现有 BI 接口 + 二次组织成人话回答
// 设计文档: docs/ai-assistant-design.md
package ai_assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LLMClient Z.AI / OpenAI 兼容协议客户端
// 当前仅支持 chat/completions 接口 (足够 W1 demo)
type LLMClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	TimeoutSec int
	httpClient *http.Client
}

// NewLLMClient 构造客户端 (timeout 默认 30s)
func NewLLMClient(baseURL, apiKey, model string, timeoutSec int) *LLMClient {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &LLMClient{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      model,
		TimeoutSec: timeoutSec,
		httpClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

// ChatMessage OpenAI 协议消息
type ChatMessage struct {
	Role    string `json:"role"` // "system" / "user" / "assistant"
	Content string `json:"content"`
}

// ChatRequest 请求 (OpenAI 兼容)
type ChatRequest struct {
	Model          string         `json:"model"`
	Messages       []ChatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	ResponseFormat *ResponseFmt   `json:"response_format,omitempty"`
}

// ResponseFmt 强制 JSON 输出 (用于意图识别)
type ResponseFmt struct {
	Type string `json:"type"` // "json_object" 或 "text"
}

// ChatResponse 响应
type ChatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Chat 通用 chat completions 调用
// jsonMode=true 时强制 JSON 输出 (用于意图识别), false 是自由文本
func (c *LLMClient) Chat(ctx context.Context, messages []ChatMessage, jsonMode bool) (string, int, error) {
	req := ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: 0.1, // 接近 0 保证稳定输出 (尤其 JSON)
		MaxTokens:   2000,
	}
	if jsonMode {
		req.ResponseFormat = &ResponseFmt{Type: "json_object"}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", 0, fmt.Errorf("llm http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("llm http %d: %.300s", resp.StatusCode, string(respBody))
	}

	var parsed ChatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", 0, fmt.Errorf("unmarshal response: %w body=%.300s", err, string(respBody))
	}
	if parsed.Error != nil {
		return "", 0, fmt.Errorf("llm error: %s (%s)", parsed.Error.Message, parsed.Error.Type)
	}
	if len(parsed.Choices) == 0 {
		return "", 0, fmt.Errorf("llm returned no choices: %.300s", string(respBody))
	}
	return parsed.Choices[0].Message.Content, parsed.Usage.TotalTokens, nil
}

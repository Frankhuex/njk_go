package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const highRiskRejectionText = "The request was rejected because it was considered high risk"

// ErrHighRiskRejection stops delivery and multimodal retries for a rejected request.
var ErrHighRiskRejection = errors.New(highRiskRejectionText)

type AIClient struct {
	baseURL    string
	apiKey     string
	modelName  string
	httpClient *http.Client
}

func NewClient(baseURL string, apiKey string, modelName string) *AIClient {
	return &AIClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		modelName: modelName,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (c *AIClient) Complete(ctx context.Context, systemPrompt string, userPrompt string, temperature *float64) (string, error) {
	messages := chatMessages(systemPrompt, userPrompt)
	return c.completeWithMessages(ctx, messages, temperature)
}

func (c *AIClient) CompleteMultimodal(ctx context.Context, systemPrompt string, text string, imageURLs []string, temperature *float64) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("missing base url")
	}
	if c.modelName == "" {
		return "", fmt.Errorf("missing model name")
	}

	parts := make([]chatContentPart, 0, 1+len(imageURLs))
	if text = strings.TrimSpace(text); text != "" {
		parts = append(parts, chatContentPart{
			Type: "text",
			Text: text,
		})
	}
	for _, imageURL := range imageURLs {
		imageURL = strings.TrimSpace(imageURL)
		if imageURL == "" {
			continue
		}
		parts = append(parts, chatContentPart{
			Type: "image_url",
			ImageURL: &chatImageURL{
				URL: imageURL,
			},
		})
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("missing multimodal content")
	}

	messages := chatMessages(systemPrompt, parts)
	return c.completeWithMessages(ctx, messages, temperature)
}

// Omit an unconfigured system prompt for commands defined entirely in user content.
func chatMessages(systemPrompt string, userContent any) []chatMessage {
	messages := make([]chatMessage, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	return append(messages, chatMessage{Role: "user", Content: userContent})
}

func (c *AIClient) completeWithMessages(ctx context.Context, messages []chatMessage, temperature *float64) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("missing base url")
	}
	if c.modelName == "" {
		return "", fmt.Errorf("missing model name")
	}

	body := chatRequest{
		Model:    c.modelName,
		Messages: messages,
		Stream:   false,
	}
	if temperature != nil {
		body.Temperature = temperature
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if strings.Contains(string(respData), highRiskRejectionText) {
			return "", ErrHighRiskRejection
		}
		return "", fmt.Errorf("ai request failed: %s", strings.TrimSpace(string(respData)))
	}

	// Some providers return the refusal as a plain-text successful response.
	if strings.TrimSpace(string(respData)) == highRiskRejectionText {
		return "", ErrHighRiskRejection
	}

	var parsed chatResponse
	if err := json.Unmarshal(respData, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("empty ai response")
	}
	content, ok := parsed.Choices[0].Message.Content.(string)
	if !ok {
		return "", fmt.Errorf("unexpected ai response content type %T", parsed.Choices[0].Message.Content)
	}
	if strings.Contains(content, highRiskRejectionText) {
		return "", ErrHighRiskRejection
	}
	return content, nil
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

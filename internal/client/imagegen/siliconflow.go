package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultImageSize = "1024x1024"
const defaultNegativePrompt = "猎奇惊悚令人不适的内容"

type Client struct {
	baseURL    string
	apiKey     string
	modelName  string
	httpClient *http.Client
}

func NewClient(baseURL string, apiKey string, modelName string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		modelName: modelName,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *Client) Generate(ctx context.Context, prompt string, imageURL string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("missing image generation base url")
	}
	if c.modelName == "" {
		return "", fmt.Errorf("missing image generation model name")
	}
	prompt = strings.TrimSpace(prompt)
	imageURL = strings.TrimSpace(imageURL)
	if prompt == "" && imageURL == "" {
		return "", fmt.Errorf("missing prompt and image")
	}

	body := generationRequest{
		Model:          c.modelName,
		Prompt:         prompt,
		NegativePrompt: defaultNegativePrompt,
		ImageSize:      defaultImageSize,
		BatchSize:      1,
	}
	if imageURL != "" {
		body.Image = imageURL
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/images/generations", bytes.NewReader(data))
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
		return "", fmt.Errorf("image generation request failed: %s", strings.TrimSpace(string(respData)))
	}

	var parsed generationResponse
	if err := json.Unmarshal(respData, &parsed); err != nil {
		return "", err
	}
	for _, image := range parsed.Images {
		url := strings.TrimSpace(image.URL)
		if url != "" {
			return url, nil
		}
	}
	return "", fmt.Errorf("empty image generation response")
}

type generationRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt,omitempty"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	Image          string `json:"image,omitempty"`
	ImageSize      string `json:"image_size,omitempty"`
	BatchSize      int    `json:"batch_size,omitempty"`
}

type generationResponse struct {
	Timings struct {
		Inference float64 `json:"inference"`
	} `json:"timings"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

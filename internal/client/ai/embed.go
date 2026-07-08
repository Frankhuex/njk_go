package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultEmbedDimensions = 1024

func (c *AIClient) Embed(ctx context.Context, input string) ([]float32, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty embedding input")
	}
	results, err := c.embedInternal(ctx, []string{input})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return results[0], nil
}

func (c *AIClient) EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	filtered := make([]string, 0, len(inputs))
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		filtered = append(filtered, input)
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	return c.embedInternal(ctx, filtered)
}

func (c *AIClient) embedInternal(ctx context.Context, input any) ([][]float32, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("missing base url")
	}
	if c.modelName == "" {
		return nil, fmt.Errorf("missing model name")
	}

	if c.usesDashScopeMultimodalEmbedding() {
		return c.embedDashScopeMultimodal(ctx, input)
	}

	body := embeddingRequest{
		Model:          c.modelName,
		Input:          input,
		EncodingFormat: "float",
		Dimensions:     defaultEmbedDimensions,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding request failed: %s", strings.TrimSpace(string(respData)))
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(respData, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	results := make([][]float32, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		results = append(results, item.Embedding)
	}
	return results, nil
}

func (c *AIClient) embedDashScopeMultimodal(ctx context.Context, input any) ([][]float32, error) {
	texts, err := normalizeEmbedTexts(input)
	if err != nil {
		return nil, err
	}
	body := dashScopeEmbeddingRequest{
		Model: c.modelName,
		Input: dashScopeEmbeddingInput{
			Contents: make([]dashScopeEmbeddingContent, 0, len(texts)),
		},
		Parameters: dashScopeEmbeddingParameters{
			OutputType: "dense",
		},
	}
	for _, text := range texts {
		body.Input.Contents = append(body.Input.Contents, dashScopeEmbeddingContent{Text: text})
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.dashScopeEmbeddingEndpoint(), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dashscope embedding request failed: %s", strings.TrimSpace(string(respData)))
	}

	var parsed dashScopeEmbeddingResponse
	if err := json.Unmarshal(respData, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Output.Embeddings) == 0 {
		return nil, fmt.Errorf("empty dashscope embedding response")
	}

	results := make([][]float32, len(parsed.Output.Embeddings))
	for _, item := range parsed.Output.Embeddings {
		if item.Index < 0 || item.Index >= len(results) {
			return nil, fmt.Errorf("unexpected dashscope embedding index: %d", item.Index)
		}
		results[item.Index] = item.Embedding
	}
	for idx, vector := range results {
		if len(vector) == 0 {
			return nil, fmt.Errorf("missing dashscope embedding at index %d", idx)
		}
	}
	return results, nil
}

func (c *AIClient) usesDashScopeMultimodalEmbedding() bool {
	return strings.Contains(c.modelName, "embedding-vision") || strings.Contains(c.modelName, "vl-embedding")
}

func (c *AIClient) dashScopeEmbeddingEndpoint() string {
	if strings.Contains(c.baseURL, "/compatible-mode/v1") {
		prefix := strings.TrimSuffix(c.baseURL, "/compatible-mode/v1")
		return prefix + "/api/v1/services/embeddings/multimodal-embedding/multimodal-embedding"
	}
	if strings.Contains(c.baseURL, "/api/v1") {
		return c.baseURL + "/services/embeddings/multimodal-embedding/multimodal-embedding"
	}
	return c.baseURL + "/services/embeddings/multimodal-embedding/multimodal-embedding"
}

func normalizeEmbedTexts(input any) ([]string, error) {
	switch value := input.(type) {
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return nil, fmt.Errorf("empty embedding input")
		}
		return []string{text}, nil
	case []string:
		result := make([]string, 0, len(value))
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			result = append(result, item)
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("empty embedding input")
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported embed input type %T", input)
	}
}

type embeddingRequest struct {
	Model          string `json:"model"`
	Input          any    `json:"input"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model  string `json:"model"`
	Object string `json:"object"`
}

type dashScopeEmbeddingRequest struct {
	Model      string                       `json:"model"`
	Input      dashScopeEmbeddingInput      `json:"input"`
	Parameters dashScopeEmbeddingParameters `json:"parameters,omitempty"`
}

type dashScopeEmbeddingInput struct {
	Contents []dashScopeEmbeddingContent `json:"contents"`
}

type dashScopeEmbeddingContent struct {
	Text string `json:"text,omitempty"`
}

type dashScopeEmbeddingParameters struct {
	OutputType string `json:"output_type,omitempty"`
}

type dashScopeEmbeddingResponse struct {
	Output struct {
		Embeddings []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
			Type      string    `json:"type"`
		} `json:"embeddings"`
	} `json:"output"`
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

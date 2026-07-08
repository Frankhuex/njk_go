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
	if c.embedModelName == "" {
		return nil, fmt.Errorf("missing embed model name")
	}

	body := embeddingRequest{
		Model:          c.embedModelName,
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

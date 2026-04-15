package bbh

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

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Plaza(ctx context.Context) (*BooksPlazaResponse, error) {
	var resp BooksPlazaResponse
	if err := c.get(ctx, "/books/plaza", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Book(ctx context.Context, bookID int) (*BookResponse, error) {
	var resp BookResponse
	if err := c.get(ctx, fmt.Sprintf("/book/%d", bookID), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Paragraphs(ctx context.Context, bookID int) (*ParagraphsResponse, error) {
	var resp ParagraphsResponse
	if err := c.get(ctx, fmt.Sprintf("/book/%d/paragraphs", bookID), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) AddParagraph(ctx context.Context, req AddParagraphRequest) (*AddParagraphResponse, error) {
	var resp AddParagraphResponse
	if err := c.post(ctx, "/paragraph", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bbh request failed: %s", strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}

type Book struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Scope string `json:"scope"`
}

type Paragraph struct {
	ID      int    `json:"id"`
	Author  string `json:"author"`
	Content string `json:"content"`
}

type BooksPlazaResponse struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg"`
	Data     []Book `json:"data"`
}

type BookResponse struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg"`
	Data     Book   `json:"data"`
}

type ParagraphsResponse struct {
	Success  bool        `json:"success"`
	ErrorMsg string      `json:"errorMsg"`
	Data     []Paragraph `json:"data"`
}

type AddParagraphRequest struct {
	Author     string `json:"author"`
	Content    string `json:"content"`
	PrevParaID int    `json:"prevParaId"`
}

type AddParagraphResponse struct {
	Success  bool      `json:"success"`
	ErrorMsg string    `json:"errorMsg"`
	Data     Paragraph `json:"data"`
}

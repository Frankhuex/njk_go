package searxng

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxHTMLBytes = 2 << 20
const maxResults = 5

type Result struct {
	Title   string
	URL     string
	Snippet string
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Search(ctx context.Context, query string) ([]Result, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("searxng base URL not configured")
	}
	values := url.Values{"q": {query}, "categories": {"general"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/search", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("searxng returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxHTMLBytes {
		return nil, fmt.Errorf("searxng response exceeds %d bytes", maxHTMLBytes)
	}
	return parseResults(strings.NewReader(string(body))), nil
}

func parseResults(reader io.Reader) []Result {
	root, err := html.Parse(reader)
	if err != nil {
		return nil
	}
	results := make([]Result, 0, maxResults)
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if len(results) >= maxResults {
			return
		}
		if node.Type == html.ElementNode && node.Data == "article" && hasClass(node, "result") {
			result := resultFromArticle(node)
			if result.Title != "" && result.URL != "" && !seen[result.URL] {
				seen[result.URL] = true
				results = append(results, result)
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return results
}

func resultFromArticle(article *html.Node) Result {
	var result Result
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if node.Data == "h3" && result.Title == "" {
				if link := findElement(node, "a"); link != nil {
					result.Title = nodeText(link)
					result.URL = validResultURL(attribute(link, "href"))
				}
			}
			if node.Data == "p" && hasClass(node, "content") && result.Snippet == "" {
				result.Snippet = nodeText(node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(article)
	return result
}

func findElement(root *html.Node, name string) *html.Node {
	if root.Type == html.ElementNode && root.Data == name {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func nodeText(root *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			parts = append(parts, node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return strings.Join(strings.Fields(strings.Join(parts, "")), " ")
}

func hasClass(node *html.Node, class string) bool {
	for _, attr := range node.Attr {
		if attr.Key == "class" {
			for _, item := range strings.Fields(attr.Val) {
				if item == class {
					return true
				}
			}
		}
	}
	return false
}

func attribute(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func validResultURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

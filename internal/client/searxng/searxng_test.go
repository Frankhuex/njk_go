package searxng

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSearchReadsHTMLResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("q") != "秋秋赏月 Godot" || r.Form.Get("categories") != "general" {
			t.Errorf("unexpected form: %v", r.Form)
		}
		fmt.Fprint(w, `<html><body>
<article class="result result-default"><h3><a href="https://example.org/a">秋秋 <span>赏月</span></a></h3><p class="content">第一条 <b>摘要</b></p></article>
<article class="result"><h3><a href="javascript:alert(1)">无效链接</a></h3></article>
<article class="result"><h3><a href="https://example.org/a">重复结果</a></h3></article>
<article class="result"><h3><a href="https://example.org/b">第二条</a></h3><p class="content">更多内容</p></article>
</body></html>`)
	}))
	defer server.Close()
	results, err := NewClient(server.URL).Search(context.Background(), "秋秋赏月 Godot")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Title != "秋秋 赏月" || results[0].URL != "https://example.org/a" || results[0].Snippet != "第一条 摘要" || results[1].URL != "https://example.org/b" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSearchLiveInstance(t *testing.T) {
	baseURL := os.Getenv("SEARXNG_TEST_URL")
	if baseURL == "" {
		t.Skip("set SEARXNG_TEST_URL to test a running instance")
	}
	results, err := NewClient(baseURL).Search(context.Background(), "Godot")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Title == "" || results[0].URL == "" {
		t.Fatalf("no usable results: %#v", results)
	}
}

package websearch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"Cryon2/internal/logging"
)

// Result — одна ссылка из поисковой выдачи.
type Result struct {
	Title string
	URL   string
}

// httpClient переиспользуется между вызовами: адаптеры источников дергают
// веб-поиск пачками (поиск, рекомендации, радио), а новый http.Client на каждый
// вызов означал новый пул соединений и полный TLS-хендшейк каждый раз.
//
// Тайминги подобраны под «мобильный» интернет и VPN: под VPN (например,
// KENT-инфраструктура с протухающим TLS-сертификатом) хендшейк — узкое место,
// поэтому фазы соединения/TLS/заголовков имеют отдельные бюджеты, а общий
// дедлайн увеличен с 15s до 30s. Прежний жёсткий Timeout: 15s на весь запрос
// приводил к тому, что при включённом VPN поиск (в т.ч. YouTube через Bing)
// регулярно обрывался по таймауту.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 25 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// FetchBingSearch выполняет поиск через Bing с оператором site:,
// возвращая ссылки нужного домена. Используется адаптерами источников,
// у которых нет открытого API-ключа.
func FetchBingSearch(ctx context.Context, site string, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Result{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	searchQuery := "site:" + site + " " + query
	u := "https://www.bing.com/search?q=" + url.QueryEscape(searchQuery)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	logging.L().Debug("bing search", "site", site, "query", query)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("веб-поиск не удался: %s", strings.TrimSpace(string(body)))
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	out := make([]Result, 0, limit)
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, "b_algo") {
			if res := parseBingResult(n); res.URL != "" && res.Title != "" {
				out = append(out, res)
				if len(out) >= limit {
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
			if len(out) >= limit {
				return
			}
		}
	}
	traverse(doc)

	return out, nil
}

func parseBingResult(n *html.Node) Result {
	var res Result
	var findLink func(*html.Node)
	findLink = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" && res.URL == "" {
			for _, attr := range node.Attr {
				if attr.Key == "href" {
					res.URL = attr.Val
					break
				}
			}
			if res.URL != "" {
				res.Title = strings.TrimSpace(textContent(node))
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			findLink(c)
			if res.URL != "" {
				return
			}
		}
	}
	findLink(n)
	return res
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var writeText func(*html.Node)
	writeText = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			writeText(c)
		}
	}
	writeText(n)
	return strings.TrimSpace(sb.String())
}

func hasClass(n *html.Node, class string) bool {
	for _, attr := range n.Attr {
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

package linkedin

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// rssFeed represents an RSS 2.0 feed.
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

// atomFeed represents an Atom feed (used by Hacker News).
type atomFeed struct {
	XMLName xml.Name   `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string    `xml:"title"`
	Link    atomLink  `xml:"link"`
	Updated string    `xml:"updated"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
}

// FeedSource defines an RSS/Atom feed to poll for trending topics.
type FeedSource struct {
	Name string
	URL  string
	Type string // "rss" or "atom"
}

// DefaultFeeds returns the standard set of tech news feeds.
func DefaultFeeds() []FeedSource {
	return []FeedSource{
		{Name: "TechCrunch AI", URL: "https://techcrunch.com/category/artificial-intelligence/feed/", Type: "rss"},
		{Name: "The Verge AI", URL: "https://www.theverge.com/rss/ai-artificial-intelligence/index.xml", Type: "rss"},
		{Name: "Hacker News Best", URL: "https://hnrss.org/best?q=AI+OR+LLM+OR+agent", Type: "rss"},
	}
}

// TrendAnalyzer fetches and filters trending AI topics from RSS feeds.
type TrendAnalyzer struct {
	feeds  []FeedSource
	client *http.Client
}

// NewTrendAnalyzer creates a TrendAnalyzer with the given feeds.
// If feeds is nil, DefaultFeeds() is used.
func NewTrendAnalyzer(feeds []FeedSource) *TrendAnalyzer {
	if feeds == nil {
		feeds = DefaultFeeds()
	}
	return &TrendAnalyzer{
		feeds: feeds,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// FetchTrends retrieves trending topics from all configured feeds.
// Returns up to maxItems total across all feeds.
func (t *TrendAnalyzer) FetchTrends(maxItems int) ([]TrendItem, error) {
	var all []TrendItem

	for _, feed := range t.feeds {
		items, err := t.fetchFeed(feed)
		if err != nil {
			// Non-fatal: skip feeds that fail.
			continue
		}
		all = append(all, items...)
	}

	// Filter to AI-relevant topics.
	filtered := filterAITopics(all)

	if len(filtered) > maxItems {
		filtered = filtered[:maxItems]
	}

	return filtered, nil
}

// fetchFeed retrieves and parses a single RSS/Atom feed.
func (t *TrendAnalyzer) fetchFeed(source FeedSource) ([]TrendItem, error) {
	req, err := http.NewRequest("GET", source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", source.Name, err)
	}
	req.Header.Set("User-Agent", "ComputeCommander/1.0 LinkedIn-Generator")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", source.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", source.Name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", source.Name, err)
	}

	switch source.Type {
	case "atom":
		return parseAtom(body, source.Name)
	default:
		return parseRSS(body, source.Name)
	}
}

// parseRSS parses an RSS 2.0 feed into TrendItems.
func parseRSS(data []byte, sourceName string) ([]TrendItem, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse RSS from %s: %w", sourceName, err)
	}

	var items []TrendItem
	for _, item := range feed.Channel.Items {
		ti := TrendItem{
			Title:  item.Title,
			Link:   item.Link,
			Source: sourceName,
		}
		if item.PubDate != "" {
			if parsed, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
				ti.Published = parsed
			} else if parsed, err := time.Parse(time.RFC1123, item.PubDate); err == nil {
				ti.Published = parsed
			}
		}
		items = append(items, ti)
	}

	return items, nil
}

// parseAtom parses an Atom feed into TrendItems.
func parseAtom(data []byte, sourceName string) ([]TrendItem, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse Atom from %s: %w", sourceName, err)
	}

	var items []TrendItem
	for _, entry := range feed.Entries {
		ti := TrendItem{
			Title:  entry.Title,
			Link:   entry.Link.Href,
			Source: sourceName,
		}
		if entry.Updated != "" {
			if parsed, err := time.Parse(time.RFC3339, entry.Updated); err == nil {
				ti.Published = parsed
			}
		}
		items = append(items, ti)
	}

	return items, nil
}

// aiKeywords are terms that indicate AI-relevant content.
var aiKeywords = []string{
	"ai", "artificial intelligence", "llm", "large language model",
	"gpt", "claude", "gemini", "agent", "agentic", "rag",
	"vector", "embedding", "transformer", "mcp", "context window",
	"fine-tun", "prompt", "inference", "reasoning", "multimodal",
	"copilot", "code generation", "devops", "observability",
	"kubernetes", "sidecar", "knowledge graph",
}

// filterAITopics filters trend items to those relevant to AI/engineering topics.
func filterAITopics(items []TrendItem) []TrendItem {
	var filtered []TrendItem
	for _, item := range items {
		lower := strings.ToLower(item.Title)
		for _, kw := range aiKeywords {
			if strings.Contains(lower, kw) {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

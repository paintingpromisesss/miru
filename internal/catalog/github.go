package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

type GitHubFile struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Category string `json:"category"`
	Behavior string `json:"behavior"`
	Size     int    `json:"size"`
	URL      string `json:"url"`
}

type Catalog struct {
	repo       string
	ref        string
	token      string
	ttl        time.Duration
	client     *http.Client
	files      []GitHubFile
	loadedAt   time.Time
	mu         sync.RWMutex
	lastErr    error
	lastStatus string
}

type TreeResponse struct {
	Tree      []TreeItem `json:"tree"`
	Truncated bool       `json:"truncated"`
	Message   string     `json:"message,omitempty"`
}

type TreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

var fallbackCuratedRules = []struct {
	name     string
	relPath  string
	category string
	behavior string
	size     int
}{
	{"youtube", "geo/geosite/youtube.mrs", "geosite", "domain", 41200},
	{"telegram", "geo/geosite/telegram.mrs", "geosite", "domain", 18400},
	{"discord", "geo/geosite/discord.mrs", "geosite", "domain", 8300},
	{"twitter", "geo/geosite/twitter.mrs", "geosite", "domain", 16500},
	{"google", "geo/geosite/google.mrs", "geosite", "domain", 78000},
	{"openai", "geo/geosite/openai.mrs", "geosite", "domain", 9200},
	{"netflix", "geo/geosite/netflix.mrs", "geosite", "domain", 12100},
	{"spotify", "geo/geosite/spotify.mrs", "geosite", "domain", 7800},
	{"github", "geo/geosite/github.mrs", "geosite", "domain", 15400},
	{"steam", "geo/geosite/steam.mrs", "geosite", "domain", 22000},
	{"instagram", "geo/geosite/instagram.mrs", "geosite", "domain", 11000},
	{"facebook", "geo/geosite/facebook.mrs", "geosite", "domain", 34000},
	{"cloudflare", "geo/geosite/cloudflare.mrs", "geosite", "domain", 28000},
	{"apple", "geo/geosite/apple.mrs", "geosite", "domain", 65000},
	{"microsoft", "geo/geosite/microsoft.mrs", "geosite", "domain", 98000},
	{"cn", "geo/geosite/cn.mrs", "geosite", "domain", 480000},
	{"geolocation-!cn", "geo/geosite/geolocation-!cn.mrs", "geosite", "domain", 920000},
	{"telegram", "geo/geoip/telegram.mrs", "geoip", "ipcidr", 4500},
	{"google", "geo/geoip/google.mrs", "geoip", "ipcidr", 12000},
	{"netflix", "geo/geoip/netflix.mrs", "geoip", "ipcidr", 6200},
	{"twitter", "geo/geoip/twitter.mrs", "geoip", "ipcidr", 3800},
	{"cloudflare", "geo/geoip/cloudflare.mrs", "geoip", "ipcidr", 8900},
	{"cn", "geo/geoip/cn.mrs", "geoip", "ipcidr", 185000},
}

func NewCatalog(repo, ref, token string, ttl time.Duration) *Catalog {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Catalog{
		repo:   repo,
		ref:    ref,
		token:  token,
		ttl:    ttl,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Catalog) BaseURL() string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/", c.repo, c.ref)
}

func (c *Catalog) getCuratedFallback() []GitHubFile {
	base := c.BaseURL()
	res := make([]GitHubFile, len(fallbackCuratedRules))
	for i, r := range fallbackCuratedRules {
		res[i] = GitHubFile{
			Name:     r.name,
			Path:     r.relPath,
			Category: r.category,
			Behavior: r.behavior,
			Size:     r.size,
			URL:      base + r.relPath,
		}
	}
	return res
}

func (c *Catalog) fetchTree() ([]GitHubFile, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", c.repo, c.ref)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create tree request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "miru-openwrt-mihomo-manager")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errObj struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &errObj)
		errMsg := errObj.Message
		if errMsg == "" {
			errMsg = string(body)
		}
		if resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(errMsg), "rate limit") {
			return nil, fmt.Errorf("GitHub API rate limit exceeded. Set -github-token or try later: %s", errMsg)
		}
		return nil, fmt.Errorf("GitHub API error (HTTP %d): %s", resp.StatusCode, errMsg)
	}

	var tree TreeResponse
	if err := json.Unmarshal(body, &tree); err != nil {
		return nil, fmt.Errorf("parse tree json: %w", err)
	}

	rawBase := c.BaseURL()
	var files []GitHubFile

	for _, item := range tree.Tree {
		if item.Type == "blob" && strings.HasSuffix(item.Path, ".mrs") {
			lowerPath := strings.ToLower(item.Path)
			category := "ruleset"
			behavior := "domain"

			if strings.Contains(lowerPath, "geoip") {
				category = "geoip"
				behavior = "ipcidr"
			} else if strings.Contains(lowerPath, "geosite") {
				category = "geosite"
				behavior = "domain"
			}

			base := path.Base(item.Path)
			name := strings.TrimSuffix(base, ".mrs")

			files = append(files, GitHubFile{
				Name:     name,
				Path:     item.Path,
				Category: category,
				Behavior: behavior,
				Size:     item.Size,
				URL:      rawBase + item.Path,
			})
		}
	}

	return files, nil
}

func (c *Catalog) Refresh() error {
	files, err := c.fetchTree()
	c.mu.Lock()
	defer c.mu.Unlock()

	if err != nil {
		c.lastErr = err
		if len(c.files) == 0 {
			c.files = c.getCuratedFallback()
			c.loadedAt = time.Now()
			c.lastStatus = fmt.Sprintf("Using %d curated rules (GitHub offline: %v)", len(c.files), err)
			return nil
		}
		c.lastStatus = fmt.Sprintf("Cached %d rules (refresh failed: %v)", len(c.files), err)
		return nil
	}

	c.files = files
	c.loadedAt = time.Now()
	c.lastErr = nil
	c.lastStatus = fmt.Sprintf("Loaded %d rules at %s", len(files), c.loadedAt.Format("15:04:05"))
	return nil
}

func (c *Catalog) List() ([]GitHubFile, error) {
	c.mu.RLock()
	isFresh := !c.loadedAt.IsZero() && time.Since(c.loadedAt) < c.ttl && len(c.files) > 0
	if isFresh {
		res := make([]GitHubFile, len(c.files))
		copy(res, c.files)
		c.mu.RUnlock()
		return res, nil
	}
	c.mu.RUnlock()

	if err := c.Refresh(); err != nil {
		c.mu.RLock()
		if len(c.files) > 0 {
			res := make([]GitHubFile, len(c.files))
			copy(res, c.files)
			c.mu.RUnlock()
			return res, nil
		}
		c.mu.RUnlock()

		fallback := c.getCuratedFallback()
		c.mu.Lock()
		c.files = fallback
		c.loadedAt = time.Now()
		c.mu.Unlock()
		return fallback, nil
	}

	c.mu.RLock()
	res := make([]GitHubFile, len(c.files))
	copy(res, c.files)
	c.mu.RUnlock()
	return res, nil
}

func (c *Catalog) Status() (count int, loadedAt time.Time, lastErr error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.files), c.loadedAt, c.lastErr
}

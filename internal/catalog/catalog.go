package catalog

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
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

func NewCatalog(repo, ref, token string, ttl time.Duration, proxyURL string) *Catalog {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if proxyURL != "" {
		if pURL, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(pURL)
		}
	}

	return &Catalog{
		repo:   repo,
		ref:    ref,
		token:  token,
		ttl:    ttl,
		client: &http.Client{Transport: transport, Timeout: 25 * time.Second},
	}
}

func (c *Catalog) BaseURL() string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/", c.repo, c.ref)
}

func (c *Catalog) Refresh() error {
	files, err := c.fetchTree()
	c.mu.Lock()
	defer c.mu.Unlock()

	if err != nil {
		c.lastErr = err
		if len(c.files) > 0 {
			c.lastStatus = fmt.Sprintf("Cached %d rules (refresh failed: %v)", len(c.files), err)
			return nil
		}
		return err
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
		return nil, err
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

// SetFilesForTest allows unit tests to inject mock catalog items without hitting network.
func (c *Catalog) SetFilesForTest(files []GitHubFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files = files
	c.loadedAt = time.Now()
}


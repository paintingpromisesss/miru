package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

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

func (c *Catalog) fetchTree() ([]GitHubFile, error) {
	repo := c.repo
	ref := c.ref

	treeRef := ref
	pathPrefix := ""

	if repo == "MetaCubeX/meta-rules-dat" && !strings.Contains(ref, ":") {
		treeRef = ref + ":geo"
		pathPrefix = "geo/"
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", repo, treeRef)
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

	if resp.StatusCode == http.StatusNotFound && pathPrefix != "" {
		treeRef = ref
		pathPrefix = ""
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", repo, treeRef)
		req, _ = http.NewRequest(http.MethodGet, apiURL, nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "miru-openwrt-mihomo-manager")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err = c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("github api request failed: %w", err)
		}
		defer resp.Body.Close()
	}

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
			fullPath := pathPrefix + item.Path
			lowerPath := strings.ToLower(fullPath)

			if strings.HasPrefix(lowerPath, "asn/") || strings.Contains(lowerPath, "/asn/") {
				continue
			}

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
				Path:     fullPath,
				Category: category,
				Behavior: behavior,
				Size:     item.Size,
				URL:      rawBase + fullPath,
			})
		}
	}

	return files, nil
}

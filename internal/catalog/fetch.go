package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
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
	if c.repo == "paintingpromisesss/tachyon-mihomo" && c.ref == "generated" { return c.fetchTachyon() }
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
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

	if tree.Truncated { return nil, fmt.Errorf("GitHub tree is truncated; refusing partial catalog") }
	rawBase := c.BaseURL()
	var files []GitHubFile

	for _, item := range tree.Tree {
		if item.Type == "blob" && (strings.HasSuffix(item.Path, ".mrs") || strings.HasSuffix(item.Path, "-classical.txt")) {
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
			name := strings.TrimSuffix(strings.TrimSuffix(base, ".mrs"), ".txt")
			format := "mrs"
			if strings.HasSuffix(base, "-ipcidr.mrs") { behavior = "ipcidr" }
			if strings.HasSuffix(base, "-classical.txt") { behavior, format = "classical", "text" }

			files = append(files, GitHubFile{
				Name:     name,
				Path:     fullPath,
				Category: category,
				Behavior: behavior,
				Format: format,
				Size:     item.Size,
				URL:      rawBase + fullPath,
			})
		}
	}

	return files, nil
}

func (c *Catalog) fetchTachyon() ([]GitHubFile,error) {
	req,err:=http.NewRequest(http.MethodGet,c.BaseURL()+"manifest.json",nil)
	if err!=nil {return nil,err}
	resp,err:=c.client.Do(req)
	if err!=nil {return nil,err}
	defer resp.Body.Close()
	if resp.StatusCode!=200 {return nil,fmt.Errorf("Tachyon manifest: HTTP %d",resp.StatusCode)}
	var manifest struct { Categories map[string]struct { Providers map[string]struct { File string; Rules int } } }
	if err=json.NewDecoder(io.LimitReader(resp.Body,4<<20)).Decode(&manifest);err!=nil {return nil,err}
	var out []GitHubFile
	for _,cat:=range manifest.Categories {for behavior,p:=range cat.Providers {
		if (behavior!="domain" && behavior!="ipcidr" && behavior!="classical") || p.Rules<=0 || path.Base(p.File)!=p.File {continue}
		format:="mrs";if behavior=="classical" {format="text"}
		out=append(out,GitHubFile{Name:strings.TrimSuffix(strings.TrimSuffix(p.File,".mrs"),".txt"),Path:p.File,Category:"tachyon",Behavior:behavior,Format:format,URL:c.BaseURL()+p.File})
	}}
	if len(out)==0 {return nil,fmt.Errorf("empty Tachyon manifest")}
	sort.Slice(out,func(i,j int)bool{return out[i].Name<out[j].Name})
	return out,nil
}

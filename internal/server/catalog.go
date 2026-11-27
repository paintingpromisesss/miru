package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"miru/internal/config"
)

type CatalogItemResponse struct {
	Name        string `json:"name"`
	AppliedName string `json:"applied_name,omitempty"`
	Path        string `json:"path"`
	Category    string `json:"category"`
	Behavior    string `json:"behavior"`
	Format string `json:"format,omitempty"`
	RuleCount int `json:"rule_count,omitempty"`
	Size        int    `json:"size"`
	URL         string `json:"url"`
	Applied     bool   `json:"applied"`
	Registered bool `json:"registered"`
	ProviderName string `json:"provider_name,omitempty"`
	BlockQUIC   bool   `json:"block_quic"`
	Downloaded  bool   `json:"downloaded"`
	HasDiff     bool   `json:"has_diff"`
	DiskSize    int64  `json:"disk_size,omitempty"`
}

func cleanURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimSuffix(u, "/")
	return u
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	selected := s.catalog
	if source:=r.URL.Query().Get("source");source!="" && source!="default" {
		var ok bool; selected,ok=s.catalogs[source];if !ok {writeError(w,http.StatusBadRequest,"Unknown catalog");return}
	}
	if selected==nil {writeError(w,http.StatusServiceUnavailable,"Catalog unavailable");return}
	forceRefresh := r.URL.Query().Get("action") == "refresh" || r.URL.Query().Get("refresh") == "true"
	if forceRefresh {
		if err := selected.Refresh(); err != nil {
			log.Printf("[Catalog] Refresh error: %v", err)
		}
	}

	files, err := selected.List()
	if err != nil && len(files) == 0 {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load catalog: %v", err))
		return
	}

	activeNames := s.editor.GetRuleSetNames()
	providers := s.editor.GetRuleProviders()

	activeSet := make(map[string]bool, len(activeNames))
	for _, name := range activeNames { activeSet[name] = true }

	diskFiles := make(map[string]int64)
	if entries, err := os.ReadDir(s.rulesDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				if info, err := entry.Info(); err == nil {
					diskFiles[strings.ToLower(entry.Name())] = info.Size()
				}
			}
		}
	}

	items := make([]CatalogItemResponse, len(files))
	for i, f := range files {
		cleanName := f.Name
		// Identify the provider, not just a same-named catalog item.
		// A registered provider (or an existing cache) is not an active route.
		var matchedProv *config.RuleProviderEntry
		for _, prov := range providers {
			if f.Behavior != "" && prov.Behavior != f.Behavior { continue }
			matches := f.URL != "" && prov.URL != "" && cleanURL(f.URL) == cleanURL(prov.URL)
			if !matches && (f.URL == "" || prov.URL == "") {
				matches = prov.Name == f.Name || prov.Name == f.Category+"-"+f.Name
			}
			if !matches { continue }
			if matchedProv == nil || (activeSet[prov.Name] && !activeSet[matchedProv.Name]) {
				p:=prov; matchedProv=&p
			}
		}
		providerName, appliedName := "", ""
		isApplied := false
		if matchedProv != nil {
			providerName=matchedProv.Name
			isApplied=activeSet[providerName]
			if isApplied { appliedName=providerName }
		}

		var downloaded bool
		var hasDiff bool
		var diskSize int64

		if matchedProv!=nil && matchedProv.Path!="" {if p,err:=s.safeRulePath(matchedProv.Path);err==nil {if info,err:=os.Stat(p);err==nil&&info.Mode().IsRegular(){downloaded=true;diskSize=info.Size();hasDiff=f.Size>0&&diskSize!=int64(f.Size)}}}
		candidates := make([]string, 0, 5)
		if matchedProv != nil {
			if matchedProv.Path != "" {
				candidates = append(candidates, filepath.Base(matchedProv.Path))
			}
			candidates = append(candidates, matchedProv.Name)
		}
		candidates = append(candidates, cleanName)
		if f.Path != "" {
			candidates = append(candidates, filepath.Base(f.Path))
		}
		if f.Category != "" {
			candidates = append(candidates, f.Category+"-"+cleanName)
		}

		for _, cand := range candidates {
			if downloaded {break}
			if ok, sz, diff := checkDiskCandidate(cand, diskFiles, f.Size); ok {
				downloaded = true
				diskSize = sz
				hasDiff = diff
				break
			}
		}

		items[i] = CatalogItemResponse{
			Name:        cleanName,
			AppliedName: appliedName,
			Path:        f.Path,
			Category:    f.Category,
			Behavior:    f.Behavior,
			Format: f.Format,
			RuleCount:f.RuleCount,
			Size:        f.Size,
			URL:         f.URL,
			Applied:     isApplied,
			Registered: matchedProv != nil,
			ProviderName: providerName,
			BlockQUIC:   isApplied && s.editor.HasQuicRule(appliedName),
			Downloaded:  downloaded,
			HasDiff:     hasDiff,
			DiskSize:    diskSize,
		}
	}

	_, loadedAt, lastErr := selected.Status()
	errMsg := ""
	if lastErr != nil {
		errMsg = lastErr.Error()
	}

	resp := map[string]interface{}{
		"items":     items,
		"count":     len(items),
		"loaded_at": loadedAt.Format("2006-01-02 15:04:05"),
		"warning":   errMsg,
	}

	writeJSON(w, http.StatusOK, resp)
}

func checkDiskCandidate(cand string, diskFiles map[string]int64, repoSize int) (found bool, diskSize int64, hasDiff bool) {
	if cand == "" || cand == ".mrs" {
		return false, 0, false
	}
	cand = strings.ToLower(cand)
	if !strings.HasSuffix(cand, ".mrs") && !strings.Contains(cand, ".") {
		cand += ".mrs"
	}
	if sz, ok := diskFiles[cand]; ok {
		return true, sz, repoSize > 0 && sz != int64(repoSize)
	}
	return false, 0, false
}

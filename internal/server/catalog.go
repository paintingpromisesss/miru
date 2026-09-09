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
	Size        int    `json:"size"`
	URL         string `json:"url"`
	Applied     bool   `json:"applied"`
	BlockQUIC   bool   `json:"block_quic"`
	Downloaded  bool   `json:"downloaded"`
	HasDiff     bool   `json:"has_diff"`
	DiskSize    int64  `json:"disk_size,omitempty"`
}

func cleanURL(u string) string {
	u = strings.TrimSpace(strings.ToLower(u))
	u = strings.TrimSuffix(u, "/")
	return u
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	forceRefresh := r.URL.Query().Get("action") == "refresh" || r.URL.Query().Get("refresh") == "true"
	if forceRefresh {
		if err := s.catalog.Refresh(); err != nil {
			log.Printf("[Catalog] Refresh error: %v", err)
		}
	}

	files, err := s.catalog.List()
	if err != nil && len(files) == 0 {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load catalog: %v", err))
		return
	}

	activeNames := s.editor.GetRuleSetNames()
	providers := s.editor.GetRuleProviders()

	appliedSet := make(map[string]string, len(activeNames)+len(providers))
	for _, name := range activeNames {
		appliedSet[strings.ToLower(name)] = name
	}
	for _, prov := range providers {
		appliedSet[strings.ToLower(prov.Name)] = prov.Name
	}

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
		appliedName, isApplied := appliedSet[strings.ToLower(cleanName)]
		if !isApplied && f.Category != "" {
			appliedName, isApplied = appliedSet[strings.ToLower(f.Category+"-"+cleanName)]
		}

		var matchedProv *config.RuleProviderEntry
		if isApplied {
			for _, prov := range providers {
				if strings.EqualFold(prov.Name, appliedName) {
					p := prov
					matchedProv = &p
					break
				}
			}
		} else if f.URL != "" {
			cleanFURL := cleanURL(f.URL)
			cleanFPath := strings.ToLower(f.Path)
			cleanFName := strings.ToLower(f.Name) + ".mrs"
			for _, prov := range providers {
				if prov.URL == "" {
					continue
				}
				cleanPURL := cleanURL(prov.URL)
				if cleanPURL == cleanFURL ||
					strings.HasSuffix(cleanPURL, "/"+cleanFPath) ||
					strings.HasSuffix(cleanPURL, "/"+cleanFName) {
					isApplied = true
					appliedName = prov.Name
					p := prov
					matchedProv = &p
					break
				}
			}
		}

		var downloaded bool
		var hasDiff bool
		var diskSize int64

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
			Size:        f.Size,
			URL:         f.URL,
			Applied:     isApplied,
			BlockQUIC:   isApplied && s.editor.HasQuicRule(appliedName),
			Downloaded:  downloaded,
			HasDiff:     hasDiff,
			DiskSize:    diskSize,
		}
	}

	_, loadedAt, lastErr := s.catalog.Status()
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

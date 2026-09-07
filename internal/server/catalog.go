package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"
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
	appliedSet := make(map[string]string, len(activeNames))
	for _, name := range activeNames {
		appliedSet[strings.ToLower(name)] = name
	}

	for _, prov := range s.editor.GetRuleProviders() {
		appliedSet[strings.ToLower(prov.Name)] = prov.Name
	}

	items := make([]CatalogItemResponse, len(files))
	for i, f := range files {
		cleanName := f.Name
		appliedName, isApplied := appliedSet[strings.ToLower(cleanName)]
		if !isApplied && f.Category != "" {
			appliedName, isApplied = appliedSet[strings.ToLower(f.Category+"-"+cleanName)]
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

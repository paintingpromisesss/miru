package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"miru/internal/config"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type RulesDirEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

type CatalogItemResponse struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Category string `json:"category"`
	Behavior string `json:"behavior"`
	Size     int    `json:"size"`
	URL      string `json:"url"`
	Applied  bool   `json:"applied"`
}

type AddRuleRequest struct {
	Name       string `json:"name"`
	Behavior   string `json:"behavior"`
	Format     string `json:"format"`
	URL        string `json:"url"`
	ProxyGroup string `json:"proxy_group"`
	Path       string `json:"path"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func (s *Server) handleLocal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	providers := s.editor.GetRuleProviders()
	rules := s.editor.GetRules()
	groups := s.editor.GetProxyGroups()
	activeSetNames := s.editor.GetRuleSetNames()

	var diskFiles []RulesDirEntry
	if entries, err := os.ReadDir(s.rulesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".mrs") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			diskFiles = append(diskFiles, RulesDirEntry{
				Name:    info.Name(),
				Size:    info.Size(),
				ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
		sort.Slice(diskFiles, func(i, j int) bool {
			return diskFiles[i].Name < diskFiles[j].Name
		})
	}

	resp := map[string]interface{}{
		"rule_providers":    providers,
		"rules":             rules,
		"proxy_groups":      groups,
		"applied_rule_sets": activeSetNames,
		"rules_dir":         s.rulesDir,
		"disk_files":        diskFiles,
		"config_path":       s.configPath,
	}

	writeJSON(w, http.StatusOK, resp)
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
	appliedSet := make(map[string]bool, len(activeNames))
	for _, name := range activeNames {
		appliedSet[strings.ToLower(name)] = true
	}

	for _, prov := range s.editor.GetRuleProviders() {
		appliedSet[strings.ToLower(prov.Name)] = true
	}

	items := make([]CatalogItemResponse, len(files))
	for i, f := range files {
		cleanName := f.Name
		isApplied := appliedSet[strings.ToLower(cleanName)]
		if !isApplied && f.Category != "" {
			isApplied = appliedSet[strings.ToLower(f.Category+"-"+cleanName)]
		}

		items[i] = CatalogItemResponse{
			Name:     cleanName,
			Path:     f.Path,
			Category: f.Category,
			Behavior: f.Behavior,
			Size:     f.Size,
			URL:      f.URL,
			Applied:  isApplied,
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

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleAddRule(w, r)
	case http.MethodDelete:
		s.handleDeleteRule(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only POST or DELETE allowed on /api/rules")
	}
}

func (s *Server) handleAddRule(w http.ResponseWriter, r *http.Request) {
	var req AddRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.ProxyGroup = strings.TrimSpace(req.ProxyGroup)
	req.URL = strings.TrimSpace(req.URL)

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Rule name is required")
		return
	}
	if req.ProxyGroup == "" {
		writeError(w, http.StatusBadRequest, "Target proxy_group is required")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "Ruleset URL is required")
		return
	}

	if req.Behavior == "" {
		if strings.Contains(strings.ToLower(req.Name), "geoip") {
			req.Behavior = "ipcidr"
		} else {
			req.Behavior = "domain"
		}
	}
	if req.Format == "" {
		req.Format = "mrs"
	}
	if req.Path == "" {
		req.Path = fmt.Sprintf("rules/%s.mrs", req.Name)
	}

	provEntry := config.RuleProviderEntry{
		Name:     req.Name,
		Type:     "http",
		Behavior: req.Behavior,
		Format:   req.Format,
		URL:      req.URL,
		Path:     req.Path,
		Interval: 86400,
	}

	if err := s.editor.AddRuleProvider(provEntry); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add rule-provider: %v", err))
		return
	}

	if err := s.editor.AddRule(req.Name, req.ProxyGroup); err != nil {
		_ = s.editor.RemoveRuleProvider(req.Name)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add rule line: %v", err))
		return
	}

	if err := s.editor.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save config: %v", err))
		return
	}

	go func() {
		if err := s.ReloadMihomo(); err != nil {
			log.Printf("[Mihomo] Reload failed: %v", err)
		}
	}()

	writeJSON(w, http.StatusOK, SuccessResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Rule %q added for group %q and Mihomo reloaded", req.Name, req.ProxyGroup),
	})
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = strings.TrimSpace(r.URL.Query().Get("provider"))
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "Query parameter 'name' or 'provider' required")
		return
	}

	deleteFileParam := strings.ToLower(r.URL.Query().Get("delete_file"))
	deleteFile := deleteFileParam == "true" || deleteFileParam == "1" || deleteFileParam == ""

	configuredPath := s.editor.GetRuleProviderPath(name)

	ruleErr := s.editor.RemoveRule(name)
	provErr := s.editor.RemoveRuleProvider(name)

	if ruleErr != nil && provErr != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Rule or provider %q not found", name))
		return
	}

	if deleteFile {
		if configuredPath != "" {
			fullPath := configuredPath
			if !filepath.IsAbs(fullPath) {
				fullPath = filepath.Join(s.rulesDir, filepath.Base(configuredPath))
			}
			if err := os.Remove(fullPath); err == nil {
				log.Printf("[Disk] Removed %s", fullPath)
			}
		}
		fallbackPath := filepath.Join(s.rulesDir, name+".mrs")
		if _, err := os.Stat(fallbackPath); err == nil {
			_ = os.Remove(fallbackPath)
		}
	}

	if err := s.editor.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save config: %v", err))
		return
	}

	go func() {
		if err := s.ReloadMihomo(); err != nil {
			log.Printf("[Mihomo] Reload failed: %v", err)
		}
	}()

	writeJSON(w, http.StatusOK, SuccessResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Rule %q removed and Mihomo reloaded", name),
	})
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if err := s.ReloadMihomo(); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Mihomo reload failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, SuccessResponse{
		Status:  "ok",
		Message: "Mihomo configuration reloaded successfully",
	})
}

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"miru/internal/config"
)

type AddRuleRequest struct {
	Name       string `json:"name"`
	Behavior   string `json:"behavior"`
	Format     string `json:"format"`
	URL        string `json:"url"`
	ProxyGroup string `json:"proxy_group"`
	Path       string `json:"path"`
	BlockQUIC  bool   `json:"block_quic"`
}

type EditRuleRequest struct {
	Name       string `json:"name"`
	ProxyGroup string `json:"proxy_group"`
	BlockQUIC  bool   `json:"block_quic"`
	URL        string `json:"url,omitempty"`
	Behavior   string `json:"behavior,omitempty"`
	Format     string `json:"format,omitempty"`
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleAddRule(w, r)
	case http.MethodPut:
		s.handleEditRule(w, r)
	case http.MethodDelete:
		s.handleDeleteRule(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only POST, PUT or DELETE allowed on /api/rules")
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

	if req.BlockQUIC {
		_ = s.editor.SetQuicRule(req.Name, true)
	}

	if err := s.editor.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save config: %v", err))
		return
	}

	s.asyncReloadMihomo("add rule " + req.Name)

	writeJSON(w, http.StatusOK, SuccessResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Rule %q added for group %q and Mihomo reloaded", req.Name, req.ProxyGroup),
	})
}

func (s *Server) handleEditRule(w http.ResponseWriter, r *http.Request) {
	var req EditRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.ProxyGroup = strings.TrimSpace(req.ProxyGroup)

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Rule name is required")
		return
	}

	if req.ProxyGroup != "" {
		if err := s.editor.AddRule(req.Name, req.ProxyGroup); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update rule target: %v", err))
			return
		}
	}

	if err := s.editor.SetQuicRule(req.Name, req.BlockQUIC); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update QUIC rule: %v", err))
		return
	}

	if req.URL != "" || req.Behavior != "" || req.Format != "" {
		_ = s.editor.UpdateRuleProvider(req.Name, req.URL, req.Behavior, req.Format)
	}

	if err := s.editor.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save config: %v", err))
		return
	}

	s.asyncReloadMihomo("edit rule " + req.Name)

	writeJSON(w, http.StatusOK, SuccessResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Rule %q updated successfully", req.Name),
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

	s.asyncReloadMihomo("delete rule " + name)

	writeJSON(w, http.StatusOK, SuccessResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Rule %q removed and Mihomo reloaded", name),
	})
}

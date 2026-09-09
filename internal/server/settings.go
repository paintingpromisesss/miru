package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"miru/internal/config"
)

// --- Proxy Groups API ---

func (s *Server) handleProxyGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var entry config.ProxyGroupEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.Name == "" {
			writeError(w, http.StatusBadRequest, "Proxy group name cannot be empty")
			return
		}
		if err := s.editor.AddProxyGroup(entry); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo("add proxy-group " + entry.Name)
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: fmt.Sprintf("Proxy group %q added", entry.Name)})

	case http.MethodPut:
		var req struct {
			OldName string `json:"old_name"`
			config.ProxyGroupEntry
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}
		targetName := strings.TrimSpace(req.OldName)
		if targetName == "" {
			targetName = strings.TrimSpace(req.Name)
		}
		if targetName == "" {
			writeError(w, http.StatusBadRequest, "Target proxy group name is required")
			return
		}
		if err := s.editor.UpdateProxyGroup(targetName, req.ProxyGroupEntry); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo("update proxy-group " + req.Name)
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: fmt.Sprintf("Proxy group %q updated", req.Name)})

	case http.MethodDelete:
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeError(w, http.StatusBadRequest, "Parameter 'name' is required")
			return
		}
		if err := s.editor.DeleteProxyGroup(name); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo("delete proxy-group " + name)
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: fmt.Sprintf("Proxy group %q deleted", name)})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Only POST, PUT, DELETE allowed")
	}
}

// --- Proxy Providers API ---

func (s *Server) handleProxyProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var entry config.ProxyProviderEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.Name == "" {
			writeError(w, http.StatusBadRequest, "Proxy provider name cannot be empty")
			return
		}
		if err := s.editor.AddProxyProvider(entry); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo("add proxy-provider " + entry.Name)
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: fmt.Sprintf("Proxy provider %q added", entry.Name)})

	case http.MethodPut:
		var req struct {
			OldName string `json:"old_name"`
			config.ProxyProviderEntry
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}
		targetName := strings.TrimSpace(req.OldName)
		if targetName == "" {
			targetName = strings.TrimSpace(req.Name)
		}
		if targetName == "" {
			writeError(w, http.StatusBadRequest, "Target proxy provider name is required")
			return
		}
		if err := s.editor.UpdateProxyProvider(targetName, req.ProxyProviderEntry); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo("update proxy-provider " + req.Name)
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: fmt.Sprintf("Proxy provider %q updated", req.Name)})

	case http.MethodDelete:
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeError(w, http.StatusBadRequest, "Parameter 'name' is required")
			return
		}
		if err := s.editor.DeleteProxyProvider(name); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo("delete proxy-provider " + name)
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: fmt.Sprintf("Proxy provider %q deleted", name)})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Only POST, PUT, DELETE allowed")
	}
}

// --- Raw Rules API ---

func (s *Server) handleRawRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Rule  string `json:"rule"`
			Index int    `json:"index"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}
		req.Rule = strings.TrimSpace(req.Rule)
		if req.Rule == "" {
			writeError(w, http.StatusBadRequest, "Rule string cannot be empty")
			return
		}
		if err := s.editor.AddRuleRaw(req.Rule, req.Index); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo("add raw rule")
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: "Rule added successfully"})

	case http.MethodPut:
		var req struct {
			Index int    `json:"index"`
			Rule  string `json:"rule"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}
		req.Rule = strings.TrimSpace(req.Rule)
		if req.Rule == "" {
			writeError(w, http.StatusBadRequest, "Rule string cannot be empty")
			return
		}
		if err := s.editor.UpdateRuleAt(req.Index, req.Rule); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo("update raw rule")
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: "Rule updated successfully"})

	case http.MethodDelete:
		idxStr := strings.TrimSpace(r.URL.Query().Get("index"))
		if idxStr == "" {
			writeError(w, http.StatusBadRequest, "Parameter 'index' is required")
			return
		}
		idx, err := strconv.Atoi(idxStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid index: "+idxStr)
			return
		}
		if err := s.editor.DeleteRuleAt(idx); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.editor.Save(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
			return
		}
		s.asyncReloadMihomo(fmt.Sprintf("delete raw rule at %d", idx))
		writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: fmt.Sprintf("Rule at index %d deleted", idx)})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Only POST, PUT, DELETE allowed")
	}
}

func (s *Server) handleMoveRawRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	if err := s.editor.MoveRule(req.From, req.To); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.editor.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save config: "+err.Error())
		return
	}
	s.asyncReloadMihomo(fmt.Sprintf("move rule from %d to %d", req.From, req.To))
	writeJSON(w, http.StatusOK, SuccessResponse{Status: "ok", Message: fmt.Sprintf("Rule moved from %d to %d", req.From, req.To)})
}

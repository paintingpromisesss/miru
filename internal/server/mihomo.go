package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type MihomoStatus struct {
	Alive   bool   `json:"alive"`
	Version string `json:"version,omitempty"`
	API     string `json:"api"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) ReloadMihomo() error {
	url := fmt.Sprintf("%s/configs?force=true", s.mihomoAPI)

	payload := map[string]interface{}{
		"path":  s.configPath,
		"force": true,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal reload payload: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("create reload request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if s.mihomoSecret != "" {
			req.Header.Set("Authorization", "Bearer "+s.mihomoSecret)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request to %s: %w", url, err)
			time.Sleep(300 * time.Millisecond)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			log.Printf("[Mihomo] Config hot-reload successful")
			return nil
		}

		lastErr = fmt.Errorf("mihomo API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		time.Sleep(300 * time.Millisecond)
	}

	return lastErr
}

func (s *Server) CheckMihomo(ctx context.Context) MihomoStatus {
	status := MihomoStatus{
		Alive: false,
		API:   s.mihomoAPI,
	}

	if s.mihomoAPI == "" {
		status.Error = "Mihomo API URL is not configured"
		return status
	}

	pingCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	url := s.mihomoAPI + "/version"
	req, err := http.NewRequestWithContext(pingCtx, http.MethodGet, url, nil)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	secret := s.mihomoSecret
	if secret == "" && s.editor != nil {
		secret = s.editor.GetSecret()
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		status.Error = "Unreachable"
		return status
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		status.Error = "Unauthorized (invalid secret)"
		return status
	}

	if resp.StatusCode != http.StatusOK {
		status.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return status
	}

	var res struct {
		Version string `json:"version"`
		Meta    bool   `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Version != "" {
		status.Alive = true
		status.Version = res.Version
		return status
	}

	status.Alive = true
	status.Version = "Connected"
	return status
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

func (s *Server) handleMihomoStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	status := s.CheckMihomo(r.Context())
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleGlobalQuic(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"global_quic_blocked": s.editor.HasGlobalQuicRule(),
		})
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "Only GET, POST or PUT allowed on /api/quic/global")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if err := s.editor.SetGlobalQuicRule(req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update global QUIC rule: %v", err))
		return
	}

	if err := s.editor.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save config: %v", err))
		return
	}

	s.asyncReloadMihomo("global QUIC toggle")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":              "ok",
		"global_quic_blocked": req.Enabled,
	})
}

func (s *Server) asyncReloadMihomo(action string) {
	go func() {
		if err := s.ReloadMihomo(); err != nil {
			log.Printf("[Mihomo] Reload warning after %s: %v", action, err)
		}
	}()
}


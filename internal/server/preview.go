package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miru/internal/mrs"
)

type RulePreviewResponse struct {
	Name   string   `json:"name"`
	Source string   `json:"source"` // "local" or "remote"
	Type   string   `json:"type"`   // "domain", "ipcidr", "classical"
	Count  int      `json:"count"`
	Rules  []string `json:"rules"`
}

func (s *Server) handleRulePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))

	var data []byte
	var source string

	if name != "" {
		for _, p := range s.editor.GetRuleProviders() {
			if p.Name == name {
				configuredPath := p.Path
				var fullPath string
				if filepath.IsAbs(configuredPath) {
					fullPath = configuredPath
				} else {
					fullPath = filepath.Join(s.rulesDir, filepath.Base(configuredPath))
				}

				if b, err := os.ReadFile(fullPath); err == nil && len(b) > 0 {
					data = b
					source = "local"
				} else if rawURL == "" && p.URL != "" {
					rawURL = p.URL
				}
				break
			}
		}

		if len(data) == 0 {
			for _, ext := range []string{".mrs", ".yaml", ".txt", ""} {
				candidate := filepath.Join(s.rulesDir, name+ext)
				if b, err := os.ReadFile(candidate); err == nil && len(b) > 0 {
					data = b
					source = "local"
					break
				}
			}
		}

		if len(data) == 0 && rawURL != "" {
			urlBase := filepath.Base(strings.Split(rawURL, "?")[0])
			if urlBase != "" && urlBase != "." && urlBase != "/" {
				candidate := filepath.Join(s.rulesDir, urlBase)
				if b, err := os.ReadFile(candidate); err == nil && len(b) > 0 {
					data = b
					source = "local"
				}
			}
		}
	}

	if len(data) == 0 {
		if rawURL == "" {
			writeError(w, http.StatusNotFound, fmt.Sprintf("Rule %q not found locally and no remote URL provided", name))
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, rawURL, nil)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid URL: %v", err))
			return
		}
		req.Header.Set("User-Agent", "Miru/1.0")

		client := &http.Client{Timeout: 20 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("Failed to fetch rule from remote: %v", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("Remote server returned HTTP %d", resp.StatusCode))
			return
		}

		b, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to read remote rule: %v", err))
			return
		}

		data = b
		source = "remote"
	}

	decoded, err := mrs.DecodeAny(data)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("Failed to decode ruleset: %v", err))
		return
	}

	respName := name
	if respName == "" {
		respName = filepath.Base(rawURL)
	}

	writeJSON(w, http.StatusOK, RulePreviewResponse{
		Name:   respName,
		Source: source,
		Type:   string(decoded.Type),
		Count:  len(decoded.Rules),
		Rules:  decoded.Rules,
	})
}

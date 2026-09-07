package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"miru/internal/catalog"
	"miru/internal/config"
	"miru/web"
)

type Server struct {
	editor       *config.YAMLEditor
	catalog      *catalog.Catalog
	rulesDir     string
	mihomoAPI    string
	mihomoSecret string
	configPath   string
	httpClient   *http.Client
}

type Config struct {
	Editor       *config.YAMLEditor
	Catalog      *catalog.Catalog
	RulesDir     string
	MihomoAPI    string
	MihomoSecret string
	ConfigPath   string
}

func New(cfg Config) *Server {
	secret := cfg.MihomoSecret
	if secret == "" && cfg.Editor != nil {
		secret = cfg.Editor.GetSecret()
	}

	return &Server{
		editor:       cfg.Editor,
		catalog:      cfg.Catalog,
		rulesDir:     cfg.RulesDir,
		mihomoAPI:    strings.TrimRight(cfg.MihomoAPI, "/"),
		mihomoSecret: secret,
		configPath:   cfg.ConfigPath,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// ReloadMihomo triggers PUT /configs?force=true on Mihomo External Controller
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

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/local", s.handleLocal)
	mux.HandleFunc("/api/catalog", s.handleCatalog)
	mux.HandleFunc("/api/rules", s.handleRules)
	mux.HandleFunc("/api/quic/global", s.handleGlobalQuic)
	mux.HandleFunc("/api/reload", s.handleReload)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		data, err := web.Files.ReadFile("index.html")
		if err != nil {
			http.Error(w, "Index file not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
}

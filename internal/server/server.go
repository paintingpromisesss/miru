package server

import (
	"encoding/json"
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

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/local", s.handleLocal)
	mux.HandleFunc("/api/catalog", s.handleCatalog)
	mux.HandleFunc("/api/rules", s.handleRules)
	mux.HandleFunc("/api/rules/preview", s.handleRulePreview)
	mux.HandleFunc("/api/proxy-groups", s.handleProxyGroups)
	mux.HandleFunc("/api/proxy-providers", s.handleProxyProviders)
	mux.HandleFunc("/api/raw-rules", s.handleRawRules)
	mux.HandleFunc("/api/raw-rules/move", s.handleMoveRawRule)
	mux.HandleFunc("/api/quic/global", s.handleGlobalQuic)
	mux.HandleFunc("/api/reload", s.handleReload)
	mux.HandleFunc("/api/mihomo/status", s.handleMihomoStatus)

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

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

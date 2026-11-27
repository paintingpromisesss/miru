package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
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
	mode string
	effectivePath string
	apply func() error
	txMu sync.Mutex
	pendingDeletes []string
	catalogs map[string]*catalog.Catalog
}

type Config struct {
	Editor       *config.YAMLEditor
	Catalog      *catalog.Catalog
	RulesDir     string
	MihomoAPI    string
	MihomoSecret string
	ConfigPath   string
	Mode string
	EffectivePath string
	Apply func() error // optional injection for tests
	Catalogs map[string]*catalog.Catalog
}

func New(cfg Config) *Server {
	if cfg.Mode == "" { cfg.Mode = "standalone" }
	secret := cfg.MihomoSecret
	if secret == "" && cfg.EffectivePath != "" { if e,err:=config.NewYAMLEditor(cfg.EffectivePath);err==nil {secret=e.GetSecret()} }
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
		mode: cfg.Mode, effectivePath:cfg.EffectivePath, apply:cfg.Apply, catalogs:cfg.Catalogs,
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/local", s.guard(s.handleLocal))
	mux.HandleFunc("/api/catalog", s.guard(s.handleCatalog))
	mux.HandleFunc("/api/rules", s.guard(s.handleRules))
	mux.HandleFunc("/api/rules/preview", s.guard(s.handleRulePreview))
	mux.HandleFunc("/api/proxy-groups", s.guard(s.handleProxyGroups))
	mux.HandleFunc("/api/proxy-providers", s.guard(s.handleProxyProviders))
	mux.HandleFunc("/api/raw-rules", s.guard(s.handleRawRules))
	mux.HandleFunc("/api/raw-rules/move", s.guard(s.handleMoveRawRule))
	mux.HandleFunc("/api/quic/global", s.guard(s.handleGlobalQuic))
	mux.HandleFunc("/api/reload", s.guard(s.handleReload))
	mux.HandleFunc("/api/mihomo/status", s.guard(s.handleMihomoStatus))

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

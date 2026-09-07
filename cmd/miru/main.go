package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"miru/internal/catalog"
	"miru/internal/config"
	"miru/internal/server"
)

var (
	configPath   = flag.String("config", "/etc/mihomo/config.yaml", "Path to mihomo config.yaml")
	port         = flag.String("port", "8080", "HTTP server listen port")
	rulesDir     = flag.String("rules-dir", "/etc/mihomo/rules", "Directory for downloaded .mrs files")
	mihomoAPI    = flag.String("mihomo-api", "http://127.0.0.1:9090", "Mihomo API base URL for hot-reload")
	mihomoSecret = flag.String("mihomo-secret", "", "Mihomo API secret (optional, auto-read from config if empty)")
	githubToken  = flag.String("github-token", "", "GitHub personal access token (avoids API rate limits)")
	catalogRepo  = flag.String("catalog-repo", "MetaCubeX/meta-rules-dat", "GitHub repo with .mrs files (owner/name)")
	catalogRef   = flag.String("catalog-ref", "meta", "Git ref/branch for catalog")
	refreshTTL   = flag.Duration("catalog-ttl", 12*time.Hour, "Catalog cache TTL in memory (default 12h)")
	githubProxy  = flag.String("github-proxy", "auto", "Proxy URL for GitHub requests ('auto' to read mixed-port/port from config, 'none' to disable)")

	AppVersion = "1.1.0"
)

func main() {
	flag.Parse()

	if _, err := os.Stat(*configPath); err != nil {
		log.Printf("[WARNING] Config file not found at %s: %v", *configPath, err)
	}

	if err := os.MkdirAll(*rulesDir, 0755); err != nil {
		log.Printf("[WARNING] Could not ensure rules directory %s: %v", *rulesDir, err)
	}

	editor, err := config.NewYAMLEditor(*configPath)
	if err != nil {
		log.Fatalf("Failed to open or parse config file %s: %v", *configPath, err)
	}

	resolvedProxy := ""
	switch strings.ToLower(*githubProxy) {
	case "none", "off", "direct":
		resolvedProxy = ""
	case "auto", "":
		resolvedProxy = editor.GetInboundProxy()
	default:
		resolvedProxy = *githubProxy
	}

	if resolvedProxy != "" {
		log.Printf("[Catalog] Routing GitHub requests through proxy: %s", resolvedProxy)
	}

	cat := catalog.NewCatalog(*catalogRepo, *catalogRef, *githubToken, *refreshTTL, resolvedProxy)

	srv := server.New(server.Config{
		Editor:       editor,
		Catalog:      cat,
		RulesDir:     *rulesDir,
		MihomoAPI:    *mihomoAPI,
		MihomoSecret: *mihomoSecret,
		ConfigPath:   *configPath,
	})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%s", *port)
	fmt.Printf("Miru %s listening on http://0.0.0.0%s (config: %s)\n", AppVersion, addr, *configPath)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

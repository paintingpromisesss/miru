package main

import (
	"flag"
	"crypto/subtle"
	"net"
	"net/url"
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
	mode = flag.String("mode","standalone","standalone or nikki")
	noAuth = flag.Bool("no-auth",false,"Explicitly disable authentication (trusted LAN only)")
	version = flag.Bool("version",false,"Print version and exit")
	check = flag.Bool("check-config",false,"Validate startup configuration and exit")
	bind = flag.String("bind","127.0.0.1","HTTP listen address")
	configPath   = flag.String("config", "/etc/mihomo/config.yaml", "Path to mihomo config.yaml")
	port         = flag.String("port", "8787", "HTTP server listen port")
	rulesDir     = flag.String("rules-dir", "/etc/mihomo/rules", "Directory for downloaded .mrs files")
	mihomoAPI    = flag.String("mihomo-api", "http://127.0.0.1:9090", "Mihomo API base URL for hot-reload")
	mihomoSecret = flag.String("mihomo-secret", "", "Mihomo API secret (optional, auto-read from config if empty)")
	githubToken  = flag.String("github-token", "", "GitHub personal access token (avoids API rate limits)")
	catalogRepo  = flag.String("catalog-repo", "MetaCubeX/meta-rules-dat", "GitHub repo with .mrs files (owner/name)")
	catalogRef   = flag.String("catalog-ref", "meta", "Git ref/branch for catalog")
	refreshTTL   = flag.Duration("catalog-ttl", 12*time.Hour, "Catalog cache TTL in memory (default 12h)")
	githubProxy  = flag.String("github-proxy", "auto", "Proxy URL for GitHub requests ('auto' to read mixed-port/port from config, 'none' to disable)")

	AppVersion = "v1.0.0"
)

func main() {
	flag.Parse()
	if *version {fmt.Println(AppVersion);return}
	set:=map[string]bool{};flag.Visit(func(f *flag.Flag){set[f.Name]=true})
	for key,env:=range map[string]string{"mode":"MIRU_MODE","bind":"MIRU_BIND","config":"MIRU_CONFIG","port":"MIRU_PORT","rules-dir":"MIRU_RULES_DIR","mihomo-api":"MIRU_MIHOMO_API","mihomo-secret":"MIRU_MIHOMO_SECRET","github-token":"MIRU_GITHUB_TOKEN","github-proxy":"MIRU_GITHUB_PROXY","no-auth":"MIRU_NO_AUTH"} {
		if !set[key] {if value:=os.Getenv(env);value!="" {if err:=flag.Set(key,value);err!=nil{log.Fatal(err)};set[key]=true}}
	}
	effectivePath:=""
	if *mode=="nikki" {
		if !set["config"] {*configPath="/etc/nikki/mixin.yaml"}
		if !set["rules-dir"] {*rulesDir="/etc/nikki/run/ruleset"}
		if !set["catalog-repo"] {*catalogRepo="paintingpromisesss/tachyon-mihomo"}
		if !set["catalog-ref"] && *catalogRepo=="paintingpromisesss/tachyon-mihomo" {*catalogRef="generated"}
		if !set["github-proxy"] {*githubProxy="none"}
		effectivePath="/etc/nikki/run/config.yaml"
	} else if *mode!="standalone" {log.Fatal("mode must be standalone or nikki")}
	password:=os.Getenv("MIRU_PASSWORD")
	if *noAuth {password="";log.Print("WARNING: authentication explicitly disabled")} else if *bind!="127.0.0.1" && *bind!="::1" && password=="" { log.Fatal("MIRU_PASSWORD or explicit -no-auth is required for non-loopback bind") }

	if _, err := os.Stat(*configPath); err != nil {
		log.Fatalf("Config file not found at %s: %v", *configPath, err)
	}

	if err := os.MkdirAll(*rulesDir, 0755); err != nil {
		log.Fatalf("Could not ensure rules directory %s: %v", *rulesDir, err)
	}

	editor, err := config.NewModeEditor(*configPath,*mode)
	if err != nil {
		log.Fatalf("Failed to open or parse config file %s: %v", *configPath, err)
	}

	if *check {fmt.Println("Configuration OK");return}
	resolvedProxy := ""
	switch strings.ToLower(*githubProxy) {
	case "none", "off", "direct":
		resolvedProxy = ""
	case "auto", "":
		resolvedProxy = editor.GetInboundProxy()
		if effectivePath!="" {if effective,err:=config.NewYAMLEditor(effectivePath);err==nil {resolvedProxy=effective.GetInboundProxy()}}
	default:
		resolvedProxy = *githubProxy
	}

	if resolvedProxy != "" {
		log.Printf("[Catalog] Routing GitHub requests through proxy: %s", resolvedProxy)
	}

	cat := catalog.NewCatalog(*catalogRepo, *catalogRef, *githubToken, *refreshTTL, resolvedProxy)
	catalogs:=map[string]*catalog.Catalog{
		"tachyon":catalog.NewCatalog("paintingpromisesss/tachyon-mihomo","generated","",*refreshTTL,""),
		"metacubex":catalog.NewCatalog("MetaCubeX/meta-rules-dat","meta",*githubToken,*refreshTTL,resolvedProxy),
	}

	srv := server.New(server.Config{
		Editor:       editor,
		Catalog:      cat,
		RulesDir:     *rulesDir,
		MihomoAPI:    *mihomoAPI,
		MihomoSecret: *mihomoSecret,
		ConfigPath:   *configPath,
		Mode:*mode,EffectivePath:effectivePath,Catalogs:catalogs,
	})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	addr := net.JoinHostPort(*bind,*port)
	fmt.Printf("Miru %s listening on http://%s (config: %s)\n", AppVersion, addr, *configPath)

	httpServer := &http.Server{
		Addr:              addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
			if r.URL.Path=="/healthz" && r.Method=="GET" {w.Header().Set("Cache-Control","no-store");w.Write([]byte("miru "+AppVersion+"\n"));return}
			if password!="" {u,p,ok:=r.BasicAuth();if !ok||u!="miru"||subtle.ConstantTimeCompare([]byte(p),[]byte(password))!=1 {w.Header().Set("WWW-Authenticate",`Basic realm="Miru"`);http.Error(w,"Unauthorized",401);return}}
			if r.Method!="GET" && r.Method!="HEAD" {if origin:=r.Header.Get("Origin");origin!="" {o,err:=url.Parse(origin);if err!=nil||o.Host!=r.Host {http.Error(w,"Cross-origin request denied",403);return}};if r.Header.Get("Sec-Fetch-Site")=="cross-site" {http.Error(w,"Cross-site request denied",403);return}}
			w.Header().Set("Cache-Control","no-store");mux.ServeHTTP(w,r)
		}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

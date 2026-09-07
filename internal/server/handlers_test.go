package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"miru/internal/catalog"
	"miru/internal/config"
)

const testYAML = `
proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - DIRECT
  - name: UNBLOCK
    type: select
    proxies:
      - DIRECT

rule-providers: {}

rules:
  - MATCH,DIRECT
`

func setupTestServer(t *testing.T) (*Server, *http.ServeMux, string, string) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	rulesDir := filepath.Join(tmpDir, "rules")
	_ = os.MkdirAll(rulesDir, 0755)

	if err := os.WriteFile(cfgPath, []byte(testYAML), 0644); err != nil {
		t.Fatalf("Write temp yaml: %v", err)
	}

	editor, err := config.NewYAMLEditor(cfgPath)
	if err != nil {
		t.Fatalf("NewYAMLEditor: %v", err)
	}

	cat := catalog.NewCatalog("MetaCubeX/meta-rules-dat", "meta", "", 1*time.Hour)

	srv := New(Config{
		Editor:     editor,
		Catalog:    cat,
		RulesDir:   rulesDir,
		MihomoAPI:  "http://127.0.0.1:9090",
		ConfigPath: cfgPath,
	})

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	return srv, mux, cfgPath, rulesDir
}

func TestAPI_LocalAndRules(t *testing.T) {
	_, mux, cfgPath, rulesDir := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/local", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/local returned HTTP %d: %s", rec.Code, rec.Body.String())
	}

	var localResp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&localResp); err != nil {
		t.Fatalf("Decode local response: %v", err)
	}

	groups, ok := localResp["proxy_groups"].([]interface{})
	if !ok || len(groups) == 0 {
		t.Fatalf("Expected proxy_groups, got %v", localResp["proxy_groups"])
	}

	addPayload := AddRuleRequest{
		Name:       "youtube",
		Behavior:   "domain",
		Format:     "mrs",
		URL:        "https://example.com/youtube.mrs",
		ProxyGroup: "UNBLOCK",
	}
	body, _ := json.Marshal(addPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/rules failed with %d: %s", rec.Code, rec.Body.String())
	}

	cfgBytes, _ := os.ReadFile(cfgPath)
	cfgStr := string(cfgBytes)
	if !strings.Contains(cfgStr, "RULE-SET,youtube,UNBLOCK") {
		t.Errorf("Expected RULE-SET,youtube,UNBLOCK in config: %s", cfgStr)
	}
	if !strings.Contains(cfgStr, "youtube:") {
		t.Errorf("Expected youtube rule-provider in config: %s", cfgStr)
	}

	dummyFile := filepath.Join(rulesDir, "youtube.mrs")
	_ = os.WriteFile(dummyFile, []byte("dummy-mrs-data"), 0644)

	req = httptest.NewRequest(http.MethodDelete, "/api/rules?name=youtube&delete_file=true", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/rules failed with %d: %s", rec.Code, rec.Body.String())
	}

	cfgBytesAfter, _ := os.ReadFile(cfgPath)
	cfgStrAfter := string(cfgBytesAfter)
	if strings.Contains(cfgStrAfter, "RULE-SET,youtube") {
		t.Errorf("Rule was not removed from config: %s", cfgStrAfter)
	}
	if _, err := os.Stat(dummyFile); !os.IsNotExist(err) {
		t.Errorf("Dummy .mrs file was not deleted from disk")
	}
}

func TestWebUIEmbedded(t *testing.T) {
	_, mux, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returned HTTP %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Miru — Mihomo Rule Manager") {
		t.Fatalf("Embedded index.html does not contain expected title: %s", body)
	}
}

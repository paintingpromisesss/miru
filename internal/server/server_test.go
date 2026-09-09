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

	cat := catalog.NewCatalog("MetaCubeX/meta-rules-dat", "meta", "", 1*time.Hour, "")

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

	// Test PUT /api/rules to edit rule and enable BlockQUIC
	editPayload := EditRuleRequest{
		Name:       "youtube",
		ProxyGroup: "STABLE",
		BlockQUIC:  true,
	}
	editBody, _ := json.Marshal(editPayload)
	req = httptest.NewRequest(http.MethodPut, "/api/rules", bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/rules failed with %d: %s", rec.Code, rec.Body.String())
	}

	cfgBytesEdited, _ := os.ReadFile(cfgPath)
	cfgStrEdited := string(cfgBytesEdited)
	if !strings.Contains(cfgStrEdited, "RULE-SET,youtube,STABLE") {
		t.Errorf("Expected updated RULE-SET,youtube,STABLE in config: %s", cfgStrEdited)
	}
	if !strings.Contains(cfgStrEdited, "AND,((RULE-SET,youtube),(NETWORK,udp),(DST-PORT,443)),REJECT") {
		t.Errorf("Expected QUIC block rule in config: %s", cfgStrEdited)
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

	// Test Global QUIC API endpoint
	quicBody, _ := json.Marshal(map[string]bool{"enabled": true})
	req = httptest.NewRequest(http.MethodPost, "/api/quic/global", bytes.NewReader(quicBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/quic/global failed with %d: %s", rec.Code, rec.Body.String())
	}

	cfgBytesQuic, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(cfgBytesQuic), "AND,((NETWORK,udp),(DST-PORT,443)),REJECT") {
		t.Errorf("Expected global QUIC rule in config: %s", string(cfgBytesQuic))
	}

	// Disable global QUIC
	quicOffBody, _ := json.Marshal(map[string]bool{"enabled": false})
	req = httptest.NewRequest(http.MethodPost, "/api/quic/global", bytes.NewReader(quicOffBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/quic/global (off) failed with %d: %s", rec.Code, rec.Body.String())
	}

	cfgBytesQuicOff, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(cfgBytesQuicOff), "AND,((NETWORK,udp),(DST-PORT,443)),REJECT") {
		t.Errorf("Expected global QUIC rule removed from config: %s", string(cfgBytesQuicOff))
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

func TestAPI_RulePreview(t *testing.T) {
	_, mux, _, rulesDir := setupTestServer(t)

	// 1. Test local text rule preview
	localRulePath := filepath.Join(rulesDir, "custom.txt")
	_ = os.WriteFile(localRulePath, []byte("domain:example.com\n+.google.com\n1.1.1.1/32\n"), 0644)

	req := httptest.NewRequest(http.MethodGet, "/api/rules/preview?name=custom", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Preview local rule failed with HTTP %d: %s", rec.Code, rec.Body.String())
	}

	var preview RulePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("Unmarshal preview response: %v", err)
	}

	if preview.Source != "local" {
		t.Errorf("Expected source 'local', got %s", preview.Source)
	}
	if preview.Count != 3 {
		t.Errorf("Expected 3 rules, got %d", preview.Count)
	}

	// 2. Test remote MRS preview (Telegram GeoIP)
	remoteURL := "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geoip/telegram.mrs"
	reqRemote := httptest.NewRequest(http.MethodGet, "/api/rules/preview?url="+remoteURL, nil)
	recRemote := httptest.NewRecorder()
	mux.ServeHTTP(recRemote, reqRemote)

	if recRemote.Code == http.StatusOK {
		var remotePreview RulePreviewResponse
		if err := json.Unmarshal(recRemote.Body.Bytes(), &remotePreview); err != nil {
			t.Fatalf("Unmarshal remote preview response: %v", err)
		}
		if remotePreview.Source != "remote" {
			t.Errorf("Expected source 'remote', got %s", remotePreview.Source)
		}
		if remotePreview.Type != "ipcidr" {
			t.Errorf("Expected type 'ipcidr', got %s", remotePreview.Type)
		}
		if remotePreview.Count == 0 {
			t.Errorf("Expected non-zero rules count")
		}
	}
}

func TestCheckMihomo(t *testing.T) {
	// 1. Mock Mihomo server that returns version
	mockMihomo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer testsecret" {
			http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"v1.19.2","meta":true}`))
	}))
	defer mockMihomo.Close()

	// Server with correct secret
	srv := New(Config{
		MihomoAPI:    mockMihomo.URL,
		MihomoSecret: "testsecret",
	})
	status := srv.CheckMihomo(t.Context())
	if !status.Alive {
		t.Fatalf("Expected alive=true, got error: %s", status.Error)
	}
	if status.Version != "v1.19.2" {
		t.Fatalf("Expected version 'v1.19.2', got '%s'", status.Version)
	}

	// Server with wrong secret
	srvWrong := New(Config{
		MihomoAPI:    mockMihomo.URL,
		MihomoSecret: "wrongsecret",
	})
	statusWrong := srvWrong.CheckMihomo(t.Context())
	if statusWrong.Alive {
		t.Fatalf("Expected alive=false for wrong secret")
	}
	if !strings.Contains(statusWrong.Error, "Unauthorized") {
		t.Fatalf("Expected unauthorized error, got: %s", statusWrong.Error)
	}

	// Test GET /api/mihomo/status route
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/mihomo/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/mihomo/status returned %d", rec.Code)
	}
	var apiStatus MihomoStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &apiStatus); err != nil {
		t.Fatalf("Unmarshal api status: %v", err)
	}
	if !apiStatus.Alive || apiStatus.Version != "v1.19.2" {
		t.Fatalf("Unexpected api status: %+v", apiStatus)
	}
}

func TestCatalogURLMatchingAndDiff(t *testing.T) {
	srv, mux, _, rulesDir := setupTestServer(t)

	// Inject mock catalog items
	srv.catalog.SetFilesForTest([]catalog.GitHubFile{
		{
			Name:     "google_gemini",
			Path:     "geo/geosite/google_gemini.mrs",
			Category: "geosite",
			Behavior: "domain",
			Size:     1000,
			URL:      "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/google_gemini.mrs",
		},
		{
			Name:     "github",
			Path:     "geo/geosite/github.mrs",
			Category: "geosite",
			Behavior: "domain",
			Size:     2000,
			URL:      "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/github.mrs",
		},
	})

	// Add custom rule provider named 'gemini_rules' having URL pointing to google_gemini.mrs
	err := srv.editor.AddRuleProvider(config.RuleProviderEntry{
		Name:     "gemini_rules",
		Type:     "http",
		Behavior: "domain",
		Format:   "mrs",
		URL:      "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/google_gemini.mrs",
		Path:     "rules/google_gemini.mrs",
	})
	if err != nil {
		t.Fatalf("AddRuleProvider failed: %v", err)
	}
	_ = srv.editor.AddRule("gemini_rules", "PROXY")
	_ = srv.editor.Save()

	// Write file to disk with matching size (1000 bytes)
	diskPath := filepath.Join(rulesDir, "google_gemini.mrs")
	_ = os.WriteFile(diskPath, make([]byte, 1000), 0644)

	// Call GET /api/catalog
	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/catalog returned %d: %s", rec.Code, rec.Body.String())
	}

	var catResp struct {
		Items []CatalogItemResponse `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catResp); err != nil {
		t.Fatalf("Unmarshal catalog response: %v", err)
	}

	var geminiItem *CatalogItemResponse
	for _, it := range catResp.Items {
		if it.Name == "google_gemini" {
			i := it
			geminiItem = &i
			break
		}
	}

	if geminiItem == nil {
		t.Fatalf("google_gemini not found in catalog response")
	}

	if !geminiItem.Applied {
		t.Errorf("Expected google_gemini to be matched and Applied=true, got false")
	}
	if geminiItem.AppliedName != "gemini_rules" {
		t.Errorf("Expected AppliedName='gemini_rules', got %q", geminiItem.AppliedName)
	}
	if !geminiItem.Downloaded {
		t.Errorf("Expected Downloaded=true, got false")
	}
	if geminiItem.HasDiff {
		t.Errorf("Expected HasDiff=false when sizes match, got true")
	}

	// Now modify disk file size to 1500 bytes -> HasDiff should be true
	_ = os.WriteFile(diskPath, make([]byte, 1500), 0644)

	recDiff := httptest.NewRecorder()
	mux.ServeHTTP(recDiff, req)

	_ = json.Unmarshal(recDiff.Body.Bytes(), &catResp)
	for _, it := range catResp.Items {
		if it.Name == "google_gemini" {
			if !it.HasDiff {
				t.Errorf("Expected HasDiff=true when size differs (1500 vs 1000), got false")
			}
			break
		}
	}
}

func TestSettingsEndpoints(t *testing.T) {
	_, mux, _, _ := setupTestServer(t)

	// 1. Test POST /api/proxy-groups
	grpPayload := map[string]interface{}{
		"name":     "FALLBACK-TEST",
		"type":     "fallback",
		"proxies":  []string{"DIRECT"},
		"url":      "http://cp.cloudflare.com/generate_204",
		"interval": 300,
	}
	b, _ := json.Marshal(grpPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/proxy-groups", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/proxy-groups failed: %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Test PUT /api/proxy-groups
	grpPayload["interval"] = 600
	b, _ = json.Marshal(grpPayload)
	req = httptest.NewRequest(http.MethodPut, "/api/proxy-groups", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/proxy-groups failed: %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Test DELETE /api/proxy-groups
	req = httptest.NewRequest(http.MethodDelete, "/api/proxy-groups?name=FALLBACK-TEST", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/proxy-groups failed: %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Test POST /api/proxy-providers
	provPayload := map[string]interface{}{
		"name":     "sub_test",
		"type":     "http",
		"url":      "https://example.com/sub.yaml",
		"path":     "./proxy_providers/sub_test.yaml",
		"interval": 3600,
	}
	b, _ = json.Marshal(provPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/proxy-providers", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/proxy-providers failed: %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Test DELETE /api/proxy-providers
	req = httptest.NewRequest(http.MethodDelete, "/api/proxy-providers?name=sub_test", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/proxy-providers failed: %d: %s", rec.Code, rec.Body.String())
	}

	// 6. Test Raw Rules: POST, PUT, MOVE, DELETE
	ruleReq := map[string]interface{}{
		"rule":  "DOMAIN-SUFFIX,google.com,PROXY",
		"index": 0,
	}
	b, _ = json.Marshal(ruleReq)
	req = httptest.NewRequest(http.MethodPost, "/api/raw-rules", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/raw-rules failed: %d: %s", rec.Code, rec.Body.String())
	}

	// PUT
	ruleReq = map[string]interface{}{
		"index": 0,
		"rule":  "DOMAIN-SUFFIX,google.com,UNBLOCK",
	}
	b, _ = json.Marshal(ruleReq)
	req = httptest.NewRequest(http.MethodPut, "/api/raw-rules", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/raw-rules failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Move
	moveReq := map[string]interface{}{
		"from": 0,
		"to":   1,
	}
	b, _ = json.Marshal(moveReq)
	req = httptest.NewRequest(http.MethodPost, "/api/raw-rules/move", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/raw-rules/move failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/raw-rules?index=1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/raw-rules failed: %d: %s", rec.Code, rec.Body.String())
	}
}



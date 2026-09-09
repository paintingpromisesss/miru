package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfig = `# Top comment preserving test
port: 7890
socks-port: 7891
secret: "router12345"

proxy-groups:
  - name: UNBLOCK
    type: select
    proxies:
      - DIRECT
  - name: STABLE
    type: select
    proxies:
      - DIRECT

rule-providers:
  apple:
    type: http
    behavior: domain
    format: mrs
    url: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/apple.mrs"
    path: ./rules/apple.mrs
    interval: 86400

rules:
  - DOMAIN-SUFFIX,local,DIRECT
  - RULE-SET,apple,UNBLOCK
  # Final fallback
  - MATCH,DIRECT
`

func TestYAMLEditor(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte(sampleConfig), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	editor, err := NewYAMLEditor(cfgPath)
	if err != nil {
		t.Fatalf("NewYAMLEditor failed: %v", err)
	}

	if secret := editor.GetSecret(); secret != "router12345" {
		t.Errorf("Expected secret 'router12345', got %q", secret)
	}

	if proxy := editor.GetInboundProxy(); proxy != "http://127.0.0.1:7890" {
		t.Errorf("Expected inbound proxy 'http://127.0.0.1:7890', got %q", proxy)
	}

	groups := editor.GetProxyGroups()
	foundUnblock := false
	for _, g := range groups {
		if g == "UNBLOCK" {
			foundUnblock = true
		}
	}
	if !foundUnblock {
		t.Errorf("Expected UNBLOCK in proxy groups, got %v", groups)
	}

	providers := editor.GetRuleProviders()
	if len(providers) != 1 || providers[0].Name != "apple" {
		t.Errorf("Expected 1 provider 'apple', got %v", providers)
	}

	err = editor.AddRuleProvider(RuleProviderEntry{
		Name:     "youtube",
		Type:     "http",
		Behavior: "domain",
		Format:   "mrs",
		URL:      "https://example.com/youtube.mrs",
		Path:     "rules/youtube.mrs",
		Interval: 86400,
	})
	if err != nil {
		t.Fatalf("AddRuleProvider failed: %v", err)
	}

	if err := editor.AddRule("youtube", "STABLE"); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}

	rules := editor.GetRules()
	matchIdx := -1
	youtubeIdx := -1
	for i, r := range rules {
		if strings.Contains(r, "MATCH") {
			matchIdx = i
		}
		if strings.Contains(r, "RULE-SET,youtube,STABLE") {
			youtubeIdx = i
		}
	}

	if youtubeIdx == -1 {
		t.Fatalf("youtube rule not found in %v", rules)
	}
	if matchIdx == -1 {
		t.Fatalf("MATCH rule not found in %v", rules)
	}
	if youtubeIdx >= matchIdx {
		t.Fatalf("RULE-SET,youtube was inserted after or at MATCH (youtube=%d, match=%d)", youtubeIdx, matchIdx)
	}

	if err := editor.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	savedBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("Read saved config failed: %v", err)
	}
	savedStr := string(savedBytes)

	if !strings.Contains(savedStr, "Top comment preserving test") {
		t.Errorf("Top comment was lost during Save()!\n%s", savedStr)
	}
	if !strings.Contains(savedStr, "Final fallback") {
		t.Errorf("Fallback comment was lost during Save()!\n%s", savedStr)
	}

	if err := editor.RemoveRule("apple"); err != nil {
		t.Fatalf("RemoveRule failed: %v", err)
	}
	if err := editor.RemoveRuleProvider("apple"); err != nil {
		t.Fatalf("RemoveRuleProvider failed: %v", err)
	}

	if err := editor.Save(); err != nil {
		t.Fatalf("Save after removal failed: %v", err)
	}

	rulesAfter := editor.GetRules()
	for _, r := range rulesAfter {
		if strings.Contains(r, "apple") {
			t.Errorf("apple rule still present: %s", r)
		}
	}
	provsAfter := editor.GetRuleProviders()
	for _, p := range provsAfter {
		if p.Name == "apple" {
			t.Errorf("apple provider still present: %+v", p)
		}
	}

	// Test QUIC rule support
	if err := editor.AddRule("youtube", "UNBLOCK"); err != nil {
		t.Fatalf("AddRule youtube failed: %v", err)
	}
	if editor.HasQuicRule("youtube") {
		t.Errorf("expected no QUIC rule for youtube initially")
	}

	if err := editor.SetQuicRule("youtube", true); err != nil {
		t.Fatalf("SetQuicRule true failed: %v", err)
	}
	if !editor.HasQuicRule("youtube") {
		t.Errorf("expected QUIC rule for youtube after SetQuicRule true")
	}

	rulesWithQuic := editor.GetRules()
	foundQuicBeforeRule := false
	for i, r := range rulesWithQuic {
		if strings.Contains(r, "AND,") && strings.Contains(r, "youtube") && strings.Contains(r, "REJECT") {
			if i+1 < len(rulesWithQuic) && strings.Contains(rulesWithQuic[i+1], "RULE-SET,youtube") {
				foundQuicBeforeRule = true
			}
		}
	}
	if !foundQuicBeforeRule {
		t.Errorf("expected QUIC rule immediately before RULE-SET,youtube, got rules: %v", rulesWithQuic)
	}

	if err := editor.RemoveRule("youtube"); err != nil {
		t.Fatalf("RemoveRule youtube failed: %v", err)
	}
	if editor.HasQuicRule("youtube") {
		t.Errorf("expected QUIC rule for youtube to be removed with RemoveRule")
	}

	// Test Global QUIC rule
	if editor.HasGlobalQuicRule() {
		t.Errorf("expected no global QUIC rule initially")
	}
	if err := editor.SetGlobalQuicRule(true); err != nil {
		t.Fatalf("SetGlobalQuicRule true failed: %v", err)
	}
	if !editor.HasGlobalQuicRule() {
		t.Errorf("expected global QUIC rule after SetGlobalQuicRule true")
	}
	rulesAfterGlobal := editor.GetRules()
	if len(rulesAfterGlobal) == 0 || !strings.Contains(rulesAfterGlobal[0], "AND,((NETWORK,udp),(DST-PORT,443)),REJECT") {
		t.Errorf("expected global QUIC rule at index 0, got: %v", rulesAfterGlobal)
	}
	if err := editor.SetGlobalQuicRule(false); err != nil {
		t.Fatalf("SetGlobalQuicRule false failed: %v", err)
	}
	if editor.HasGlobalQuicRule() {
		t.Errorf("expected no global QUIC rule after SetGlobalQuicRule false")
	}
}

func TestProxyGroupsAndProvidersAndRawRules(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte(sampleConfig), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	editor, err := NewYAMLEditor(cfgPath)
	if err != nil {
		t.Fatalf("NewYAMLEditor failed: %v", err)
	}

	// 1. Test Proxy Groups CRUD
	groups := editor.GetProxyGroupEntries()
	if len(groups) != 2 {
		t.Fatalf("Expected 2 proxy groups, got %d", len(groups))
	}

	err = editor.AddProxyGroup(ProxyGroupEntry{
		Name:      "AUTO-FALLBACK",
		Type:      "fallback",
		Proxies:   []string{"DIRECT", "REJECT"},
		URL:       "https://cp.cloudflare.com/generate_204",
		Interval:  300,
		Tolerance: 50,
	})
	if err != nil {
		t.Fatalf("AddProxyGroup failed: %v", err)
	}

	groupsAfterAdd := editor.GetProxyGroupEntries()
	if len(groupsAfterAdd) != 3 {
		t.Fatalf("Expected 3 proxy groups after add, got %d", len(groupsAfterAdd))
	}

	err = editor.UpdateProxyGroup("AUTO-FALLBACK", ProxyGroupEntry{
		Name:     "AUTO-FALLBACK",
		Type:     "url-test",
		Proxies:  []string{"DIRECT"},
		URL:      "https://cp.cloudflare.com/generate_204",
		Interval: 600,
	})
	if err != nil {
		t.Fatalf("UpdateProxyGroup failed: %v", err)
	}

	groupsAfterUpdate := editor.GetProxyGroupEntries()
	foundUpdated := false
	for _, g := range groupsAfterUpdate {
		if g.Name == "AUTO-FALLBACK" && g.Type == "url-test" && g.Interval == 600 {
			foundUpdated = true
			break
		}
	}
	if !foundUpdated {
		t.Fatalf("Failed to verify updated proxy group")
	}

	err = editor.DeleteProxyGroup("AUTO-FALLBACK")
	if err != nil {
		t.Fatalf("DeleteProxyGroup failed: %v", err)
	}
	if len(editor.GetProxyGroupEntries()) != 2 {
		t.Fatalf("Expected 2 proxy groups after delete")
	}

	// 2. Test Proxy Providers CRUD
	providers := editor.GetProxyProviders()
	if len(providers) != 0 {
		t.Fatalf("Expected 0 proxy providers initially")
	}

	err = editor.AddProxyProvider(ProxyProviderEntry{
		Name:     "sub1",
		Type:     "http",
		URL:      "https://example.com/sub.yaml",
		Path:     "./proxy_providers/sub1.yaml",
		Interval: 3600,
		HealthCheck: &HealthCheckConfig{
			Enable:   true,
			URL:      "http://cp.cloudflare.com/generate_204",
			Interval: 300,
		},
	})
	if err != nil {
		t.Fatalf("AddProxyProvider failed: %v", err)
	}

	providers = editor.GetProxyProviders()
	if len(providers) != 1 || providers[0].Name != "sub1" {
		t.Fatalf("Expected 1 provider 'sub1', got %v", providers)
	}

	err = editor.UpdateProxyProvider("sub1", ProxyProviderEntry{
		Name:     "sub1",
		Type:     "http",
		URL:      "https://example.com/sub_new.yaml",
		Path:     "./proxy_providers/sub1.yaml",
		Interval: 7200,
	})
	if err != nil {
		t.Fatalf("UpdateProxyProvider failed: %v", err)
	}

	providers = editor.GetProxyProviders()
	if len(providers) != 1 || providers[0].Interval != 7200 {
		t.Fatalf("Failed to verify updated proxy provider")
	}

	err = editor.DeleteProxyProvider("sub1")
	if err != nil {
		t.Fatalf("DeleteProxyProvider failed: %v", err)
	}
	if len(editor.GetProxyProviders()) != 0 {
		t.Fatalf("Expected 0 proxy providers after delete")
	}

	// 3. Test Raw Rules CRUD and Move
	rulesBefore := editor.GetRules()
	initialCount := len(rulesBefore)

	err = editor.AddRuleRaw("DOMAIN,test.example.com,DIRECT", 0)
	if err != nil {
		t.Fatalf("AddRuleRaw failed: %v", err)
	}
	rulesAfterAdd := editor.GetRules()
	if len(rulesAfterAdd) != initialCount+1 || rulesAfterAdd[0] != "DOMAIN,test.example.com,DIRECT" {
		t.Fatalf("Expected rule at index 0, got %v", rulesAfterAdd)
	}

	err = editor.UpdateRuleAt(0, "DOMAIN,test2.example.com,REJECT")
	if err != nil {
		t.Fatalf("UpdateRuleAt failed: %v", err)
	}
	if editor.GetRules()[0] != "DOMAIN,test2.example.com,REJECT" {
		t.Fatalf("UpdateRuleAt did not update rule")
	}

	err = editor.MoveRule(0, 1)
	if err != nil {
		t.Fatalf("MoveRule failed: %v", err)
	}
	if editor.GetRules()[1] != "DOMAIN,test2.example.com,REJECT" {
		t.Fatalf("MoveRule did not move rule to index 1")
	}

	err = editor.DeleteRuleAt(1)
	if err != nil {
		t.Fatalf("DeleteRuleAt failed: %v", err)
	}
	if len(editor.GetRules()) != initialCount {
		t.Fatalf("Expected rule count to return to initial")
	}

	// Save and verify persistence
	if err := editor.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
}


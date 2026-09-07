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

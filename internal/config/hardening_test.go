package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuicRecognitionDoesNotMatchPrefixOrCustomRule(t *testing.T) {
	if isQuicRuleForRuleSet("AND,((RULE-SET,youtube_images),(NETWORK,udp),(DST-PORT,443)),REJECT","youtube") { t.Fatal("prefix collision") }
	if isGlobalQuicRule("AND,((NETWORK,udp),(DST-PORT,443),(DOMAIN,example.org)),REJECT") { t.Fatal("custom rule misidentified") }
	if !isQuicRuleForRuleSet("AND,((RULE-SET,youtube),(NETWORK,udp),(DST-PORT,443)),REJECT","youtube") { t.Fatal("managed rule not recognized") }
}
func TestUpdateGroupPreservesUnmanagedFields(t *testing.T) {
	p:=filepath.Join(t.TempDir(),"config.yaml")
	if err:=os.WriteFile(p,[]byte("proxy-groups:\n  - name: VPN\n    type: fallback\n    include-all-proxies: true\n    empty-fallback: REJECT\n    timeout: 5000\n"),0600);err!=nil {t.Fatal(err)}
	e,err:=NewYAMLEditor(p);if err!=nil {t.Fatal(err)}
	if err=e.UpdateProxyGroup("VPN",ProxyGroupEntry{Name:"VPN",Type:"url-test",Tolerance:50});err!=nil {t.Fatal(err)}
	if err=e.Save();err!=nil {t.Fatal(err)}
	b,err:=os.ReadFile(p);if err!=nil {t.Fatal(err)}
	for _,want:=range []string{"include-all-proxies: true","empty-fallback: REJECT","timeout: 5000","type: url-test"} { if !strings.Contains(string(b),want) {t.Fatalf("missing %s",want)} }
}

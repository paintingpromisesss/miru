package mrs

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestDecodeDomainMRS(t *testing.T) {
	// Download small MRS file for testing
	url := "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/google.mrs"
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Skipf("skipping live test due to network: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("skipping live test: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Type != TypeDomain {
		t.Errorf("expected domain type, got %s", decoded.Type)
	}

	if len(decoded.Rules) == 0 {
		t.Errorf("expected non-empty rules, got 0")
	}

	t.Logf("Decoded %d rules from google.mrs (header count %d). First 5 rules:", len(decoded.Rules), decoded.Count)
	for i := 0; i < len(decoded.Rules) && i < 5; i++ {
		t.Logf("  [%d] %s", i, decoded.Rules[i])
	}
}

func TestDecodeIPCIDRMRS(t *testing.T) {
	url := "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geoip/telegram.mrs"
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Skipf("skipping live test due to network: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("skipping live test: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Type != TypeIPCIDR {
		t.Errorf("expected ipcidr type, got %s", decoded.Type)
	}

	if len(decoded.Rules) == 0 {
		t.Errorf("expected non-empty rules, got 0")
	}

	t.Logf("Decoded %d CIDRs from telegram.mrs. First 5 CIDRs:", len(decoded.Rules))
	for i := 0; i < len(decoded.Rules) && i < 5; i++ {
		t.Logf("  [%d] %s", i, decoded.Rules[i])
	}
}

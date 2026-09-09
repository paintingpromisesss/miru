package server

import (
	"net/http"
	"os"
	"sort"
	"strings"
)

type RulesDirEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func (s *Server) handleLocal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	providers := s.editor.GetRuleProviders()
	rules := s.editor.GetRules()
	groups := s.editor.GetProxyGroups()
	activeSetNames := s.editor.GetRuleSetNames()

	var diskFiles []RulesDirEntry
	if entries, err := os.ReadDir(s.rulesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".mrs") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			diskFiles = append(diskFiles, RulesDirEntry{
				Name:    info.Name(),
				Size:    info.Size(),
				ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
		sort.Slice(diskFiles, func(i, j int) bool {
			return diskFiles[i].Name < diskFiles[j].Name
		})
	}

	quicBlocked := make([]string, 0)
	for _, prov := range providers {
		if s.editor.HasQuicRule(prov.Name) {
			quicBlocked = append(quicBlocked, prov.Name)
		}
	}
	for _, name := range activeSetNames {
		if s.editor.HasQuicRule(name) {
			found := false
			for _, q := range quicBlocked {
				if strings.EqualFold(q, name) {
					found = true
					break
				}
			}
			if !found {
				quicBlocked = append(quicBlocked, name)
			}
		}
	}

	resp := map[string]interface{}{
		"rule_providers":      providers,
		"rules":               rules,
		"proxy_groups":        groups,
		"proxy_group_entries": s.editor.GetProxyGroupEntries(),
		"proxy_providers":     s.editor.GetProxyProviders(),
		"discovered_proxies":  s.editor.GetDiscoveredProxies(),
		"applied_rule_sets":   activeSetNames,
		"quic_blocked":        quicBlocked,
		"global_quic_blocked": s.editor.HasGlobalQuicRule(),
		"rules_dir":           s.rulesDir,
		"disk_files":          diskFiles,
		"config_path":         s.configPath,
		"mihomo_status":       s.CheckMihomo(r.Context()),
	}

	writeJSON(w, http.StatusOK, resp)
}

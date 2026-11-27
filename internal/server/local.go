package server

import (
	"net/http"
	"path/filepath"
	"io/fs"
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
	targets := s.routingTargets()
	activeSetNames := s.editor.GetRuleSetNames()

	var diskFiles []RulesDirEntry
	_ = filepath.WalkDir(s.rulesDir,func(p string,d fs.DirEntry,err error)error{
		if err!=nil||d.IsDir(){return nil}
		name:=strings.ToLower(d.Name())
		if !(strings.HasSuffix(name,".mrs")||strings.HasSuffix(name,".txt")||strings.HasSuffix(name,".yaml")){return nil}
		info,err:=d.Info();if err!=nil||!info.Mode().IsRegular(){return nil}
		rel,err:=filepath.Rel(s.rulesDir,p);if err!=nil{return nil}
		diskFiles=append(diskFiles,RulesDirEntry{Name:rel,Size:info.Size(),ModTime:info.ModTime().Format("2006-01-02 15:04:05")});return nil
	})
	sort.Slice(diskFiles,func(i,j int)bool{return diskFiles[i].Name<diskFiles[j].Name})

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
		"routing_targets": targets,
		"mode": s.mode,
		"proxy_group_entries": s.editor.GetProxyGroupEntries(),
		"proxy_providers":     s.editor.GetProxyProviders(),
		"discovered_proxies":  targets,
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

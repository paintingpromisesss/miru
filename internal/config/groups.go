package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProxyGroupEntry struct {
	Name          string   `json:"name" yaml:"name"`
	Type          string   `json:"type" yaml:"type"`
	Proxies       []string `json:"proxies,omitempty" yaml:"proxies,omitempty"`
	Use           []string `json:"use,omitempty" yaml:"use,omitempty"`
	URL           string   `json:"url,omitempty" yaml:"url,omitempty"`
	Interval      int      `json:"interval,omitempty" yaml:"interval,omitempty"`
	Tolerance     int      `json:"tolerance,omitempty" yaml:"tolerance,omitempty"`
	Lazy          *bool    `json:"lazy,omitempty" yaml:"lazy,omitempty"`
	Strategy      string   `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	DisableUDP    *bool    `json:"disable_udp,omitempty" yaml:"disable-udp,omitempty"`
	Filter        string   `json:"filter,omitempty" yaml:"filter,omitempty"`
	ExcludeFilter string   `json:"exclude_filter,omitempty" yaml:"exclude-filter,omitempty"`
}

func (e *YAMLEditor) GetProxyGroupEntries() []ProxyGroupEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("proxy-groups")
	if section == nil || section.Kind != yaml.SequenceNode {
		return nil
	}

	var entries []ProxyGroupEntry
	for _, item := range section.Content {
		if item.Kind == yaml.MappingNode {
			var entry ProxyGroupEntry
			// Decode using yaml.Node directly
			if err := item.Decode(&entry); err == nil && entry.Name != "" {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func (e *YAMLEditor) GetDiscoveredProxies() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var names []string
	seen := make(map[string]bool)

	// 1. Built-in proxies
	appendUniqueProxy(&names, seen, "DIRECT")
	appendUniqueProxy(&names, seen, "REJECT")
	appendUniqueProxy(&names, seen, "GLOBAL")

	// 2. Individual proxies defined in "proxies"
	if pSec := e.readSection("proxies"); pSec != nil && pSec.Kind == yaml.SequenceNode {
		for _, item := range pSec.Content {
			if item.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(item.Content); j += 2 {
					if item.Content[j].Value == "name" && item.Content[j+1].Kind == yaml.ScalarNode {
						appendUniqueProxy(&names, seen, item.Content[j+1].Value)
						break
					}
				}
			}
		}
	}

	// 3. Proxy groups
	if gSec := e.readSection("proxy-groups"); gSec != nil && gSec.Kind == yaml.SequenceNode {
		for _, item := range gSec.Content {
			if item.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(item.Content); j += 2 {
					if item.Content[j].Value == "name" && item.Content[j+1].Kind == yaml.ScalarNode {
						appendUniqueProxy(&names, seen, item.Content[j+1].Value)
						break
					}
				}
			}
		}
	}

	return names
}

func appendUniqueProxy(names *[]string, seen map[string]bool, n string) {
	n = strings.TrimSpace(n)
	if n != "" && !seen[strings.ToLower(n)] {
		seen[strings.ToLower(n)] = true
		*names = append(*names, n)
	}
}

func (e *YAMLEditor) AddProxyGroup(entry ProxyGroupEntry) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if strings.TrimSpace(entry.Name) == "" {
		return fmt.Errorf("proxy group name cannot be empty")
	}
	if strings.TrimSpace(entry.Type) == "" {
		entry.Type = "select"
	}

	section := e.ensureSection("proxy-groups", yaml.SequenceNode, "!!seq")
	if section.Kind != yaml.SequenceNode {
		section.Kind = yaml.SequenceNode
		section.Tag = "!!seq"
	}

	// Check if already exists
	for _, item := range section.Content {
		if item.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(item.Content); j += 2 {
				if item.Content[j].Value == "name" && strings.EqualFold(item.Content[j+1].Value, entry.Name) {
					return fmt.Errorf("proxy group %q already exists", entry.Name)
				}
			}
		}
	}

	// Marshal entry to yaml.Node
	raw, err := yaml.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal proxy group: %w", err)
	}
	var docNode yaml.Node
	if err := yaml.Unmarshal(raw, &docNode); err != nil || len(docNode.Content) == 0 {
		return fmt.Errorf("unmarshal proxy group node: %w", err)
	}

	section.Content = append(section.Content, docNode.Content[0])
	return nil
}

func (e *YAMLEditor) UpdateProxyGroup(name string, entry ProxyGroupEntry) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.readSection("proxy-groups")
	if section == nil || section.Kind != yaml.SequenceNode {
		return fmt.Errorf("proxy-groups section not found")
	}

	idx := -1
	for i, item := range section.Content {
		if item.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(item.Content); j += 2 {
				if item.Content[j].Value == "name" && strings.EqualFold(item.Content[j+1].Value, name) {
					idx = i
					break
				}
			}
		}
		if idx >= 0 {
			break
		}
	}

	if idx < 0 {
		return fmt.Errorf("proxy group %q not found", name)
	}

	raw, err := yaml.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal updated proxy group: %w", err)
	}
	var docNode yaml.Node
	if err := yaml.Unmarshal(raw, &docNode); err != nil || len(docNode.Content) == 0 {
		return fmt.Errorf("unmarshal updated proxy group node: %w", err)
	}

	section.Content[idx] = docNode.Content[0]
	return nil
}

func (e *YAMLEditor) DeleteProxyGroup(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.readSection("proxy-groups")
	if section == nil || section.Kind != yaml.SequenceNode {
		return fmt.Errorf("proxy-groups section not found")
	}

	var filtered []*yaml.Node
	removed := false
	for _, item := range section.Content {
		matched := false
		if item.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(item.Content); j += 2 {
				if item.Content[j].Value == "name" && strings.EqualFold(item.Content[j+1].Value, name) {
					matched = true
					removed = true
					break
				}
			}
		}
		if !matched {
			filtered = append(filtered, item)
		}
	}

	if !removed {
		return fmt.Errorf("proxy group %q not found", name)
	}

	section.Content = filtered
	return nil
}

package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type RuleProviderEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Behavior string `json:"behavior"`
	Format   string `json:"format"`
	URL      string `json:"url"`
	Path     string `json:"path"`
	Interval int    `json:"interval"`
}

type YAMLEditor struct {
	mu   sync.RWMutex
	path string
	node *yaml.Node
}

func NewYAMLEditor(path string) (*YAMLEditor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("parse config yaml %q: %w", path, err)
	}

	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		node = yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{
					Kind: yaml.MappingNode,
					Tag:  "!!map",
				},
			},
		}
	}

	return &YAMLEditor{
		path: path,
		node: &node,
	}, nil
}

func (e *YAMLEditor) Path() string {
	return e.path
}

func (e *YAMLEditor) Save() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(e.node); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	_ = enc.Close()

	dir := filepath.Dir(e.path)
	tmpFile, err := os.CreateTemp(dir, "mihomo-cfg-*.tmp")
	if err != nil {
		if writeErr := os.WriteFile(e.path, buf.Bytes(), 0644); writeErr != nil {
			return fmt.Errorf("write config fallback: %w", writeErr)
		}
		return nil
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(buf.Bytes()); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp config: %w", err)
	}

	if err := os.Rename(tmpName, e.path); err != nil {
		if writeErr := os.WriteFile(e.path, buf.Bytes(), 0644); writeErr != nil {
			os.Remove(tmpName)
			return fmt.Errorf("rename/write config: %w", writeErr)
		}
		os.Remove(tmpName)
	}

	return nil
}

func (e *YAMLEditor) getDocRoot() *yaml.Node {
	if e.node.Kind == yaml.DocumentNode && len(e.node.Content) > 0 {
		return e.node.Content[0]
	}
	return nil
}

func (e *YAMLEditor) readSection(key string) *yaml.Node {
	root := e.getDocRoot()
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}

func (e *YAMLEditor) ensureSection(key string, defaultKind yaml.Kind, defaultTag string) *yaml.Node {
	root := e.getDocRoot()
	if root == nil {
		root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		e.node.Content = []*yaml.Node{root}
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: defaultKind, Tag: defaultTag}
	root.Content = append(root.Content, keyNode, valNode)
	return valNode
}

func (e *YAMLEditor) GetSecret() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	node := e.readSection("secret")
	if node != nil && node.Kind == yaml.ScalarNode {
		return strings.TrimSpace(node.Value)
	}
	return ""
}

// In Mihomo, proxy-groups is a sequence of mappings: [{name: "group1", ...}, ...]
func (e *YAMLEditor) GetProxyGroups() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("proxy-groups")
	var groups []string

	if section != nil {
		if section.Kind == yaml.SequenceNode {
			for _, item := range section.Content {
				if item.Kind == yaml.MappingNode {
					for j := 0; j+1 < len(item.Content); j += 2 {
						if item.Content[j].Value == "name" && item.Content[j+1].Kind == yaml.ScalarNode {
							name := strings.TrimSpace(item.Content[j+1].Value)
							if name != "" {
								groups = append(groups, name)
							}
							break
						}
					}
				}
			}
		} else if section.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(section.Content); i += 2 {
				groups = append(groups, section.Content[i].Value)
			}
		}
	}

	hasDirect, hasReject := false, false
	for _, g := range groups {
		if strings.EqualFold(g, "DIRECT") {
			hasDirect = true
		}
		if strings.EqualFold(g, "REJECT") {
			hasReject = true
		}
	}
	if !hasDirect {
		groups = append(groups, "DIRECT")
	}
	if !hasReject {
		groups = append(groups, "REJECT")
	}

	return groups
}

func (e *YAMLEditor) GetRuleProviders() []RuleProviderEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("rule-providers")
	if section == nil || section.Kind != yaml.MappingNode {
		return nil
	}

	var entries []RuleProviderEntry
	for i := 0; i+1 < len(section.Content); i += 2 {
		key := section.Content[i]
		val := section.Content[i+1]
		entry := RuleProviderEntry{
			Name:     key.Value,
			Type:     "http",
			Behavior: "domain",
			Format:   "mrs",
			Interval: 86400,
		}

		if val.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(val.Content); j += 2 {
				k := val.Content[j].Value
				v := val.Content[j+1].Value
				switch k {
				case "type":
					entry.Type = v
				case "behavior":
					entry.Behavior = v
				case "format":
					entry.Format = v
				case "url":
					entry.URL = v
				case "path":
					entry.Path = v
				case "interval":
					if iv, err := strconv.Atoi(v); err == nil {
						entry.Interval = iv
					}
				}
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func (e *YAMLEditor) GetRuleProviderPath(name string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("rule-providers")
	if section == nil || section.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(section.Content); i += 2 {
		if section.Content[i].Value == name {
			val := section.Content[i+1]
			if val.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(val.Content); j += 2 {
					if val.Content[j].Value == "path" {
						return val.Content[j+1].Value
					}
				}
			}
		}
	}
	return ""
}

func (e *YAMLEditor) AddRuleProvider(entry RuleProviderEntry) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.ensureSection("rule-providers", yaml.MappingNode, "!!map")

	for i := 0; i+1 < len(section.Content); i += 2 {
		if section.Content[i].Value == entry.Name {
			return fmt.Errorf("rule-provider %q already exists", entry.Name)
		}
	}

	if entry.Type == "" {
		entry.Type = "http"
	}
	if entry.Behavior == "" {
		entry.Behavior = "domain"
	}
	if entry.Format == "" {
		entry.Format = "mrs"
	}
	if entry.Path == "" {
		entry.Path = fmt.Sprintf("rules/%s.mrs", entry.Name)
	}
	if entry.Interval <= 0 {
		entry.Interval = 86400
	}

	nameNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Name}
	valNode := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}

	valNode.Content = append(valNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "type"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Type},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "behavior"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Behavior},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "format"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Format},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "interval"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(entry.Interval)},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "url"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.URL},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "path"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Path},
	)

	section.Content = append(section.Content, nameNode, valNode)
	return nil
}

func (e *YAMLEditor) RemoveRuleProvider(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.readSection("rule-providers")
	if section == nil || section.Kind != yaml.MappingNode {
		return fmt.Errorf("rule-providers section not found")
	}

	for i := 0; i+1 < len(section.Content); i += 2 {
		if section.Content[i].Value == name {
			section.Content = append(section.Content[:i], section.Content[i+2:]...)
			return nil
		}
	}
	return fmt.Errorf("rule-provider %q not found", name)
}

func (e *YAMLEditor) AddRule(ruleSet, proxyGroup string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.ensureSection("rules", yaml.SequenceNode, "!!seq")
	if section.Kind != yaml.SequenceNode {
		section.Kind = yaml.SequenceNode
		section.Tag = "!!seq"
	}

	newRuleStr := fmt.Sprintf("RULE-SET,%s,%s", ruleSet, proxyGroup)

	for _, n := range section.Content {
		if n.Kind == yaml.ScalarNode {
			parts := strings.Split(n.Value, ",")
			if len(parts) >= 3 && strings.EqualFold(strings.TrimSpace(parts[0]), "RULE-SET") && strings.TrimSpace(parts[1]) == ruleSet {
				parts[2] = proxyGroup
				n.Value = strings.Join(parts, ",")
				return nil
			}
		}
	}

	// Must be inserted before the final MATCH fallback rule
	insertIdx := len(section.Content)
	for i, n := range section.Content {
		if n.Kind == yaml.ScalarNode {
			ruleParts := strings.Split(n.Value, ",")
			if len(ruleParts) > 0 && strings.EqualFold(strings.TrimSpace(ruleParts[0]), "MATCH") {
				insertIdx = i
				break
			}
		}
	}

	ruleNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: newRuleStr,
	}

	before := append([]*yaml.Node{}, section.Content[:insertIdx]...)
	after := append([]*yaml.Node{}, section.Content[insertIdx:]...)
	section.Content = append(before, append([]*yaml.Node{ruleNode}, after...)...)

	return nil
}

func (e *YAMLEditor) RemoveRule(ruleSet string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.readSection("rules")
	if section == nil || section.Kind != yaml.SequenceNode {
		return fmt.Errorf("rules section not found")
	}

	var filtered []*yaml.Node
	removed := false

	for _, n := range section.Content {
		if n.Kind == yaml.ScalarNode {
			parts := strings.Split(n.Value, ",")
			if len(parts) >= 2 &&
				strings.EqualFold(strings.TrimSpace(parts[0]), "RULE-SET") &&
				strings.TrimSpace(parts[1]) == ruleSet {
				removed = true
				continue
			}
		}
		filtered = append(filtered, n)
	}

	if !removed {
		return fmt.Errorf("rule for rule-set %q not found", ruleSet)
	}

	section.Content = filtered
	return nil
}

func (e *YAMLEditor) GetRules() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("rules")
	if section == nil || section.Kind != yaml.SequenceNode {
		return nil
	}
	var rules []string
	for _, n := range section.Content {
		if n.Kind == yaml.ScalarNode {
			rules = append(rules, n.Value)
		}
	}
	return rules
}

func (e *YAMLEditor) GetRuleSetNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("rules")
	if section == nil || section.Kind != yaml.SequenceNode {
		return nil
	}
	var names []string
	seen := make(map[string]bool)

	for _, n := range section.Content {
		if n.Kind == yaml.ScalarNode {
			parts := strings.Split(n.Value, ",")
			if len(parts) >= 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "RULE-SET") {
				name := strings.TrimSpace(parts[1])
				if name != "" && !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}
	return names
}

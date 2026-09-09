package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type HealthCheckConfig struct {
	Enable         bool   `json:"enable" yaml:"enable"`
	URL            string `json:"url,omitempty" yaml:"url,omitempty"`
	Interval       int    `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout        int    `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Lazy           *bool  `json:"lazy,omitempty" yaml:"lazy,omitempty"`
	ExpectedStatus string `json:"expected_status,omitempty" yaml:"expected-status,omitempty"`
}

type ProxyProviderEntry struct {
	Name          string             `json:"name" yaml:"-"`
	Type          string             `json:"type" yaml:"type"` // http or file
	URL           string             `json:"url,omitempty" yaml:"url,omitempty"`
	Path          string             `json:"path" yaml:"path"`
	Interval      int                `json:"interval,omitempty" yaml:"interval,omitempty"`
	Filter        string             `json:"filter,omitempty" yaml:"filter,omitempty"`
	ExcludeFilter string             `json:"exclude_filter,omitempty" yaml:"exclude-filter,omitempty"`
	ExcludeType   string             `json:"exclude_type,omitempty" yaml:"exclude-type,omitempty"`
	Proxy         string             `json:"proxy,omitempty" yaml:"proxy,omitempty"`
	HealthCheck   *HealthCheckConfig `json:"health_check,omitempty" yaml:"health-check,omitempty"`
}

func (e *YAMLEditor) GetProxyProviders() []ProxyProviderEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("proxy-providers")
	if section == nil || section.Kind != yaml.MappingNode {
		return nil
	}

	var entries []ProxyProviderEntry
	for i := 0; i+1 < len(section.Content); i += 2 {
		keyNode := section.Content[i]
		valNode := section.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode && valNode.Kind == yaml.MappingNode {
			var entry ProxyProviderEntry
			if err := valNode.Decode(&entry); err == nil {
				entry.Name = keyNode.Value
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func (e *YAMLEditor) AddProxyProvider(entry ProxyProviderEntry) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	entry.Name = strings.TrimSpace(entry.Name)
	if entry.Name == "" {
		return fmt.Errorf("proxy provider name cannot be empty")
	}
	if entry.Type == "" {
		entry.Type = "http"
	}
	if entry.Path == "" {
		entry.Path = fmt.Sprintf("./proxy_providers/%s.yaml", entry.Name)
	}
	if entry.Type == "http" && entry.Interval <= 0 {
		entry.Interval = 86400
	}

	section := e.ensureSection("proxy-providers", yaml.MappingNode, "!!map")
	if section.Kind != yaml.MappingNode {
		section.Kind = yaml.MappingNode
		section.Tag = "!!map"
	}

	for i := 0; i+1 < len(section.Content); i += 2 {
		if strings.EqualFold(section.Content[i].Value, entry.Name) {
			return fmt.Errorf("proxy-provider %q already exists", entry.Name)
		}
	}

	raw, err := yaml.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal proxy provider: %w", err)
	}
	var docNode yaml.Node
	if err := yaml.Unmarshal(raw, &docNode); err != nil || len(docNode.Content) == 0 {
		return fmt.Errorf("unmarshal proxy provider node: %w", err)
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Name}
	section.Content = append(section.Content, keyNode, docNode.Content[0])
	return nil
}

func (e *YAMLEditor) UpdateProxyProvider(name string, entry ProxyProviderEntry) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.readSection("proxy-providers")
	if section == nil || section.Kind != yaml.MappingNode {
		return fmt.Errorf("proxy-providers section not found")
	}

	idx := -1
	for i := 0; i+1 < len(section.Content); i += 2 {
		if strings.EqualFold(section.Content[i].Value, name) {
			idx = i
			break
		}
	}

	if idx < 0 {
		return fmt.Errorf("proxy-provider %q not found", name)
	}

	raw, err := yaml.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal updated proxy provider: %w", err)
	}
	var docNode yaml.Node
	if err := yaml.Unmarshal(raw, &docNode); err != nil || len(docNode.Content) == 0 {
		return fmt.Errorf("unmarshal updated proxy provider node: %w", err)
	}

	// Update key if name changed
	if entry.Name != "" && !strings.EqualFold(entry.Name, name) {
		section.Content[idx].Value = entry.Name
	}
	section.Content[idx+1] = docNode.Content[0]
	return nil
}

func (e *YAMLEditor) DeleteProxyProvider(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.readSection("proxy-providers")
	if section == nil || section.Kind != yaml.MappingNode {
		return fmt.Errorf("proxy-providers section not found")
	}

	var filtered []*yaml.Node
	removed := false
	for i := 0; i+1 < len(section.Content); i += 2 {
		if strings.EqualFold(section.Content[i].Value, name) {
			removed = true
			continue
		}
		filtered = append(filtered, section.Content[i], section.Content[i+1])
	}

	if !removed {
		return fmt.Errorf("proxy-provider %q not found", name)
	}

	section.Content = filtered
	return nil
}

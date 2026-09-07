package config

import (
	"fmt"
	"strconv"

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

func (e *YAMLEditor) UpdateRuleProvider(name, url, behavior, format string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.readSection("rule-providers")
	if section == nil || section.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(section.Content); i += 2 {
		if section.Content[i].Value == name {
			val := section.Content[i+1]
			if val.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(val.Content); j += 2 {
					k := val.Content[j].Value
					switch k {
					case "url":
						if url != "" {
							val.Content[j+1].Value = url
						}
					case "behavior":
						if behavior != "" {
							val.Content[j+1].Value = behavior
						}
					case "format":
						if format != "" {
							val.Content[j+1].Value = format
						}
					}
				}
			}
			return nil
		}
	}
	return nil
}

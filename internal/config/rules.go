package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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

	var finalFiltered []*yaml.Node
	for _, n := range filtered {
		if n.Kind == yaml.ScalarNode && isQuicRuleForRuleSet(n.Value, ruleSet) {
			continue
		}
		finalFiltered = append(finalFiltered, n)
	}

	section.Content = finalFiltered
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

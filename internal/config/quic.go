package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func isQuicRuleForRuleSet(ruleStr, ruleSet string) bool {
	r := strings.ToLower(ruleStr)
	name := strings.ToLower(ruleSet)
	return strings.Contains(r, "and,") &&
		(strings.Contains(r, "rule-set,"+name) || strings.Contains(r, "rule-set, "+name)) &&
		strings.Contains(r, "443") &&
		strings.Contains(r, "udp") &&
		strings.Contains(r, "reject")
}

func (e *YAMLEditor) HasQuicRule(ruleSet string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("rules")
	if section == nil || section.Kind != yaml.SequenceNode {
		return false
	}
	for _, n := range section.Content {
		if n.Kind == yaml.ScalarNode && isQuicRuleForRuleSet(n.Value, ruleSet) {
			return true
		}
	}
	return false
}

func (e *YAMLEditor) SetQuicRule(ruleSet string, enable bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.ensureSection("rules", yaml.SequenceNode, "!!seq")
	if section.Kind != yaml.SequenceNode {
		section.Kind = yaml.SequenceNode
		section.Tag = "!!seq"
	}

	quicRuleStr := fmt.Sprintf("AND,((RULE-SET,%s),(NETWORK,udp),(DST-PORT,443)),REJECT", ruleSet)

	var filtered []*yaml.Node
	ruleSetIdx := -1
	for _, n := range section.Content {
		if n.Kind == yaml.ScalarNode {
			if isQuicRuleForRuleSet(n.Value, ruleSet) {
				continue
			}
			parts := strings.Split(n.Value, ",")
			if len(parts) >= 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "RULE-SET") && strings.EqualFold(strings.TrimSpace(parts[1]), ruleSet) {
				ruleSetIdx = len(filtered)
			}
		}
		filtered = append(filtered, n)
	}

	if !enable {
		section.Content = filtered
		return nil
	}

	quicNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: quicRuleStr,
	}

	if ruleSetIdx >= 0 {
		before := append([]*yaml.Node{}, filtered[:ruleSetIdx]...)
		after := append([]*yaml.Node{}, filtered[ruleSetIdx:]...)
		section.Content = append(before, append([]*yaml.Node{quicNode}, after...)...)
	} else {
		insertIdx := len(filtered)
		for i, n := range filtered {
			if n.Kind == yaml.ScalarNode {
				parts := strings.Split(n.Value, ",")
				if len(parts) > 0 && strings.EqualFold(strings.TrimSpace(parts[0]), "MATCH") {
					insertIdx = i
					break
				}
			}
		}
		before := append([]*yaml.Node{}, filtered[:insertIdx]...)
		after := append([]*yaml.Node{}, filtered[insertIdx:]...)
		section.Content = append(before, append([]*yaml.Node{quicNode}, after...)...)
	}

	return nil
}

func isGlobalQuicRule(ruleStr string) bool {
	r := strings.ToLower(ruleStr)
	return strings.Contains(r, "and,") &&
		!strings.Contains(r, "rule-set") &&
		strings.Contains(r, "443") &&
		strings.Contains(r, "udp") &&
		strings.Contains(r, "reject")
}

func (e *YAMLEditor) HasGlobalQuicRule() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	section := e.readSection("rules")
	if section == nil || section.Kind != yaml.SequenceNode {
		return false
	}
	for _, n := range section.Content {
		if n.Kind == yaml.ScalarNode && isGlobalQuicRule(n.Value) {
			return true
		}
	}
	return false
}

func (e *YAMLEditor) SetGlobalQuicRule(enable bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	section := e.ensureSection("rules", yaml.SequenceNode, "!!seq")
	if section.Kind != yaml.SequenceNode {
		section.Kind = yaml.SequenceNode
		section.Tag = "!!seq"
	}

	var filtered []*yaml.Node
	for _, n := range section.Content {
		if n.Kind == yaml.ScalarNode && isGlobalQuicRule(n.Value) {
			continue
		}
		filtered = append(filtered, n)
	}

	if !enable {
		section.Content = filtered
		return nil
	}

	quicNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: "AND,((NETWORK,udp),(DST-PORT,443)),REJECT",
	}

	section.Content = append([]*yaml.Node{quicNode}, filtered...)
	return nil
}

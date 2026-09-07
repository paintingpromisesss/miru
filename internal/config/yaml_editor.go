package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

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

func (e *YAMLEditor) GetInboundProxy() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if node := e.readSection("mixed-port"); node != nil && node.Kind == yaml.ScalarNode {
		val := strings.TrimSpace(node.Value)
		if val != "" && val != "0" {
			return fmt.Sprintf("http://127.0.0.1:%s", val)
		}
	}

	if node := e.readSection("port"); node != nil && node.Kind == yaml.ScalarNode {
		val := strings.TrimSpace(node.Value)
		if val != "" && val != "0" {
			return fmt.Sprintf("http://127.0.0.1:%s", val)
		}
	}

	if node := e.readSection("socks-port"); node != nil && node.Kind == yaml.ScalarNode {
		val := strings.TrimSpace(node.Value)
		if val != "" && val != "0" {
			return fmt.Sprintf("socks5://127.0.0.1:%s", val)
		}
	}

	return ""
}

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

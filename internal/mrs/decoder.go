package mrs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
	"gopkg.in/yaml.v3"
)

var (
	MagicBytes         = [4]byte{'M', 'R', 'S', 1} // MRSv1
	ErrInvalidMagic    = errors.New("invalid MRS magic bytes")
	ErrInvalidBehavior = errors.New("unsupported or invalid behavior")
	ErrInvalidVersion  = errors.New("invalid ruleset version")
)

type RuleType string

const (
	TypeDomain    RuleType = "domain"
	TypeIPCIDR    RuleType = "ipcidr"
	TypeClassical RuleType = "classical"
)

type DecodedRuleSet struct {
	Type  RuleType `json:"type"`
	Count int      `json:"count"`
	Rules []string `json:"rules"`
}

// DecodeAny attempts to decode MRS binary first, and falls back to YAML or plain text.
func DecodeAny(data []byte) (*DecodedRuleSet, error) {
	if len(data) >= 4 && data[0] == 0x28 && data[1] == 0xb5 && data[2] == 0x2f && data[3] == 0xfd {
		if res, err := Decode(data); err == nil {
			return res, nil
		}
	} else if len(data) >= 4 && bytes.Equal(data[:4], MagicBytes[:]) {
		reader := bytes.NewReader(data)
		var magic [4]byte
		io.ReadFull(reader, magic[:])
		var behaviorByte [1]byte
		io.ReadFull(reader, behaviorByte[:])
		var count int64
		binary.Read(reader, binary.BigEndian, &count)
		var extraLen int64
		binary.Read(reader, binary.BigEndian, &extraLen)
		if extraLen > 0 {
			io.CopyN(io.Discard, reader, extraLen)
		}
		if behaviorByte[0] == 0 {
			if doms, err := readDomainSet(reader); err == nil {
				return &DecodedRuleSet{Type: TypeDomain, Count: int(count), Rules: doms}, nil
			}
		} else if behaviorByte[0] == 1 {
			if cidrs, err := readIPCIDRSet(reader); err == nil {
				return &DecodedRuleSet{Type: TypeIPCIDR, Count: int(count), Rules: cidrs}, nil
			}
		}
	}

	type yamlFile struct {
		Payload []string `yaml:"payload"`
	}
	var yf yamlFile
	if err := yaml.Unmarshal(data, &yf); err == nil && len(yf.Payload) > 0 {
		return &DecodedRuleSet{
			Type:  TypeClassical,
			Count: len(yf.Payload),
			Rules: yf.Payload,
		}, nil
	}

	var lines []string
	rawLines := bytes.Split(data, []byte("\n"))
	for _, l := range rawLines {
		trimmed := string(bytes.TrimSpace(l))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		lines = append(lines, trimmed)
	}

	return &DecodedRuleSet{
		Type:  TypeClassical,
		Count: len(lines),
		Rules: lines,
	}, nil
}

// Decode parses an MRS binary stream (zstd compressed) and returns the rules list.
func Decode(data []byte) (*DecodedRuleSet, error) {
	reader, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zstd init: %w", err)
	}
	defer reader.Close()

	var magic [4]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}
	if magic != MagicBytes {
		return nil, ErrInvalidMagic
	}

	var behaviorByte [1]byte
	if _, err := io.ReadFull(reader, behaviorByte[:]); err != nil {
		return nil, fmt.Errorf("read behavior: %w", err)
	}

	var count int64
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil {
		return nil, fmt.Errorf("read count: %w", err)
	}

	// Extra payload (reserved in MRS spec)
	var extraLen int64
	if err := binary.Read(reader, binary.BigEndian, &extraLen); err != nil {
		return nil, fmt.Errorf("read extra length: %w", err)
	}
	if extraLen > 0 {
		extraBuf := make([]byte, extraLen)
		if _, err := io.ReadFull(reader, extraBuf); err != nil {
			return nil, fmt.Errorf("read extra: %w", err)
		}
	}

	res := &DecodedRuleSet{
		Count: int(count),
	}

	switch behaviorByte[0] {
	case 0:
		res.Type = TypeDomain
		domains, err := readDomainSet(reader)
		if err != nil {
			return nil, fmt.Errorf("decode domain set: %w", err)
		}
		res.Rules = domains

	case 1:
		res.Type = TypeIPCIDR
		cidrs, err := readIPCIDRSet(reader)
		if err != nil {
			return nil, fmt.Errorf("decode ipcidr set: %w", err)
		}
		res.Rules = cidrs

	default:
		return nil, fmt.Errorf("%w: %d", ErrInvalidBehavior, behaviorByte[0])
	}

	return res, nil
}

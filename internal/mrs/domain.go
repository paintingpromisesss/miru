package mrs

import (
	"encoding/binary"
	"errors"
	"io"
	"slices"

	"github.com/openacid/low/bitmap"
)

type domainSet struct {
	leaves      []uint64
	labelBitmap []uint64
	labels      []byte
	ranks       []int32
	selects     []int32
}

func readDomainSet(r io.Reader) ([]string, error) {
	var version [1]byte
	if _, err := io.ReadFull(r, version[:]); err != nil {
		return nil, err
	}
	if version[0] != 1 {
		return nil, ErrInvalidVersion
	}

	ds := &domainSet{}
	var length int64

	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length < 1 {
		return nil, errors.New("invalid leaves length")
	}
	ds.leaves = make([]uint64, length)
	for i := int64(0); i < length; i++ {
		if err := binary.Read(r, binary.BigEndian, &ds.leaves[i]); err != nil {
			return nil, err
		}
	}

	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length < 1 {
		return nil, errors.New("invalid labelBitmap length")
	}
	ds.labelBitmap = make([]uint64, length)
	for i := int64(0); i < length; i++ {
		if err := binary.Read(r, binary.BigEndian, &ds.labelBitmap[i]); err != nil {
			return nil, err
		}
	}

	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length < 1 {
		return nil, errors.New("invalid labels length")
	}
	ds.labels = make([]byte, length)
	if _, err := io.ReadFull(r, ds.labels); err != nil {
		return nil, err
	}

	ds.selects, ds.ranks = bitmap.IndexSelect32R64(ds.labelBitmap)

	var rawKeys []string
	ds.traverse(0, 0, nil, &rawKeys)
	slices.Sort(rawKeys)

	// Remove trie internal markers (+.xxx)
	finalRules := make([]string, 0, len(rawKeys))
	for _, key := range rawKeys {
		if _, ok := slices.BinarySearch(rawKeys, "+."+key); ok {
			continue
		}
		finalRules = append(finalRules, key)
	}

	return finalRules, nil
}

func (ds *domainSet) traverse(nodeID, bmIdx int, currentKey []byte, rawKeys *[]string) bool {
	if getBit(ds.leaves, nodeID) != 0 {
		*rawKeys = append(*rawKeys, reverseRunes(string(currentKey)))
	}

	for ; ; bmIdx++ {
		if getBit(ds.labelBitmap, bmIdx) != 0 {
			return true
		}
		nextLabel := ds.labels[bmIdx-nodeID]
		currentKey = append(currentKey, nextLabel)
		nextNodeID := countZeros(ds.labelBitmap, ds.ranks, bmIdx+1)
		nextBmIdx := selectIthOne(ds.labelBitmap, ds.ranks, ds.selects, nextNodeID-1) + 1
		if !ds.traverse(nextNodeID, nextBmIdx, currentKey, rawKeys) {
			return false
		}
		currentKey = currentKey[:len(currentKey)-1]
	}
}

func getBit(bm []uint64, i int) uint64 {
	return bm[i>>6] & (1 << uint(i&63))
}

func countZeros(bm []uint64, ranks []int32, i int) int {
	a, _ := bitmap.Rank64(bm, ranks, int32(i))
	return i - int(a)
}

func selectIthOne(bm []uint64, ranks, selects []int32, i int) int {
	a, _ := bitmap.Select32R64(bm, selects, ranks, int32(i))
	return int(a)
}

func reverseRunes(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

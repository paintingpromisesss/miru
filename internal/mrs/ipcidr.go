package mrs

import (
	"encoding/binary"
	"errors"
	"io"
	"net/netip"

	"go4.org/netipx"
)

func readIPCIDRSet(r io.Reader) ([]string, error) {
	var version [1]byte
	if _, err := io.ReadFull(r, version[:]); err != nil {
		return nil, err
	}
	if version[0] != 1 {
		return nil, ErrInvalidVersion
	}

	var length int64
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, errors.New("invalid ipcidr length")
	}

	var cidrs []string
	for i := int64(0); i < length; i++ {
		var a16From, a16To [16]byte
		if err := binary.Read(r, binary.BigEndian, &a16From); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.BigEndian, &a16To); err != nil {
			return nil, err
		}

		from := netip.AddrFrom16(a16From).Unmap()
		to := netip.AddrFrom16(a16To).Unmap()

		ipRange := netipx.IPRangeFrom(from, to)
		for _, prefix := range ipRange.Prefixes() {
			cidrs = append(cidrs, prefix.String())
		}
	}

	return cidrs, nil
}

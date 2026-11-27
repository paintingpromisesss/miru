package mrs

import (
	"bytes"
	"encoding/binary"
	"testing"
)
func TestInvalidBinaryDoesNotFallBackToText(t *testing.T) {
	if _,err:=DecodeAny([]byte{'M','R','S',1});err==nil { t.Fatal("truncated binary accepted") }
}
func TestOversizedHeader(t *testing.T) {
	var b bytes.Buffer
	b.Write(MagicBytes[:])
	b.WriteByte(0)
	binary.Write(&b,binary.BigEndian,int64(maxPreviewRules+1))
	if _,err:=DecodeAny(b.Bytes());err==nil { t.Fatal("oversized header accepted") }
}

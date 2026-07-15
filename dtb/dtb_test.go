package dtb_test

import (
	"encoding/binary"
	"testing"

	"github.com/bobuhiro11/gokvm/dtb"
)

func TestBuilderProducesValidFDTHeader(t *testing.T) {
	t.Parallel()

	b := dtb.NewBuilder()
	b.AddPropString("/", "compatible", "linux,dummy-virt")
	b.AddPropU32("/", "#address-cells", 2)
	b.AddPropU64Array("/memory", "reg", []uint64{0, 1 << 30})
	b.AddPropString("/chosen", "bootargs", "console=ttyAMA0")
	b.AddPropEmpty("/intc", "interrupt-controller")

	blob := b.Bytes()

	if len(blob) < 40 {
		t.Fatalf("blob too short: %d bytes", len(blob))
	}

	const (
		fdtMagic   = 0xd00dfeed
		headerSize = 40
	)

	magic := binary.BigEndian.Uint32(blob[0:4])
	if magic != fdtMagic {
		t.Errorf("magic: got %#x, want %#x", magic, fdtMagic)
	}

	totalSize := binary.BigEndian.Uint32(blob[4:8])
	if int(totalSize) != len(blob) {
		t.Errorf("totalsize: got %d, want %d (actual blob length)", totalSize, len(blob))
	}

	offDTStruct := binary.BigEndian.Uint32(blob[8:12])
	offDTStrings := binary.BigEndian.Uint32(blob[12:16])
	offMemRsvMap := binary.BigEndian.Uint32(blob[16:20])
	version := binary.BigEndian.Uint32(blob[20:24])
	sizeDTStrings := binary.BigEndian.Uint32(blob[32:36])
	sizeDTStruct := binary.BigEndian.Uint32(blob[36:40])

	if version != 17 {
		t.Errorf("version: got %d, want 17", version)
	}

	if offMemRsvMap != headerSize {
		t.Errorf("off_mem_rsvmap: got %d, want %d", offMemRsvMap, headerSize)
	}

	if offDTStruct != offMemRsvMap+16 {
		t.Errorf("off_dt_struct: got %d, want %d", offDTStruct, offMemRsvMap+16)
	}

	if offDTStrings != offDTStruct+sizeDTStruct {
		t.Errorf("off_dt_strings: got %d, want %d", offDTStrings, offDTStruct+sizeDTStruct)
	}

	if int(offDTStrings+sizeDTStrings) != len(blob) {
		t.Errorf("strings block end: got %d, want %d (blob length)", offDTStrings+sizeDTStrings, len(blob))
	}

	// The structure block must begin with FDT_BEGIN_NODE (1) and end
	// with FDT_END (9).
	const (
		tokenBeginNode = 1
		tokenEnd       = 9
	)

	firstToken := binary.BigEndian.Uint32(blob[offDTStruct:])
	if firstToken != tokenBeginNode {
		t.Errorf("first struct token: got %d, want FDT_BEGIN_NODE(%d)", firstToken, tokenBeginNode)
	}

	lastToken := binary.BigEndian.Uint32(blob[offDTStruct+sizeDTStruct-4:])
	if lastToken != tokenEnd {
		t.Errorf("last struct token: got %d, want FDT_END(%d)", lastToken, tokenEnd)
	}

	// The strings block should contain each property name, NUL
	// terminated.
	strings := string(blob[offDTStrings : offDTStrings+sizeDTStrings])
	for _, want := range []string{"compatible", "#address-cells", "reg", "bootargs", "interrupt-controller"} {
		if !contains(strings, want+"\x00") {
			t.Errorf("strings block missing %q", want)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}

	return false
}

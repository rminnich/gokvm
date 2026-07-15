// Package dtb builds a minimal Flattened Devicetree (FDT/DTB) blob
// sufficient to boot a Linux arm64 guest kernel under KVM.
//
// The FDT binary format (header layout, structure-block tokens, and
// string-table encoding) is a public, versioned specification (the
// "Devicetree Specification", https://www.devicetree.org/specifications/);
// the layout implemented here follows version 0.4 (header version 17).
// The specific node/property conventions used (compatible strings for
// PSCI, the ARM architected timer, and the GICv2 interrupt controller)
// are the standard devicetree bindings documented under
// Documentation/devicetree/bindings/ in the Linux kernel sources, and
// are the same conventions used by, e.g., QEMU's "virt" machine.
package dtb

import (
	"bytes"
	"encoding/binary"
)

const (
	magic             = 0xd00dfeed
	fdtVersion        = 17
	fdtLastCompVerion = 16

	tokenBeginNode = 0x00000001
	tokenEndNode   = 0x00000002
	tokenProp      = 0x00000003
	tokenEnd       = 0x00000009

	// Sizes, in bytes, of the cell types AddPropU32/U32Array/U64Array
	// encode, and of a memory-reservation-block entry (two u64s).
	uint32Size    = 4
	uint64Size    = 8
	memRsvMapSize = 2 * uint64Size
)

// node is an in-progress devicetree node: a name, a set of properties
// (in insertion order), and child nodes (in insertion order).
type node struct {
	name     string
	propKeys []string
	propVals [][]byte
	children []*node
}

// Builder builds a single devicetree, rooted at "/".
type Builder struct {
	root    node
	strings []string // property names, in first-use order
	strOff  map[string]uint32
}

// NewBuilder returns a Builder for a new devicetree.
func NewBuilder() *Builder {
	return &Builder{strOff: map[string]uint32{}}
}

// AddProp adds a property with a raw byte-string value to the node at
// path (e.g. "/", "/cpus", "/cpus/cpu@0"). The node (and any missing
// parents) is created if it does not already exist.
func (b *Builder) AddProp(path, name string, value []byte) {
	n := b.node(path)
	n.propKeys = append(n.propKeys, name)
	n.propVals = append(n.propVals, value)
}

// AddPropU32 adds a single big-endian uint32 (a devicetree "cell")
// property.
func (b *Builder) AddPropU32(path, name string, value uint32) {
	buf := make([]byte, uint32Size)
	binary.BigEndian.PutUint32(buf, value)
	b.AddProp(path, name, buf)
}

// AddPropU32Array adds a property whose value is an array of
// big-endian uint32 cells.
func (b *Builder) AddPropU32Array(path, name string, values []uint32) {
	buf := make([]byte, uint32Size*len(values))
	for i, v := range values {
		binary.BigEndian.PutUint32(buf[i*uint32Size:], v)
	}

	b.AddProp(path, name, buf)
}

// AddPropU64Array adds a property whose value is an array of
// big-endian uint64 cells (used for e.g. "reg" with #address-cells=2
// #size-cells=2).
func (b *Builder) AddPropU64Array(path, name string, values []uint64) {
	buf := make([]byte, uint64Size*len(values))
	for i, v := range values {
		binary.BigEndian.PutUint64(buf[i*uint64Size:], v)
	}

	b.AddProp(path, name, buf)
}

// AddPropString adds a property whose value is a single
// NUL-terminated string.
func (b *Builder) AddPropString(path, name, value string) {
	b.AddProp(path, name, append([]byte(value), 0))
}

// AddPropStrings adds a property whose value is a list of
// NUL-terminated strings concatenated together.
func (b *Builder) AddPropStrings(path, name string, values []string) {
	size := 0
	for _, v := range values {
		size += len(v) + 1
	}

	buf := make([]byte, 0, size)
	for _, v := range values {
		buf = append(buf, v...)
		buf = append(buf, 0)
	}

	b.AddProp(path, name, buf)
}

// AddPropEmpty adds a valueless (boolean) property, such as
// "interrupt-controller".
func (b *Builder) AddPropEmpty(path, name string) {
	b.AddProp(path, name, nil)
}

// node returns the node at path, splitting on "/" and creating any
// node (including intermediate ones) that doesn't already exist.
func (b *Builder) node(path string) *node {
	cur := &b.root

	if path == "" || path == "/" {
		return cur
	}

	for _, part := range splitPath(path) {
		var next *node

		for _, c := range cur.children {
			if c.name == part {
				next = c

				break
			}
		}

		if next == nil {
			next = &node{name: part}
			cur.children = append(cur.children, next)
		}

		cur = next
	}

	return cur
}

func splitPath(path string) []string {
	var parts []string

	start := 0

	for i := range len(path) {
		if path[i] == '/' {
			if i > start {
				parts = append(parts, path[start:i])
			}

			start = i + 1
		}
	}

	if start < len(path) {
		parts = append(parts, path[start:])
	}

	return parts
}

func pad4(buf *bytes.Buffer) {
	for buf.Len()%4 != 0 {
		buf.WriteByte(0)
	}
}

func writeU32(buf *bytes.Buffer, v uint32) {
	var tmp [uint32Size]byte

	binary.BigEndian.PutUint32(tmp[:], v)
	buf.Write(tmp[:])
}

// u32 converts a non-negative int (a length or offset) to uint32. All
// such values in this package are devicetree blob sizes/offsets, which
// are always well under the 4 GiB range representable in uint32.
func u32(n int) uint32 {
	return uint32(n) //nolint:gosec // bounded by realistic devicetree blob sizes
}

// strOffset returns the offset of name within the (eventual) strings
// block, adding it if not already present.
func (b *Builder) strOffset(name string) uint32 {
	if off, ok := b.strOff[name]; ok {
		return off
	}

	off := uint32(0)
	for _, s := range b.strings {
		off += u32(len(s)) + 1
	}

	b.strOff[name] = off
	b.strings = append(b.strings, name)

	return off
}

func (b *Builder) writeNode(buf *bytes.Buffer, n *node) {
	writeU32(buf, tokenBeginNode)
	buf.WriteString(n.name)
	buf.WriteByte(0)
	pad4(buf)

	for i, key := range n.propKeys {
		val := n.propVals[i]

		writeU32(buf, tokenProp)
		writeU32(buf, u32(len(val)))
		writeU32(buf, b.strOffset(key))
		buf.Write(val)
		pad4(buf)
	}

	for _, c := range n.children {
		b.writeNode(buf, c)
	}

	writeU32(buf, tokenEndNode)
}

// Bytes serializes the devicetree into a flattened DTB blob.
func (b *Builder) Bytes() []byte {
	// Structure block (the root node has no name, "/").
	var structBuf bytes.Buffer

	root := b.root
	root.name = ""
	b.writeNode(&structBuf, &root)
	writeU32(&structBuf, tokenEnd)

	// Strings block: concatenation of NUL-terminated property names,
	// in the order they were first referenced above.
	var stringsBuf bytes.Buffer
	for _, s := range b.strings {
		stringsBuf.WriteString(s)
		stringsBuf.WriteByte(0)
	}

	const headerSize = 40 // 10 x uint32

	// No memory reservations: a single terminating {0,0} entry.
	memRsvMap := make([]byte, memRsvMapSize)

	offMemRsvMap := uint32(headerSize)
	offDTStruct := offMemRsvMap + u32(len(memRsvMap))
	offDTStrings := offDTStruct + u32(structBuf.Len())
	totalSize := offDTStrings + u32(stringsBuf.Len())

	var out bytes.Buffer

	writeU32(&out, magic)
	writeU32(&out, totalSize)
	writeU32(&out, offDTStruct)
	writeU32(&out, offDTStrings)
	writeU32(&out, offMemRsvMap)
	writeU32(&out, fdtVersion)
	writeU32(&out, fdtLastCompVerion)
	writeU32(&out, 0) // boot_cpuid_phys
	writeU32(&out, u32(stringsBuf.Len()))
	writeU32(&out, u32(structBuf.Len()))
	out.Write(memRsvMap)
	out.Write(structBuf.Bytes())
	out.Write(stringsBuf.Bytes())

	return out.Bytes()
}

package dtb

import "fmt"

// The memory map below (GICv2 distributor/CPU interface and PL011 UART
// addresses) matches the well-known convention used by QEMU's "virt"
// machine, so that this generated devicetree looks the same as one a
// guest kernel would normally see.
const (
	// UARTBase/UARTSize is the guest-physical MMIO window of the
	// console UART (PL011-compatible).
	UARTBase = 0x0900_0000
	UARTSize = 0x1000

	// GICDistBase/GICDistSize and GICCPUBase/GICCPUSize are the
	// guest-physical MMIO windows of the GICv2 distributor and CPU
	// interface, respectively.
	GICDistBase = 0x0800_0000
	GICDistSize = 0x1_0000
	GICCPUBase  = 0x0801_0000
	GICCPUSize  = 0x1_0000
)

// GenerateVirt builds a minimal devicetree, in the style of QEMU's
// "virt" machine, describing memSize bytes of RAM starting at guest
// physical address 0, nCpus "arm,armv8" CPUs booted via PSCI, a GICv2
// interrupt controller, the ARM architected timer, PSCI itself, and a
// PL011-compatible UART as the console, with bootargs as the kernel
// command line.
func GenerateVirt(memSize uint64, nCpus int, bootargs string) []byte {
	b := NewBuilder()

	b.AddPropString("/", "compatible", "linux,dummy-virt")
	b.AddPropU32("/", "#address-cells", 2)
	b.AddPropU32("/", "#size-cells", 2)
	b.AddPropU32("/", "interrupt-parent", 1) // phandle of /intc

	b.AddPropString("/chosen", "bootargs", bootargs)
	b.AddPropString("/chosen", "stdout-path", "/uart")

	b.AddPropU64Array("/memory", "reg", []uint64{0, memSize})
	b.AddPropString("/memory", "device_type", "memory")

	b.AddPropU32("/cpus", "#address-cells", 1)
	b.AddPropU32("/cpus", "#size-cells", 0)

	for i := 0; i < nCpus; i++ {
		path := fmt.Sprintf("/cpus/cpu@%d", i)
		b.AddPropString(path, "device_type", "cpu")
		b.AddPropString(path, "compatible", "arm,armv8")
		b.AddPropU32(path, "reg", uint32(i))
		b.AddPropString(path, "enable-method", "psci")
	}

	b.AddPropString("/psci", "compatible", "arm,psci-0.2")
	b.AddPropString("/psci", "method", "hvc")

	// GICv2: standard "arm,cortex-a15-gic" binding, reg = <dist, cpu>.
	b.AddPropString("/intc", "compatible", "arm,cortex-a15-gic")
	b.AddPropEmpty("/intc", "interrupt-controller")
	b.AddPropU32("/intc", "#interrupt-cells", 3)
	b.AddPropU64Array("/intc", "reg", []uint64{GICDistBase, GICDistSize, GICCPUBase, GICCPUSize})
	b.AddPropU32("/intc", "phandle", 1)

	// ARM architected timer: standard PPI assignment (secure phys, phys,
	// virt, hyp), each level-low triggered, as used by e.g. QEMU's virt
	// machine device tree.
	b.AddPropString("/timer", "compatible", "arm,armv8-timer")
	b.AddPropU32Array("/timer", "interrupts", []uint32{
		1, 13, 0xff08,
		1, 14, 0xff08,
		1, 11, 0xff08,
		1, 10, 0xff08,
	})

	b.AddPropStrings("/uart", "compatible", []string{"arm,pl011", "arm,primecell"})
	b.AddPropU64Array("/uart", "reg", []uint64{UARTBase, UARTSize})

	return b.Bytes()
}

package machine

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/kvm"
)

const (
	// arm64ImageMagic is the magic value found at offset 56 of an arm64
	// Linux "Image" kernel (Documentation/arch/arm64/booting.rst).
	arm64ImageMagic = 0x644d5241

	// arm64DefaultTextOffset is used when the header's own text_offset
	// field is zero, as permitted for kernels older than v4.6.
	arm64DefaultTextOffset = 0x80000

	// uartBase/uartSize define the MMIO window of a minimal, PL011-like
	// UART. Only the data register (offset 0) is implemented: writes are
	// printed to stdout, reads return 0. This is enough for polled early
	// console output; it is not a full PL011 emulation.
	uartBase = 0x0900_0000
	uartSize = 0x1000
)

// ErrNotARM64Image indicates the kernel file does not have a valid
// arm64 Image header magic.
var ErrNotARM64Image = errors.New("not an arm64 Image kernel (bad magic)")

// Machine is a minimal arm64/KVM virtual machine: it can create a VM and
// vcpus, load a raw arm64 Image kernel, and run it, printing any output
// written to a minimal MMIO UART. It does not (yet) generate a device
// tree blob, so a guest kernel will typically panic early looking for
// memory/console information -- see the package-level scope note.
type Machine struct {
	kvmFd, vmFd uintptr
	vcpuFds     []uintptr
	mem         []byte
	runs        []*kvm.RunData
	stopped     uint32
}

// Close stops all vcpus.
func (m *Machine) Close() error {
	atomic.StoreUint32(&m.stopped, 1)

	for _, r := range m.runs {
		r.ImmediateExit = 1
	}

	return nil
}

// New opens kvmPath, creates a VM with nCpus vcpus (each initialized via
// PreferredTarget/VCPUInitialize, as required on arm64 before a vcpu can
// be run), and attaches memSize bytes of guest RAM at guest physical
// address 0.
func New(kvmPath string, nCpus int, memSize int) (*Machine, error) {
	if memSize < MinMemSize {
		return nil, fmt.Errorf("memory size %d:%w", memSize, ErrMemTooSmall)
	}

	m := &Machine{}

	devKVM, err := os.OpenFile(kvmPath, os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	m.kvmFd = devKVM.Fd()

	if m.vmFd, err = kvm.CreateVM(m.kvmFd); err != nil {
		return nil, fmt.Errorf("CreateVM: %w", err)
	}

	// KVM_CREATE_IRQCHIP creates an in-kernel VGIC on arm64, just as it
	// creates the PIC/IOAPIC on x86.
	if err := kvm.CreateIRQChip(m.vmFd); err != nil {
		return nil, fmt.Errorf("CreateIRQChip: %w", err)
	}

	target, err := kvm.PreferredTarget(m.vmFd)
	if err != nil {
		return nil, fmt.Errorf("PreferredTarget: %w", err)
	}

	mmapSize, err := kvm.GetVCPUMMmapSize(m.kvmFd)
	if err != nil {
		return nil, err
	}

	m.vcpuFds = make([]uintptr, nCpus)
	m.runs = make([]*kvm.RunData, nCpus)

	for cpu := 0; cpu < nCpus; cpu++ {
		m.vcpuFds[cpu], err = kvm.CreateVCPU(m.vmFd, cpu)
		if err != nil {
			return nil, fmt.Errorf("CreateVCPU(%d): %w", cpu, err)
		}

		if err := kvm.VCPUInitialize(m.vcpuFds[cpu], target); err != nil {
			return nil, fmt.Errorf("VCPUInitialize(%d): %w", cpu, err)
		}

		r, err := syscall.Mmap(int(m.vcpuFds[cpu]), 0, int(mmapSize),
			syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return nil, err
		}

		m.runs[cpu] = (*kvm.RunData)(unsafe.Pointer(&r[0]))
	}

	if m.mem, err = syscall.Mmap(-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_ANONYMOUS); err != nil {
		return m, err
	}

	if err := kvm.SetUserMemoryRegion(m.vmFd, &kvm.UserspaceMemoryRegion{
		Slot: 0, Flags: 0, GuestPhysAddr: 0, MemorySize: uint64(memSize),
		UserspaceAddr: uint64(uintptr(unsafe.Pointer(&m.mem[0]))),
	}); err != nil {
		return m, err
	}

	return m, nil
}

// LoadLinux loads a raw arm64 "Image" kernel (not bzImage/PVH -- those
// are x86 formats) at the load address given by its own text_offset
// header field, and sets each vcpu's initial PC/X0/PSTATE per the arm64
// Linux boot protocol: PC = load address, X0 = dtb address (0, since we
// do not yet generate a device tree -- see package scope note), PSTATE
// = EL1h with IRQ/FIQ masked.
func (m *Machine) LoadLinux(kernel io.ReaderAt, params string) error {
	var header [64]byte
	if _, err := kernel.ReadAt(header[:], 0); err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	if binary.LittleEndian.Uint32(header[56:60]) != arm64ImageMagic {
		return ErrNotARM64Image
	}

	textOffset := binary.LittleEndian.Uint64(header[8:16])
	if textOffset == 0 {
		textOffset = arm64DefaultTextOffset
	}

	loadAddr := textOffset
	if int(loadAddr) >= len(m.mem) {
		return fmt.Errorf("%w: text_offset %#x is beyond guest memory size %#x",
			ErrBadVA, loadAddr, len(m.mem))
	}

	n, err := kernel.ReadAt(m.mem[loadAddr:], 0)
	if (err != nil && !errors.Is(err, io.EOF)) || n == 0 {
		if n == 0 && err == nil {
			return ErrZeroSizeKernel
		}

		return err
	}

	for _, vcpuFd := range m.vcpuFds {
		regs, err := kvm.GetRegs(vcpuFd)
		if err != nil {
			return err
		}

		regs.PC = loadAddr
		regs.Regs[0] = 0 // X0: dtb physical address, 0 == none provided
		regs.Pstate = kvm.PstateInit

		if err := kvm.SetRegs(vcpuFd, regs); err != nil {
			return err
		}
	}

	return nil
}

// uartMMIO handles MMIO accesses within the UART's address window. Only
// a write to the data register (offset 0) is meaningful: the low byte
// is printed to stdout.
func uartMMIO(physAddr, data uint64, length uint32, isWrite bool) uint64 {
	if !isWrite {
		return 0
	}

	if physAddr == uartBase && length >= 1 {
		fmt.Printf("%c", byte(data))
	}

	return 0
}

// RunOnce runs the guest vcpu until it exits, handling the MMIO UART,
// HLT, and interrupted-syscall exits. Any other exit reason is
// returned as an error.
func (m *Machine) RunOnce(cpu int) (bool, error) {
	fd := m.vcpuFds[cpu]

	_ = kvm.Run(fd)

	if atomic.LoadUint32(&m.stopped) != 0 {
		return false, ErrMachineStopped
	}

	exit := kvm.ExitType(m.runs[cpu].ExitReason)

	switch exit {
	case kvm.EXITHLT, kvm.EXITSHUTDOWN:
		return false, nil
	case kvm.EXITMMIO:
		physAddr, data, length, isWrite := m.runs[cpu].MMIO()
		if physAddr >= uartBase && physAddr < uartBase+uartSize {
			uartMMIO(physAddr, data, length, isWrite)
		}

		return true, nil
	case kvm.EXITINTR:
		return true, nil
	case kvm.EXITUNKNOWN:
		return true, nil
	default:
		return false, fmt.Errorf("%w: %s", kvm.ErrUnexpectedExitReason, exit.String())
	}
}

// RunInfiniteLoop runs a vcpu until it halts, is stopped, or errors.
func (m *Machine) RunInfiniteLoop(cpu int) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for {
		isContinue, err := m.RunOnce(cpu)
		if isContinue {
			if err != nil {
				log.Printf("%v", err)
			}

			continue
		}

		return err
	}
}

// GetRegs gets the general purpose registers for a vcpu.
func (m *Machine) GetRegs(cpu int) (*kvm.Regs, error) {
	return kvm.GetRegs(m.vcpuFds[cpu])
}

// SetRegs sets the general purpose registers for a vcpu.
func (m *Machine) SetRegs(cpu int, r *kvm.Regs) error {
	return kvm.SetRegs(m.vcpuFds[cpu], r)
}

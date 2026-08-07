package machine

import (
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/bootparam"
	"github.com/bobuhiro11/gokvm/ebda"
	"github.com/bobuhiro11/gokvm/iodev"
	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/pvh"
	"github.com/bobuhiro11/gokvm/serial"
	"golang.org/x/arch/x86/x86asm"
)

const (
	bootParamAddr = 0x10000
	cmdlineAddr   = 0x20000

	initrdAddr  = 0xf000000
	highMemBase = 0x100000

	pageTableBase = 0x30_000
)

const (
	// CR0 bits.
	CR0xPE = 1
	CR0xMP = (1 << 1)
	CR0xEM = (1 << 2)
	CR0xTS = (1 << 3)
	CR0xET = (1 << 4)
	CR0xNE = (1 << 5)
	CR0xWP = (1 << 16)
	CR0xAM = (1 << 18)
	CR0xNW = (1 << 29)
	CR0xCD = (1 << 30)
	CR0xPG = (1 << 31)

	// CR4 bits.
	CR4xVME        = 1
	CR4xPVI        = (1 << 1)
	CR4xTSD        = (1 << 2)
	CR4xDE         = (1 << 3)
	CR4xPSE        = (1 << 4)
	CR4xPAE        = (1 << 5)
	CR4xMCE        = (1 << 6)
	CR4xPGE        = (1 << 7)
	CR4xPCE        = (1 << 8)
	CR4xOSFXSR     = (1 << 8)
	CR4xOSXMMEXCPT = (1 << 10)
	CR4xUMIP       = (1 << 11)
	CR4xVMXE       = (1 << 13)
	CR4xSMXE       = (1 << 14)
	CR4xFSGSBASE   = (1 << 16)
	CR4xPCIDE      = (1 << 17)
	CR4xOSXSAVE    = (1 << 18)
	CR4xSMEP       = (1 << 20)
	CR4xSMAP       = (1 << 21)

	EFERxSCE = 1
	EFERxLME = (1 << 8)
	EFERxLMA = (1 << 10)
	EFERxNXE = (1 << 11)

	// 64-bit page entry bits.
	PDE64xPRESENT  = 1
	PDE64xRW       = (1 << 1)
	PDE64xUSER     = (1 << 2)
	PDE64xACCESSED = (1 << 5)
	PDE64xDIRTY    = (1 << 6)
	PDE64xPS       = (1 << 7)
	PDE64xG        = (1 << 8)
)

const (
	// Poison is x86 machine code that forces a vmexit.
	// Disassembly:
	//   mov eax, 0xcafebabe
	//   nop
	//   ud2
	Poison = "\xB8\xBE\xBA\xFE\xCA\x90\x0F\x0B"
)

// New creates a new KVM machine for amd64.
func New(kvmPath string, nCpus int, memSize int) (*Machine, error) {
	if memSize < MinMemSize {
		return nil, fmt.Errorf("memory size %d:%w", memSize, ErrMemTooSmall)
	}

	m := &Machine{}
	m.pci = newPCI()

	var err error

	m.kvmFd, m.vmFd, m.vcpuFds, m.runs, err = initVMandVCPU(kvmPath, nCpus)
	if err != nil {
		return nil, err
	}

	// Allocate tid slots for each vCPU goroutine.
	m.tids = make([]int32, nCpus)

	for cpuNr := range m.runs {
		if err := m.initCPUID(cpuNr); err != nil {
			return nil, err
		}
	}

	if m.mem, err = syscall.Mmap(-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_ANONYMOUS); err != nil {
		return m, err
	}

	err = kvm.SetUserMemoryRegion(m.vmFd, &kvm.UserspaceMemoryRegion{
		Slot: 0, Flags: 0, GuestPhysAddr: 0, MemorySize: uint64(memSize),
		UserspaceAddr: uint64(uintptr(unsafe.Pointer(&m.mem[0]))),
	})
	if err != nil {
		return m, err
	}

	// Poison memory using exponential doubling — ~14x faster than byte-by-byte copy.
	// Seed the first copy, then double the filled region each iteration.
	region := m.mem[highMemBase:]
	copy(region, Poison)
	for i := len(Poison); i < len(region); i *= 2 {
		copy(region[i:], region[:i])
	}

	return m, nil
}

// SetupRegs sets up the general purpose registers including RIP and BP.
func (m *Machine) SetupRegs(rip, bp uint64, amd64 bool) error {
	for _, cpu := range m.vcpuFds {
		if err := m.initRegs(cpu, rip, bp); err != nil {
			return err
		}

		if err := m.initSregs(cpu, amd64); err != nil {
			return err
		}
	}

	return nil
}

// RunData returns the kvm.RunData for the VM.
func (m *Machine) RunData() []*kvm.RunData {
	return m.runs
}

// GetRegs gets regs for vCPU.
func (m *Machine) GetRegs(cpu int) (*kvm.Regs, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return nil, err
	}

	return kvm.GetRegs(fd)
}

// GetSRegs gets sregs for vCPU.
func (m *Machine) GetSRegs(cpu int) (*kvm.Sregs, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return nil, err
	}

	return kvm.GetSregs(fd)
}

// SetRegs sets regs for vCPU.
func (m *Machine) SetRegs(cpu int, r *kvm.Regs) error {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return err
	}

	return kvm.SetRegs(fd, r)
}

// SetSRegs sets sregs for vCPU.
func (m *Machine) SetSRegs(cpu int, s *kvm.Sregs) error {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return err
	}

	return kvm.SetSregs(fd, s)
}

func (m *Machine) initRegs(vcpufd uintptr, rip, bp uint64) error {
	regs, err := kvm.GetRegs(vcpufd)
	if err != nil {
		return err
	}

	regs.RFLAGS = 2
	regs.RIP = rip
	regs.RSI = bp

	if err := kvm.SetRegs(vcpufd, regs); err != nil {
		return err
	}

	return nil
}

func (m *Machine) initSregs(vcpufd uintptr, amd64 bool) error {
	sregs, err := kvm.GetSregs(vcpufd)
	if err != nil {
		return err
	}

	if !amd64 {
		sregs.CS.Base, sregs.CS.Limit, sregs.CS.G = 0, 0xFFFFFFFF, 1
		sregs.DS.Base, sregs.DS.Limit, sregs.DS.G = 0, 0xFFFFFFFF, 1
		sregs.FS.Base, sregs.FS.Limit, sregs.FS.G = 0, 0xFFFFFFFF, 1
		sregs.GS.Base, sregs.GS.Limit, sregs.GS.G = 0, 0xFFFFFFFF, 1
		sregs.ES.Base, sregs.ES.Limit, sregs.ES.G = 0, 0xFFFFFFFF, 1
		sregs.SS.Base, sregs.SS.Limit, sregs.SS.G = 0, 0xFFFFFFFF, 1

		sregs.CS.DB, sregs.SS.DB = 1, 1
		sregs.CR0 |= 1

		if err := kvm.SetSregs(vcpufd, sregs); err != nil {
			return err
		}

		return nil
	}

	high64k := m.mem[pageTableBase : pageTableBase+0x6000]

	for i := range high64k {
		high64k[i] = 0
	}

	copy(high64k, []byte{
		0x03,
		0x10 | uint8((pageTableBase>>8)&0xff),
		uint8((pageTableBase >> 16) & 0xff),
		uint8((pageTableBase >> 24) & 0xff), 0, 0, 0, 0,
	})

	for i := uint64(0); i < 4; i++ {
		ptb := pageTableBase + (i+2)*0x1000
		copy(high64k[int(i*8)+0x1000:],
			[]byte{
				0x63,
				uint8((ptb >> 8) & 0xff),
				uint8((ptb >> 16) & 0xff),
				uint8((ptb >> 24) & 0xff), 0, 0, 0, 0,
			})
	}

	for i := uint64(0); i < 0x1_0000_0000; i += 0x2_00_000 {
		ptb := i | 0xe3
		ix := int((i/0x2_00_000)*8 + 0x2000)
		copy(high64k[ix:], []byte{
			uint8(ptb),
			uint8((ptb >> 8) & 0xff),
			uint8((ptb >> 16) & 0xff),
			uint8((ptb >> 24) & 0xff), 0, 0, 0, 0,
		})
	}

	if false {
		log.Printf("Page tables: %s", hex.Dump(m.mem[pageTableBase:pageTableBase+0x3000]))
	}

	sregs.CR3 = uint64(pageTableBase)
	sregs.CR4 = CR4xPAE
	sregs.CR0 = CR0xPE | CR0xMP | CR0xET | CR0xNE | CR0xWP | CR0xAM | CR0xPG
	sregs.EFER = EFERxLME | EFERxLMA

	seg := kvm.Segment{
		Base:     0,
		Limit:    0xffffffff,
		Selector: 1 << 3,
		Typ:      11,
		Present:  1,
		DPL:      0,
		DB:       0,
		S:        1,
		L:        1,
		G:        1,
		AVL:      0,
	}

	sregs.CS = seg

	seg.Typ = 3
	seg.Selector = 2 << 3
	sregs.DS, sregs.ES, sregs.FS, sregs.GS, sregs.SS = seg, seg, seg, seg, seg

	if err := kvm.SetSregs(vcpufd, sregs); err != nil {
		return err
	}

	return nil
}

func (m *Machine) initCPUID(cpu int) error {
	cpuid := kvm.CPUID{
		Nent:    100,
		Entries: make([]kvm.CPUIDEntry2, 100),
	}

	if err := kvm.GetSupportedCPUID(m.kvmFd, &cpuid); err != nil {
		return err
	}

	// https://www.kernel.org/doc/html/latest/virt/kvm/cpuid.html
	for i := 0; i < int(cpuid.Nent); i++ {
		switch cpuid.Entries[i].Function {
		case kvm.CPUIDFuncPerMon:
			cpuid.Entries[i].Eax = 0

		case kvm.CPUIDSignature:
			cpuid.Entries[i].Eax = kvm.CPUIDFeatures
			cpuid.Entries[i].Ebx = 0x4b4d564b
			cpuid.Entries[i].Ecx = 0x564b4d56
			cpuid.Entries[i].Edx = 0x4d

		case 7:
			cpuid.Entries[i].Edx &= ^(uint32(1) << 4)

		default:
			continue
		}
	}

	if err := kvm.SetCPUID2(m.vcpuFds[cpu], &cpuid); err != nil {
		return err
	}

	return nil
}

// SingleStep enables single stepping the guest.
func (m *Machine) SingleStep(onoff bool) error {
	for cpu := range m.vcpuFds {
		if err := kvm.SingleStep(m.vcpuFds[cpu], onoff); err != nil {
			return fmt.Errorf("single step %d:%w", cpu, err)
		}
	}

	return nil
}

// RunInfiniteLoop runs the guest cpu until there is an error.
// If the error is ErrExitDebug, this function can be called again.
// When the machine is stopped (via Close/Signal), it captures the vCPU
// register state into an AMD64State before returning.
func (m *Machine) RunInfiniteLoop(cpu int) error {
	// https://www.kernel.org/doc/Documentation/virtual/kvm/api.txt
	// - vcpu ioctls: These query and set attributes that control the operation
	//   of a single virtual cpu.
	//
	//   vcpu ioctls should be issued from the same thread that was used to create
	//   the vcpu, except for asynchronous vcpu ioctl that are marked as such in
	//   the documentation.  Otherwise, the first ioctl after switching threads
	//   could see a performance impact.
	//
	// - device ioctls: These query and set attributes that control the operation
	//   of a single device.
	//
	//   device ioctls must be issued from the same process (address space) that
	//   was used to create the VM.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Record this goroutine's OS thread ID so Close() can tgkill it.
	atomic.StoreInt32(&m.tids[cpu], int32(syscall.Gettid()))
	defer atomic.StoreInt32(&m.tids[cpu], 0)

	for {
		isContinue, err := m.RunOnce(cpu)
		if isContinue {
			if err != nil {
				fmt.Printf("%v\r\n", err)
			}

			continue
		}

		if err != nil {
			// If we're stopping, capture register state for resume.
			if errors.Is(err, ErrMachineStopped) {
				m.captureArchState(cpu)
			}

			return err
		}
	}
}

// RunOnce runs the guest vCPU until it exits.
func (m *Machine) RunOnce(cpu int) (bool, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return false, err
	}

	_ = kvm.Run(fd)

	if m.isStopped() {
		return false, ErrMachineStopped
	}

	exit := kvm.ExitType(m.runs[cpu].ExitReason)

	switch exit {
	case kvm.EXITHLT:
		return false, err
	case kvm.EXITIO:
		direction, size, port, count, offset := m.runs[cpu].IO()
		f := m.ioportHandlers[port][direction]

		bytes := (*(*[100]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(m.runs[cpu])) + uintptr(offset))))[0:size]
		for i := 0; i < int(count); i++ {
			if err := f(port, bytes); err != nil {
				return false, err
			}
		}

		return true, err
	case kvm.EXITUNKNOWN:
		return true, err
	case kvm.EXITINTR:
		// When a signal is sent to the thread hosting the VM it will result in EINTR
		// refs https://gist.github.com/mcastelino/df7e65ade874f6890f618dc51778d83a
		return true, nil
	case kvm.EXITDEBUG:
		return false, kvm.ErrDebug

	case kvm.EXITDCR,
		kvm.EXITEXCEPTION,
		kvm.EXITFAILENTRY,
		kvm.EXITHYPERCALL,
		kvm.EXITINTERNALERROR,
		kvm.EXITIRQWINDOWOPEN,
		kvm.EXITMMIO,
		kvm.EXITNMI,
		kvm.EXITS390RESET,
		kvm.EXITS390SIEIC,
		kvm.EXITSETTPR,
		kvm.EXITSHUTDOWN,
		kvm.EXITTPRACCESS:
		if err != nil {
			return false, err
		}

		return false, fmt.Errorf("%w: %s", kvm.ErrUnexpectedExitReason, exit.String())
	default:
		if err != nil {
			return false, err
		}

		r, _ := m.GetRegs(cpu)
		s, _ := m.GetSRegs(cpu)
		return false, fmt.Errorf("%w: %v: regs:\n%s",
			kvm.ErrUnexpectedExitReason,
			kvm.ExitType(m.runs[cpu].ExitReason).String(), show("", &s, &r))
	}
}

// VCPU runs a single vCPU goroutine.
func (m *Machine) VCPU(stdout io.Writer, cpu, traceCount int) error {
	trace := traceCount > 0

	var err error

	for tc := 0; ; tc++ {
		err = m.RunInfiniteLoop(cpu)
		if err == nil {
			continue
		}

		if !errors.Is(err, kvm.ErrDebug) {
			return fmt.Errorf("CPU %d: %w", cpu, err)
		}

		if err := m.SingleStep(trace); err != nil {
			fmt.Fprintf(stdout, "Setting trace to %v:%v", trace, err)
		}

		if tc%traceCount != 0 {
			continue
		}

		_, r, s, err := m.Inst(cpu)
		if err != nil {
			fmt.Fprintf(stdout, "disassembling after debug exit:%v", err)
		} else {
			fmt.Fprintf(stdout, "%#x:%s\r\n", r.RIP, s)
		}
	}
}

// LoadPVH loads a PVH firmware image (amd64 only).
func (m *Machine) LoadPVH(kern, initrd *os.File, cmdline string) error {
	edbaval := uint32(bootparam.EBDAStart >> 4)
	edbabytes := make([]byte, 4)

	binary.LittleEndian.PutUint32(edbabytes, edbaval)
	copy(m.mem[pvh.EBDAPointer:], edbabytes)

	e, err := ebda.New(len(m.vcpuFds))
	if err != nil {
		return err
	}

	eb, err := e.Bytes()
	if err != nil {
		return err
	}

	copy(m.mem[bootparam.EBDAStart:], eb)

	gdt := pvh.CreateGDT()

	copy(m.mem[pvh.BootGDTStart:], gdt.Bytes())
	copy(m.mem[pvh.BootIDTStart:], []byte{0x0})

	fwElf, err := elf.NewFile(kern)
	if err != nil {
		return err
	}

	ripAddr := fwElf.Entry

	for _, entry := range fwElf.Progs {
		if entry.Type == elf.PT_LOAD {
			_, err := entry.ReadAt(m.mem[entry.Paddr:], 0)
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
		} else if entry.Type == elf.PT_NOTE {
			if entry.Filesz == 0 {
				return errPTNoteHasNoFSize
			}

			addr, _ := pvh.ParsePVHEntry(kern, entry)

			if fwElf.Entry != uint64(addr) {
				ripAddr = uint64(addr)
			}
		}

		continue
	}

	for _, cpu := range m.vcpuFds {
		if err := pvh.InitRegs(cpu, ripAddr); err != nil {
			return err
		}

		if err := pvh.InitSRegs(cpu, gdt); err != nil {
			return err
		}
	}

	pvhstartinfo := pvh.NewStartInfo(bootparam.EBDAStart, cmdlineAddr)

	if initrd != nil {
		initrdSize, err := initrd.ReadAt(m.mem[initrdAddr:], 0)
		if err != nil && initrdSize == 0 && !errors.Is(err, io.EOF) {
			return fmt.Errorf("initrd: (%v, %w)", initrdSize, err)
		}

		copy(m.mem[cmdlineAddr:], cmdline)
		m.mem[cmdlineAddr+len(cmdline)] = 0

		ramdiskmod := pvh.NewModListEntry(initrdAddr, uint64(initrdSize), 0)

		pvhstartinfo.NrModules += 1
		pvhstartinfo.ModlistPAddr = pvh.PVHModlistStart

		ramdiskmodbytes, err := ramdiskmod.Bytes()
		if err != nil {
			return err
		}

		copy(m.mem[pvh.PVHModlistStart:], ramdiskmodbytes)

		m.AddDevice(&iodev.Noop{Port: 0x80, Psize: 0x30})
	} else {
		m.AddDevice(&iodev.PostCode{})
	}

	memmapentries := make([]*pvh.HVMMemMapTableEntry, 0)

	entry0 := pvh.NewMemMapTableEntry(0, bootparam.EBDAStart, bootparam.E820Ram)
	memmapentries = append(memmapentries, entry0)

	entry := pvh.NewMemMapTableEntry(pvh.HighRAMStart, uint64(len(m.mem)-pvh.HighRAMStart), bootparam.E820Ram)
	memmapentries = append(memmapentries, entry)

	pvhstartinfo.MemMapEntries = uint32(len(memmapentries))

	memOffset := pvh.PVHMemMapStart

	for _, entry := range memmapentries {
		b, err := entry.Bytes()
		if err != nil {
			return err
		}

		copy(m.mem[memOffset:], b)

		memOffset += len(b)
	}

	pvhstartinfob, err := pvhstartinfo.Bytes()
	if err != nil {
		return err
	}

	copy(m.mem[pvh.PVHInfoStart:], pvhstartinfob)

	if m.serial, err = serial.New(m); err != nil {
		return err
	}

	m.AddDevice(&iodev.FWDebug{})
	m.AddDevice(iodev.NewCMOS(0xC000000, 0x0))
	m.AddDevice(iodev.NewACPIPMTimer())
	m.initIOPortHandlers()

	return nil
}

// LoadLinux loads a bzImage or ELF kernel (amd64 only).
func (m *Machine) LoadLinux(kernel, initrd io.ReaderAt, params string) error {
	var (
		DefaultKernelAddr = uint64(highMemBase)
		err               error
	)

	e, err := ebda.New(len(m.vcpuFds))
	if err != nil {
		return err
	}

	bytes, err := e.Bytes()
	if err != nil {
		return err
	}

	copy(m.mem[bootparam.EBDAStart:], bytes)

	var initrdSize int
	if initrd != nil {
		initrdSize, err = initrd.ReadAt(m.mem[initrdAddr:], 0)
		if err != nil && initrdSize == 0 && !errors.Is(err, io.EOF) {
			return fmt.Errorf("initrd: (%v, %w)", initrdSize, err)
		}
	}

	copy(m.mem[cmdlineAddr:], params)
	m.mem[cmdlineAddr+len(params)] = 0

	var isElfFile bool

	k, err := elf.NewFile(kernel)
	if err == nil {
		isElfFile = true
	}

	bootParam := &bootparam.BootParam{}

	if !isElfFile {
		bootParam, err = bootparam.New(kernel)
		if err != nil {
			return err
		}
	}

	// refs https://github.com/kvmtool/kvmtool/blob/0e1882a49f81cb15d328ef83a78849c0ea26eecc/x86/bios.c#L66-L86
	bootParam.AddE820Entry(bootparam.RealModeIvtBegin, bootparam.EBDAStart-bootparam.RealModeIvtBegin, bootparam.E820Ram)
	bootParam.AddE820Entry(bootparam.EBDAStart, bootparam.VGARAMBegin-bootparam.EBDAStart, bootparam.E820Reserved)
	bootParam.AddE820Entry(bootparam.MBBIOSBegin, bootparam.MBBIOSEnd-bootparam.MBBIOSBegin, bootparam.E820Reserved)
	bootParam.AddE820Entry(highMemBase, uint64(len(m.mem)-highMemBase), bootparam.E820Ram)

	bootParam.Hdr.VidMode = 0xFFFF
	bootParam.Hdr.TypeOfLoader = 0xFF
	bootParam.Hdr.RamdiskImage = initrdAddr
	bootParam.Hdr.RamdiskSize = uint32(initrdSize)
	bootParam.Hdr.LoadFlags |= bootparam.CanUseHeap | bootparam.LoadedHigh | bootparam.KeepSegments
	bootParam.Hdr.HeapEndPtr = 0xFE00
	bootParam.Hdr.ExtLoaderVer = 0
	bootParam.Hdr.CmdlinePtr = cmdlineAddr
	bootParam.Hdr.CmdlineSize = uint32(len(params) + 1)

	bytes, err = bootParam.Bytes()
	if err != nil {
		return err
	}

	copy(m.mem[bootParamAddr:], bytes)

	var (
		amd64    bool
		kernSize int
	)

	switch isElfFile {
	case false:
		// Load kernel
		// copy to g.mem with offset setupsz
		//
		// The 32-bit (non-real-mode) kernel starts at offset (setup_sects+1)*512 in
		// the kernel file (again, if setup_sects == 0 the real value is 4.) It should
		// be loaded at address 0x10000 for Image/zImage kernels and highMemBase for bzImage kernels.
		//
		// refs: https://www.kernel.org/doc/html/latest/x86/boot.html#loading-the-rest-of-the-kernel
		setupsz := int(bootParam.Hdr.SetupSects+1) * 512

		kernSize, err = kernel.ReadAt(m.mem[DefaultKernelAddr:], int64(setupsz))

		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("kernel: (%v, %w)", kernSize, err)
		}
	case true:
		if k.Class == elf.ELFCLASS64 {
			amd64 = true
		}

		DefaultKernelAddr = k.Entry

		for i, p := range k.Progs {
			if p.Type != elf.PT_LOAD {
				continue
			}

			log.Printf("Load elf segment @%#x from file %#x %#x bytes", p.Paddr, p.Off, p.Filesz)

			n, err := p.ReadAt(m.mem[p.Paddr:], 0)
			if !errors.Is(err, io.EOF) || uint64(n) != p.Filesz {
				return fmt.Errorf("reading ELF prog %d@%#x: %d/%d bytes, err %w", i, p.Paddr, n, p.Filesz, err)
			}

			kernSize += n
		}
	}

	if kernSize == 0 {
		return ErrZeroSizeKernel
	}

	if err := m.SetupRegs(DefaultKernelAddr, bootParamAddr, amd64); err != nil {
		return err
	}

	if err := m.SetupDevices(); err != nil {
		return err
	}

	return nil
}

// SetupDevices initialises the serial console, CMOS, and IO port handlers.
// Called at the end of LoadLinux/LoadPVH and also on resume (Load).
func (m *Machine) SetupDevices() error {
	var err error

	if m.serial, err = serial.New(m); err != nil {
		return err
	}

	// Restore serial state if loaded from a saved state file.
	if m.pendingSerialIER != 0 || m.pendingSerialLCR != 0 {
		m.serial.IER = m.pendingSerialIER
		m.serial.LCR = m.pendingSerialLCR
		m.pendingSerialIER = 0
		m.pendingSerialLCR = 0
	}

	m.AddDevice(iodev.NewCMOS(0xC000_0000, 0x0))
	m.AddDevice(&iodev.Noop{Port: 0x80, Psize: 0xA0})
	m.initIOPortHandlers()

	return nil
}

// GetReg gets a pointer to a named x86 register in kvm.Regs.
func GetReg(r *kvm.Regs, reg x86asm.Reg) (*uint64, error) {
	if reg == x86asm.RAX {
		return &r.RAX, nil
	}
	if reg == x86asm.RCX {
		return &r.RCX, nil
	}
	if reg == x86asm.RDX {
		return &r.RDX, nil
	}
	if reg == x86asm.RBX {
		return &r.RBX, nil
	}
	if reg == x86asm.RSP {
		return &r.RSP, nil
	}
	if reg == x86asm.RBP {
		return &r.RBP, nil
	}
	if reg == x86asm.RSI {
		return &r.RSI, nil
	}
	if reg == x86asm.RDI {
		return &r.RDI, nil
	}
	if reg == x86asm.R8 {
		return &r.R8, nil
	}
	if reg == x86asm.R9 {
		return &r.R9, nil
	}
	if reg == x86asm.R10 {
		return &r.R10, nil
	}
	if reg == x86asm.R11 {
		return &r.R11, nil
	}
	if reg == x86asm.R12 {
		return &r.R12, nil
	}
	if reg == x86asm.R13 {
		return &r.R13, nil
	}
	if reg == x86asm.R14 {
		return &r.R14, nil
	}
	if reg == x86asm.R15 {
		return &r.R15, nil
	}
	if reg == x86asm.RIP {
		return &r.RIP, nil
	}

	return nil, fmt.Errorf("register %v%w", reg, ErrUnsupported)
}

// initVMandVCPU sets up the KVM VM and vCPUs for amd64.
func initVMandVCPU(kvmPath string, nCpus int) (uintptr, uintptr, []uintptr, []*kvm.RunData, error) {
	var err error

	devKVM, err := os.OpenFile(kvmPath, os.O_RDWR, 0o644)
	if err != nil {
		return 0, 0, nil, nil, err
	}

	kvmFd := devKVM.Fd()
	vmFd := uintptr(0)
	vcpuFds := make([]uintptr, nCpus)
	runs := make([]*kvm.RunData, nCpus)

	if vmFd, err = kvm.CreateVM(kvmFd); err != nil {
		return 0, 0, nil, nil, fmt.Errorf("CreateVM: %w", err)
	}

	if err := kvm.SetTSSAddr(vmFd, pvh.KVMTSSStart); err != nil {
		return 0, 0, nil, nil, err
	}

	if err := kvm.SetIdentityMapAddr(vmFd, pvh.KVMIdentityMapStart); err != nil {
		return 0, 0, nil, nil, err
	}

	if err := kvm.CreateIRQChip(vmFd); err != nil {
		return 0, 0, nil, nil, err
	}

	if err := kvm.CreatePIT2(vmFd); err != nil {
		return 0, 0, nil, nil, err
	}

	mmapSize, err := kvm.GetVCPUMMmapSize(kvmFd)
	if err != nil {
		return 0, 0, nil, nil, err
	}

	for cpu := 0; cpu < nCpus; cpu++ {
		vcpuFds[cpu], err = kvm.CreateVCPU(vmFd, cpu)
		if err != nil {
			return 0, 0, nil, nil, err
		}

		r, err := syscall.Mmap(int(vcpuFds[cpu]), 0, int(mmapSize),
			syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return 0, 0, nil, nil, err
		}

		runs[cpu] = (*kvm.RunData)(unsafe.Pointer(&r[0]))
	}

	return kvmFd, vmFd, vcpuFds, runs, nil
}

// Translate translates a virtual address for all active CPUs.
func (m *Machine) Translate(vaddr uint64) ([]*kvm.Translation, error) {
	t := make([]*kvm.Translation, 0, len(m.vcpuFds))

	for cpu := range m.vcpuFds {
		tr := &kvm.Translation{LinearAddress: vaddr}
		if err := kvm.Translate(m.vcpuFds[cpu], tr); err != nil {
			return t, err
		}

		t = append(t, tr)
	}

	return t, nil
}

// Arch returns the architecture string for this machine.
func (m *Machine) Arch() string {
	return runtime.GOARCH
}

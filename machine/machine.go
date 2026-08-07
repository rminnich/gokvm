package machine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/iodev"
	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/pci"
	"github.com/bobuhiro11/gokvm/serial"
	"github.com/bobuhiro11/gokvm/tap"
	"github.com/bobuhiro11/gokvm/virtio"
)

const (
	serialIRQ    = 4
	virtioNetIRQ = 9
	virtioBlkIRQ = 10

	MinMemSize = 1 << 25
)

var ErrZeroSizeKernel = errors.New("kernel is 0 bytes")

// ErrWriteToCF9 indicates a write to cf9, the standard x86 reset port.
var ErrWriteToCF9 = fmt.Errorf("power cycle via 0xcf9")

// ErrBadVA indicates a bad virtual address was used.
var ErrBadVA = fmt.Errorf("bad virtual address")

// ErrBadCPU indicates a cpu number is invalid.
var ErrBadCPU = fmt.Errorf("bad cpu number")

// ErrUnsupported indicates something we do not yet do.
var ErrUnsupported = fmt.Errorf("unsupported")

// ErrMemTooSmall indicates the requested memory size is too small.
var ErrMemTooSmall = fmt.Errorf("mem request must be at least 1<<20")

var ErrNotELF64File = fmt.Errorf("file is not ELF64")

var errPTNoteHasNoFSize = fmt.Errorf("elf programm PT_NOTE has file size equel zero")

// ErrMachineStopped is returned by RunOnce when Machine.Close has been called.
var ErrMachineStopped = errors.New("machine stopped")

// Machine holds the state for a KVM virtual machine.
type Machine struct {
	kvmFd, vmFd    uintptr
	vcpuFds        []uintptr
	mem            []byte
	runs           []*kvm.RunData
	pci            *pci.PCI
	serial         *serial.Serial
	devices        []iodev.Device
	ioportHandlers [0x10000][2]func(port uint64, bytes []byte) error
	stopped        uint32
}

// newPCI creates a new PCI bus with a bridge.
func newPCI() *pci.PCI {
	return pci.New(pci.NewBridge())
}

// Close stops vCPU goroutines and releases PCI device resources.
func (m *Machine) Close() error {
	atomic.StoreUint32(&m.stopped, 1)

	for _, r := range m.runs {
		r.ImmediateExit = 1
	}

	for _, d := range m.pci.Devices {
		if c, ok := d.(io.Closer); ok {
			c.Close()
		}
	}

	return nil
}

func (m *Machine) isStopped() bool {
	return atomic.LoadUint32(&m.stopped) != 0
}

func (m *Machine) AddTapIf(tapIfName string) error {
	t, err := tap.New(tapIfName)
	if err != nil {
		return err
	}

	v := virtio.NewNet(virtioNetIRQ, m, t, m.mem)
	go v.TxThreadEntry()
	go v.RxThreadEntry()
	m.pci.Devices = append(m.pci.Devices, v)

	return nil
}

func (m *Machine) AddDisk(diskPath string) error {
	v, err := virtio.NewBlk(diskPath, virtioBlkIRQ, m, m.mem)
	if err != nil {
		return err
	}

	go v.IOThreadEntry()
	m.pci.Devices = append(m.pci.Devices, v)

	return nil
}

// GetInputChan returns a chan <- byte for serial input.
func (m *Machine) GetInputChan() chan<- byte {
	return m.serial.GetInputChan()
}

// RunInfiniteLoop runs the guest cpu until there is an error.
// If the error is ErrExitDebug, this function can be called again.
// This is defined in machine_amd64.go for amd64; see that file for the implementation.

func (m *Machine) registerIOPortHandler(start, end uint64, inHandler, outHandler func(port uint64, bytes []byte) error) {
	for i := start; i < end; i++ {
		m.ioportHandlers[i][kvm.EXITIOIN] = inHandler
		m.ioportHandlers[i][kvm.EXITIOOUT] = outHandler
	}
}

func (m *Machine) initIOPortHandlers() {
	funcNone := func(port uint64, bytes []byte) error {
		return nil
	}

	funcError := func(port uint64, bytes []byte) error {
		return fmt.Errorf("%w: unexpected io port 0x%x", kvm.ErrUnexpectedExitReason, port)
	}

	funcOutbCF9 := func(port uint64, bytes []byte) error {
		if len(bytes) == 1 && bytes[0] == 0xe {
			return fmt.Errorf("write 0xe to cf9: %w", ErrWriteToCF9)
		}

		return fmt.Errorf("write %#x to cf9: %w", bytes, ErrWriteToCF9)
	}

	funcInbPS2 := func(port uint64, bytes []byte) error {
		bytes[0] = 0x20

		return nil
	}

	m.registerIOPortHandler(0, 0x10000, funcError, funcError)
	m.registerIOPortHandler(0xcf9, 0xcfa, funcNone, funcOutbCF9)
	m.registerIOPortHandler(0x3c0, 0x3db, funcNone, funcNone)
	m.registerIOPortHandler(0x3b4, 0x3b6, funcNone, funcNone)
	m.registerIOPortHandler(0x2f8, 0x300, funcNone, funcNone)
	m.registerIOPortHandler(0x3e8, 0x3f0, funcNone, funcNone)
	m.registerIOPortHandler(0x2e8, 0x2f0, funcNone, funcNone)
	m.registerIOPortHandler(0xcfe, 0xcff, funcNone, funcNone)
	m.registerIOPortHandler(0xcfa, 0xcfc, funcNone, funcNone)
	m.registerIOPortHandler(0xc000, 0xd000, funcNone, funcNone)
	m.registerIOPortHandler(0x60, 0x70, funcInbPS2, funcNone)
	m.registerIOPortHandler(0xed, 0xee, funcNone, funcNone)

	m.registerIOPortHandler(serial.COM1Addr, serial.COM1Addr+8, m.serial.In, m.serial.Out)

	m.registerIOPortHandler(0xcf8, 0xcf9, m.pci.PciConfAddrIn, m.pci.PciConfAddrOut)
	m.registerIOPortHandler(0xcfc, 0xd00, m.pci.PciConfDataIn, m.pci.PciConfDataOut)

	for _, dev := range m.devices {
		m.registerIOPortHandler(dev.IOPort(), dev.IOPort()+dev.Size(), dev.Read, dev.Write)
	}

	for _, dev := range m.pci.Devices {
		m.registerIOPortHandler(dev.IOPort(), dev.IOPort()+dev.Size(), dev.Read, dev.Write)
	}
}

// InjectSerialIRQ injects a serial interrupt.
func (m *Machine) InjectSerialIRQ() error {
	if err := kvm.IRQLineStatus(m.vmFd, serialIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLineStatus(m.vmFd, serialIRQ, 1); err != nil {
		return err
	}

	return nil
}

// InjectVirtioNetIRQ injects a virtio net interrupt.
func (m *Machine) InjectVirtioNetIRQ() error {
	if err := kvm.IRQLineStatus(m.vmFd, virtioNetIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLineStatus(m.vmFd, virtioNetIRQ, 1); err != nil {
		return err
	}

	return nil
}

// InjectVirtioBlkIRQ injects a virtio block interrupt.
func (m *Machine) InjectVirtioBlkIRQ() error {
	if err := kvm.IRQLineStatus(m.vmFd, virtioBlkIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLineStatus(m.vmFd, virtioBlkIRQ, 1); err != nil {
		return err
	}

	return nil
}

// ReadAt implements io.ReaderAt for the guest memory.
func (m *Machine) ReadAt(b []byte, off int64) (int, error) {
	mem := bytes.NewReader(m.mem)

	return mem.ReadAt(b, off)
}

// WriteAt implements io.WriterAt for the guest memory.
func (m *Machine) WriteAt(b []byte, off int64) (int, error) {
	if off > int64(len(m.mem)) {
		return 0, syscall.EFBIG
	}

	n := copy(m.mem[off:], b)

	return n, nil
}

func showone(indent string, in interface{}) string {
	var ret string

	s := reflect.ValueOf(in).Elem()
	typeOfT := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		if f.Kind() == reflect.String {
			ret += fmt.Sprintf(indent+"%s %s = %s\n", typeOfT.Field(i).Name, f.Type(), f.Interface())
		} else {
			ret += fmt.Sprintf(indent+"%s %s = %#x\n", typeOfT.Field(i).Name, f.Type(), f.Interface())
		}
	}

	return ret
}

func show(indent string, l ...interface{}) string {
	var ret string

	for _, i := range l {
		ret += showone(indent, i)
	}

	return ret
}

// CPUToFD translates a CPU number to an fd.
func (m *Machine) CPUToFD(cpu int) (uintptr, error) {
	if cpu > len(m.vcpuFds) {
		return 0, fmt.Errorf("cpu %d out of range 0-%d:%w", cpu, len(m.vcpuFds), ErrBadCPU)
	}

	return m.vcpuFds[cpu], nil
}

// VtoP returns the physical address for a vCPU virtual address.
func (m *Machine) VtoP(cpu int, vaddr uint64) (int64, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return 0, err
	}

	t := &kvm.Translation{LinearAddress: vaddr}
	if err := kvm.Translate(fd, t); err != nil {
		return -1, err
	}

	if t.Valid == 0 || t.PhysicalAddress > uint64(len(m.mem)) {
		return -1, fmt.Errorf("%#x:valid not set:%w", vaddr, ErrBadVA)
	}

	return int64(t.PhysicalAddress), nil
}

func (m *Machine) GetSerial() *serial.Serial {
	return m.serial
}

func (m *Machine) AddDevice(dev iodev.Device) {
	m.devices = append(m.devices, dev)
}

// unsafeRunDataBytes returns the IO data bytes from a RunData.
// This is used by RunOnce (defined in machine_amd64.go) to access the
// IO port data buffer.
func unsafeRunDataBytes(rd *kvm.RunData, offset, size uintptr) []byte {
	return (*(*[100]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(rd)) + offset)))[0:size]
}

// logRunOnce logs a warning when RunOnce exits unexpectedly.
func logRunOnce(msg string) {
	log.Printf("RunOnce: %s", msg)
}

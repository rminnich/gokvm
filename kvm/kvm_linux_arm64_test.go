package kvm_test

import (
	"os"
	"testing"

	"github.com/bobuhiro11/gokvm/kvm"
)

// setupARM64VCPU opens /dev/kvm, creates a VM and a single vcpu, and
// initializes the vcpu with the host's preferred target. On arm64,
// VCPUInitialize must be called before any other vcpu ioctl (GetRegs,
// SetRegs, GetOneReg, SetOneReg, Run, ...) will succeed.
func setupARM64VCPU(t *testing.T) (devKVM *os.File, vmFd, vcpuFd uintptr) {
	t.Helper()

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	vmFd, err = kvm.CreateVM(devKVM.Fd())
	if err != nil {
		devKVM.Close()
		t.Fatal(err)
	}

	vcpuFd, err = kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		devKVM.Close()
		t.Fatal(err)
	}

	init, err := kvm.PreferredTarget(vmFd)
	if err != nil {
		devKVM.Close()
		t.Fatal(err)
	}

	if err := kvm.VCPUInitialize(vcpuFd, init); err != nil {
		devKVM.Close()
		t.Fatal(err)
	}

	return devKVM, vmFd, vcpuFd
}

func TestARM64PreferredTarget(t *testing.T) {
	t.Parallel()

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	init, err := kvm.PreferredTarget(vmFd)
	if err != nil {
		t.Fatal(err)
	}

	if init.Target > kvm.TargetGenericV8 {
		t.Fatalf("unexpected preferred target: %d", init.Target)
	}
}

func TestARM64VCPUInitialize(t *testing.T) {
	t.Parallel()

	devKVM, _, _ := setupARM64VCPU(t)
	defer devKVM.Close()
}

func TestARM64GetSetRegs(t *testing.T) {
	t.Parallel()

	devKVM, _, vcpuFd := setupARM64VCPU(t)
	defer devKVM.Close()

	regs, err := kvm.GetRegs(vcpuFd)
	if err != nil {
		t.Fatal(err)
	}

	const wantPC = 0x8000_0000

	regs.PC = wantPC
	regs.Regs[0] = 0x1234

	if err := kvm.SetRegs(vcpuFd, regs); err != nil {
		t.Fatal(err)
	}

	got, err := kvm.GetRegs(vcpuFd)
	if err != nil {
		t.Fatal(err)
	}

	if got.PC != wantPC {
		t.Errorf("PC: got %#x, want %#x", got.PC, wantPC)
	}

	if got.Regs[0] != 0x1234 {
		t.Errorf("Regs[0]: got %#x, want %#x", got.Regs[0], 0x1234)
	}
}

func TestARM64OneRegPC(t *testing.T) {
	t.Parallel()

	devKVM, _, vcpuFd := setupARM64VCPU(t)
	defer devKVM.Close()

	const want uint64 = 0x4000_0000

	set := want
	if err := kvm.SetOneReg(vcpuFd, kvm.RegPC, &set); err != nil {
		t.Fatal(err)
	}

	var got uint64
	if err := kvm.GetOneReg(vcpuFd, kvm.RegPC, &got); err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Errorf("PC via ONE_REG: got %#x, want %#x", got, want)
	}

	// GetRegs should agree with the value set via ONE_REG.
	regs, err := kvm.GetRegs(vcpuFd)
	if err != nil {
		t.Fatal(err)
	}

	if regs.PC != want {
		t.Errorf("PC via GetRegs: got %#x, want %#x", regs.PC, want)
	}
}

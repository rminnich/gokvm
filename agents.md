# Agent: Go Systems Architect

## Current Objective

Multi-architecture support and clean VM lifecycle via Runner interface.

## Context

- **Project**: gokvm
- **Architecture**: amd64 primary; arm64 + riscv64 build-verified stubs
- **Repo branch**: main
- **HEAD commit**: `9ed286b` vmm: Info() returns *Save with Mem+VCPUs; vmm.Save/VCPUSave types; machine.Mem() accessor

## Coding Conventions

- No line wrapping unless line exceeds 256 characters
- Exception: map literals and struct slice literals may be wrapped freely
- After each change: run boot tests, then commit
- Commit messages: concise, no fluff or praise
- **Architecture-specific code**: `runtime.GOARCH` at runtime where possible; `_GOARCH.go` file naming otherwise. No `//go:build` tags.
- **agents.md always in its own separate commit**

## Boot Test Protocol

30s timeout (60s for kernel_cpu). KVM on this machine is flaky — retry up to 3 times.

**make qemu:**
```
expect -c '
set timeout 30
spawn qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm --append "root=/dev/ram rw console=ttyS0 rdinit=/init"
expect "Setting console log level"
sleep 1
send "echo booted\r"
expect { "booted" { puts "PASS: make qemu"; exit 0 } timeout { puts "FAIL"; exit 1 } }
'
```

**make run-cpu (preferred):**
```
expect -c '
set timeout 60
spawn ./gokvm boot -c 1 -k ./kernel_cpu -i ./initrd
expect "Setting console log level"
sleep 3
send "echo booted\r"
expect { "booted" { puts "PASS: run-cpu"; exit 0 } timeout { puts "FAIL"; exit 1 } }
'
```

**multi-arch build:**
```
go build ./... && GOARCH=arm64 go build ./... && GOARCH=riscv64 go build ./... && echo "all ok"
```

## Kernels

| Kernel | Version | Boot time | Notes |
|---|---|---|---|
| `bzImage` | 5.14.3 | ~minutes | Heavy lockdep; `-c 2` |
| `kernel_cpu` | 6.0.0 | ~35s | Clean, NR_CPUS=1; `-c 1`; **preferred** |

`make kernel_cpu` downloads from https://github.com/u-root/cpu/raw/main/vm/kernel_linux_amd64

## Runner Interface (vmm/vmm.go)

```go
type Runner interface {
    Init() error    // create/configure VM hardware
    Setup() error   // load kernel/initrd into guest memory
    Boot() error    // start vCPUs + console I/O, blocks until exit
    Close() error   // stop VMM, interrupt blocked vCPU ioctls
    Info() *Save    // return Save struct after VM is stopped
}
```

## vmm.Save / vmm.VCPUSave (vmm/save.go)

```go
// machine.Save (machine/save.go) — RAM captured once, shared across VMMs.
type machine.Save struct {
    Mem []byte   // snapshot of guest RAM
}
func (m *Machine) NewSave() *machine.Save  // captures RAM

// vmm.Save — top-level save struct.
type Save struct {
    Arch    string
    Machine *machine.Save  // guest RAM — one copy, not per-VMM
    VCPUs   []VCPUSave     // per-vCPU register state
}

type VCPUSave struct {
    CPU  int
    Regs interface{}  // *machine.AMD64State on amd64; nil otherwise
}
```

`amd64VMM.Info()` calls `v.m.NewSave()` for RAM (once) and `GetAMD64State()` per CPU.
Stub arches return `&Save{Arch: runtime.GOARCH}` with nil Machine.

## machine.AMD64State (machine/archstate_amd64.go)

```go
type AMD64State struct {
    GPR    map[x86asm.Reg]uint64  // RAX..R15, RIP (x86asm constants)
    RFLAGS uint64
    Sregs  kvm.Sregs              // CR0,CR3,CR4,EFER,segments,etc.
}
```

Captured automatically in `RunInfiniteLoop` when `ErrMachineStopped` is returned.
Retrieved via `machine.GetAMD64State()` — nil until VM is stopped.

## machine.Signal / machine.Close

```go
// Send any signal to all vCPU OS threads via tgkill.
func (m *Machine) Signal(sig syscall.Signal)

// Close sets stopped=1, ImmediateExit=1, then Signal(SIGHUP).
// kvm.Run returns EINTR → nil → isStopped() → ErrMachineStopped.
func (m *Machine) Close() error
```

tids tracked in `m.tids []int32`, set by `RunInfiniteLoop` after `LockOSThread`.

## Exit Sequence (^A^X or ^A^Z)

1. `serial.Start` breaks out of its loop (^A^X) or saves then breaks (^A^Z)
2. Serial goroutine calls `v.Close()` → `machine.Close()`
3. Sets `stopped=1`, `ImmediateExit=1`, `Signal(SIGHUP)` to each vCPU thread
4. `kvm.Run` returns EINTR → `isStopped()` → `ErrMachineStopped`
5. `RunInfiniteLoop` calls `captureArchState(cpu)` → stores `*AMD64State`
6. All goroutines return → `g.Wait()` unblocks → process exits

## Architecture Files

| File | Description |
|---|---|
| `vmm/vmm.go` | Runner interface, Config, VMM struct |
| `vmm/save.go` | Save, VCPUSave types |
| `vmm/vmm_amd64.go` | `amd64VMM` full impl |
| `vmm/vmm_arm64.go` | arm64 stub |
| `vmm/vmm_riscv64.go` | riscv64 stub |
| `machine/archstate_amd64.go` | AMD64State, captureArchState, GetAMD64State |
| `machine/state_amd64.go` | Save(path)/Load(path) gob file I/O (paused — GetRegs deadlock) |

## Save/Restore Status

**Phase 1 (save)**: WORKING. `^A^Z` → `serial.Start` saves synchronously → `os.Exit(0)`. ~1G gob file.

**Phase 2 (resume)**: PARTIALLY WORKING. VM resumes, guest kernel runs, u-root init starts.
Crash in guest: `uart_start+0x118` overflow — guest serial driver locking issue on resume.
Root cause: guest's UART driver has internal state (IER, LCR, buffer locks) that conflicts
with the freshly-reset emulated serial port after resume.

**What works**: memory restored, registers restored, CPU runs, kernel executes, u-root starts.
**What fails**: guest tty layer crashes flushing buffered serial data after resume.

**Fix needed**: Either save/restore the serial port state (IER, LCR) in the gob file,
or flush the guest's tty buffers before saving (e.g. send a sentinel to drain the serial).

**`SetupDevices()`** added to `machine_amd64.go` — called by both `LoadLinux` and the resume path.
This fixed the nil pointer panic that previously prevented resume from running at all.

## Makefile Targets

```
make run           # bzImage, -c 2
make run-cpu       # kernel_cpu, -c 1
make qemu          # qemu with bzImage
make kernel_cpu    # download u-root/cpu kernel
make build-arm64   # GOARCH=arm64 go build ./...
make build-riscv64 # GOARCH=riscv64 go build ./...
make build-otherarch # both
```

## Session State Checklist

- [x] Interface refactor
- [x] Runner interface — Init/Setup/Boot/Close/Info
- [x] vmm.Save / vmm.VCPUSave — per-vCPU register state container
- [x] machine.AMD64State — x86asm.Reg GPR map + kvm.Sregs
- [x] machine.Signal(syscall.Signal) — tgkill to all vCPU threads
- [x] machine.Close() — uses Signal(SIGHUP); clean exit confirmed
- [x] machine.Mem() — returns copy of guest RAM
- [x] AMD64State captured on ErrMachineStopped in RunInfiniteLoop
- [x] arm64/riscv64 stubs — Close()/Info()
- [x] kernel_cpu (Linux 6.0) — make run-cpu
- [x] ^A^X / ^A^Z — serial breaks, calls Close(), vCPUs exit
- [ ] Save/restore via file — GetRegs deadlock (use AMD64State path instead)
- [ ] ppc64le/s390x stubs (trivial)
- [ ] Wire Info()/Save into resume path (replace gob Save/Load)

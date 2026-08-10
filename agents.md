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

30s timeout. KVM on this machine is flaky — retry up to 3 times.

**make lkvm (preferred — closest to gokvm):**
```
expect -c '
set timeout 30
spawn ~/bin/lkvm run -k ./kernel_cpu -i ./initrd
expect "Setting console log level"
sleep 1
send "echo booted\r"
expect { "booted" { puts "PASS: lkvm"; exit 0 } timeout { puts "FAIL"; exit 1 } }
'
```

**make run-cpu (gokvm with kernel_cpu):**
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

**make qemu (fallback, bzImage):**
```
expect -c '
set timeout 30
spawn qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm --append "root=/dev/ram rw console=ttyS0 rdinit=/init"
expect "Setting console log level"
sleep 1
send "echo booted\r"
expect { "booted" { puts "PASS: qemu"; exit 0 } timeout { puts "FAIL"; exit 1 } }
'
```

**multi-arch build:**
```
go build ./... && GOARCH=arm64 go build ./... && GOARCH=riscv64 go build ./... && echo "all ok"
```

**make net-test (iperf3 over virtio-net):**
```
make net-test
```

Requires `kernel.apparmor_restrict_unprivileged_userns=0` on the host
(`sudo sysctl kernel.apparmor_restrict_unprivileged_userns=0`); otherwise
the userns tap setup fails. See scripts/net-test.sh + scripts/net-test.expect.

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

## Boot Performance

| Configuration | Time |
|---|---|
| lkvm | ~2.0s |
| gokvm (with poison) | ~2.7s |
| gokvm -P (no poison) | ~2.4s |

**`-P` flag** disables memory poisoning — use for performance testing. Poison uses exponential doubling (~330ms, mostly page faults on 1G RAM).

**IRQ routing**: EBDA MP table now has Bus (ISA), IOAPIC, and 16 I/O interrupt source entries. Eliminates `BIOS bug, no explicit IRQ entries` kernel message. `nr_irqs` now 48 (was 24). Implemented in `ebda/ebda_amd64.go`.

**Networking**: initrd uses u-root's default gosh shell (removed `-defaultsh bash`). gosh does not source `.bashrc`, so `make net-test` configures eth0 directly in the guest. virtio-net iperf3 results (kernel_cpu, tap, single stream):

| Test | gokvm pre-GSO | gokvm GSO | lkvm |
|---|---|---|---|
| TCP guest RX (host→guest) | ~5.4–6.7 Gbits/s | ~23.5 Gbits/s | ~26.6 Gbits/s |
| TCP guest TX (guest→host) | ~3.8–4.3 Gbits/s | ~16.8 Gbits/s | ~22.0 Gbits/s |
| UDP jitter / loss | 0ms / 0% | 0ms / 0% | 0ms / 0% |

Perf work (commits f06b448, 175c242, 778587b): QueueSize 32→256 (vring_size(256,4096)=10246 bytes), RX drains the tap and injects one IRQ per batch with a reused buffer, TX accumulates descriptor chains into one reused buffer, VIRTIO_RING_F_EVENT_IDX advertised and honored (used_event suppresses IRQs, avail_event written back so the guest keeps kicking), and tap GSO offload (TUNSETOFFLOAD + IFF_VNET_HDR): tap does the segmentation, virtio-net advertises the GSO feature set, vnet hdr forwarded verbatim on RX and written whole on TX.

Two bugs fixed in 778587b:
- Feature bits were one bit too high (set advertised VIRTIO_NET_F_MAC=5 instead of GSO=6); guest read zeroed config MAC → EADDRNOTAVAIL on link up. Values now match include/uapi/linux/virtio_net.h.
- Rx() consumed one avail entry per descriptor; with GSO the guest posts big RX buffers as chains of MAX_SKB_FRAGS+2 descriptors. Rx() now follows the driver's Next pointers, one avail+used entry per packet.

Remaining gap to lkvm is the modern virtio-pci MMIO + MSI-X interface. Compare via `make net-test` (gokvm) and `make net-test-lkvm`.

**Phase 1 (save)**: WORKING. `^A^Z` → saves synchronously → `os.Exit(0)`. ~1G gob file.
Saves: guest RAM, CPU Regs+Sregs per vCPU, serial IER+LCR.

**Phase 2 (resume)**: PARTIAL. VM resumes, kernel runs, u-root init starts.
Crashes in guest `uart_start+0x118` — Linux tty layer software state inconsistency.
This is a fundamental kernel issue, not a hardware register gap.

**Paused here** — significant progress made. Resume from scratch on this item when ready.

**VMState** (machine/state_amd64.go):
```go
type VMState struct {
    Mem       []byte
    Regs      []*kvm.Regs
    Sregs     []*kvm.Sregs
    SerialIER byte
    SerialLCR byte
}
```

## Makefile Targets

```
make run           # bzImage, -c 2
make run-cpu       # kernel_cpu, -c 1
make lkvm          # lkvm run -k kernel_cpu (preferred test)
make qemu          # qemu with bzImage (fallback)
make kernel_cpu    # download u-root/cpu kernel
make build-arm64   # GOARCH=arm64 go build ./...
make build-riscv64 # GOARCH=riscv64 go build ./...
make build-otherarch # both
make test-save-restore # save/restore test with kernel_cpu
make net-test       # iperf3 over virtio-net tap, gokvm (needs userns sysctl 0)
make net-test-lkvm  # same iperf3 harness against lkvm for comparison
```

## Session State Checklist

- [x] Interface refactor
- [x] Runner interface — Init/Setup/Boot/Close/Info
- [x] vmm.Save / vmm.VCPUSave — per-vCPU register state container
- [x] machine.AMD64State — x86asm.Reg GPR map + kvm.Sregs
- [x] machine.Signal(syscall.Signal) — tgkill to all vCPU threads
- [x] machine.Close() — uses Signal(SIGHUP); clean exit confirmed
- [x] AMD64State captured on ErrMachineStopped in RunInfiniteLoop
- [x] arm64/riscv64 stubs — Close()/Info()
- [x] kernel_cpu (Linux 6.0) — make run-cpu / make lkvm
- [x] ^A^X / ^A^Z — serial breaks, calls Close(), vCPUs exit
- [x] lkvm as preferred test tool — make lkvm
- [x] -P flag — disable memory poison for faster boot (~2.4s vs ~2.7s)
- [x] EBDA MP table — Bus/IOAPIC/IRQ entries; eliminates "BIOS bug" kernel message
- [x] Exponential doubling for poison (~14x faster than loop copy)
- [x] Lean cmdline — removed notsc/debug/dyndbg; ~2.7s boot (was minutes)
- [x] net-test — iperf3 over virtio-net tap; gosh default shell; TX 475M/RX 1.1G
- [x] virtio-net perf: QueueSize 256, batched RX IRQ, reused buffers, event_idx — RX 5.4–6.7G / TX 3.8–4.3G
- [ ] Save/restore via file — paused (tty software state crash on resume)
- [ ] ppc64le/s390x stubs (trivial)

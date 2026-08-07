# Agent: Go Systems Architect

## Current Objective

Multi-architecture support and clean VM lifecycle management via Runner interface.

## Context

- **Project**: gokvm
- **Architecture**: amd64 primary; arm64 + riscv64 build-verified stubs
- **Repo branch**: main
- **HEAD commit**: `199d7cd` gitignore: add *.swp

## Coding Conventions

- No line wrapping unless line exceeds 256 characters
- Exception: map literals and struct slice literals may be wrapped freely
- After each change: run `make run` and `make qemu` boot tests, then commit
- Commit messages: concise, no fluff or praise
- **Architecture-specific code**: `runtime.GOARCH` at runtime where possible; `_GOARCH.go` file naming otherwise. No `//go:build` tags.
- **agents.md always in its own separate commit**

## Boot Test Protocol

30s timeout. KVM on this machine is flaky — retry up to 3 times.

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

**make run:**
```
expect -c '
set timeout 30
spawn ./gokvm boot -c 2 -i ./initrd
expect "Setting console log level"
sleep 3
send "echo booted\r"
expect { "booted" { puts "PASS: make run"; exit 0 } timeout { puts "FAIL"; exit 1 } }
'
```

**multi-arch build test:**
```
go build ./... && GOARCH=arm64 go build ./... && GOARCH=riscv64 go build ./... && echo "all ok"
```

## Runner Interface (vmm/vmm.go)

```go
type Runner interface {
    Init() error    // create/configure VM hardware
    Setup() error   // load kernel/initrd into guest memory
    Boot() error    // start vCPUs + console I/O, blocks until exit
    Close() error   // stop VMM, interrupt blocked vCPU ioctls
    Info() VMInfo   // return arch-defined info block
}

type VMInfo struct {
    Arch         string
    NCPUs        int
    MemSize      int
    KernelPath   string
    CPUIDEntries []CPUIDEntry  // amd64 only
}

type CPUIDEntry struct {
    Function, Index, Eax, Ebx, Ecx, Edx uint32
}
```

## Architecture Files

| File | Description |
|---|---|
| `vmm/vmm.go` | Runner interface, VMInfo, Config, VMM struct |
| `vmm/vmm_amd64.go` | `amd64VMM` — full implementation; `Info()` returns CPUID from KVM |
| `vmm/vmm_arm64.go` | `arm64Runner` stub — all methods return "not supported" |
| `vmm/vmm_riscv64.go` | `riscv64Runner` stub — all methods return "not supported" |

## machine.Close() — tgkill approach

`Machine` now tracks each vCPU goroutine's OS thread ID in `m.tids []int32`.
`RunInfiniteLoop` stores `syscall.Gettid()` into `m.tids[cpu]` after `LockOSThread`.
`machine.Close()` sends `SIGUSR1` to each tid via `syscall.Tgkill(pid, tid, SIGUSR1)`.

**Status**: implemented but not yet verified to reliably unblock `kvm.Run`.
The `kvm.Run()` function already handles `EINTR` (returns nil), then `isStopped()` returns true → `ErrMachineStopped`.

## Known Issues / Next Steps

### Save/Restore (paused)
- `^A^Z` triggers save but `kvm.GetRegs(fd)` deadlocks with running vCPU on same fd
- Fix: `Save()` now sets `stopped=1` + `ImmediateExit=1` before calling `GetRegs` — but race still possible
- `^A^X` exits cleanly via `os.Exit(0)` in `serial.Start`
- Low priority vs multi-arch work

### Multi-arch
- [x] amd64 — full implementation
- [x] arm64 — stub with `Close()`/`Info()`
- [x] riscv64 — stub with `Close()`/`Info()`
- [ ] ppc64le — needs `vmm_ppc64le.go` stub (same pattern as arm64/riscv64)
- [ ] s390x — needs `vmm_s390x.go` stub

### Better kernel
- Host has Linux 7.0 at `/home/rminnich/badgokvm/vmlinuz` (root-only, 17M)
- Current test kernel is 5.14.3 with heavy lockdep testsuite overhead
- `kunit.enable=0` in cmdline helps but kernel still slow

## Makefile Targets

```
make run          # ./gokvm boot -c 2 -i ./initrd
make qemu         # qemu-system-x86_64 with bzImage
make build-arm64  # GOARCH=arm64 go build ./...
make build-riscv64 # GOARCH=riscv64 go build ./...
make build-otherarch # both arm64 and riscv64
```

## Session State Checklist

- [x] Interface refactor
- [x] Resumable state — ^A^Z save, -R resume (save blocks on GetRegs, paused)
- [x] Boot test protocol — expect-based, 30s timeout
- [x] Serial input fix
- [x] Boot time — `kunit.enable=0`
- [x] Architecture factoring — `_amd64.go` files; no build tags
- [x] Runner interface — Init/Setup/Boot/Close/Info
- [x] VMInfo struct — arch-neutral with CPUID on amd64
- [x] Close() — tgkill SIGUSR1 to vCPU threads
- [x] arm64 stub — Close()/Info()
- [x] riscv64 stub — Close()/Info()
- [x] ^A^X clean exit via os.Exit(0)
- [ ] Save/restore deadlock fix (paused)
- [ ] ppc64le/s390x stubs
- [ ] Verify tgkill actually unblocks kvm.Run reliably

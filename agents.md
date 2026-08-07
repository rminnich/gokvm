# Agent: Go Systems Architect

## Current Objective

Multi-architecture support and clean VM lifecycle management via Runner interface.

## Context

- **Project**: gokvm
- **Architecture**: amd64 primary; arm64 + riscv64 build-verified stubs
- **Repo branch**: main
- **HEAD commit**: `235d943` Makefile: add kernel_cpu target (u-root/cpu 6.0 kernel); run-cpu target uses -c 1

## Coding Conventions

- No line wrapping unless line exceeds 256 characters
- Exception: map literals and struct slice literals may be wrapped freely
- After each change: run boot tests, then commit
- Commit messages: concise, no fluff or praise
- **Architecture-specific code**: `runtime.GOARCH` at runtime where possible; `_GOARCH.go` file naming otherwise. No `//go:build` tags.
- **agents.md always in its own separate commit**

## Boot Test Protocol

30s timeout (60s for kernel_cpu). KVM on this machine is flaky — retry up to 3 times.

**make qemu (fastest):**
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

**make run-cpu (preferred for gokvm testing):**
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

**make run (fallback, -c 2, slower):**
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

## Kernels

| Kernel | Version | Source | Boot time | Notes |
|---|---|---|---|---|
| `bzImage` | 5.14.3 | bobuhiro11/bins | ~minutes | Heavy lockdep; use with `-c 2` |
| `kernel_cpu` | 6.0.0 | u-root/cpu repo | ~35s | Clean, NR_CPUS=1; use with `-c 1` |

**Preferred**: `kernel_cpu` from https://github.com/u-root/cpu/raw/main/vm/kernel_linux_amd64
- No lockdep testsuite, much cleaner boot
- `make kernel_cpu` downloads it; `make run-cpu` boots with it

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
```

## Architecture Files

| File | Description |
|---|---|
| `vmm/vmm.go` | Runner interface, VMInfo, Config, VMM struct |
| `vmm/vmm_amd64.go` | `amd64VMM` — full impl; `Info()` returns CPUID; `Close()` calls machine.Close() |
| `vmm/vmm_arm64.go` | `arm64Runner` stub |
| `vmm/vmm_riscv64.go` | `riscv64Runner` stub |

## machine.Close() — tgkill SIGHUP (WORKING)

`Machine.tids []int32` tracks each vCPU goroutine's OS thread ID.
`RunInfiniteLoop` stores `syscall.Gettid()` after `LockOSThread`.
`machine.Close()` sends `SIGHUP` to each tid via `syscall.Tgkill(pid, tid, syscall.SIGHUP)`.
`kvm.Run()` returns `EINTR` → nil, then `isStopped()` → `ErrMachineStopped`.
Serial goroutine breaks out of loop → calls `v.Close()` → chain above → clean exit. ✓

## Exit Sequence

1. User types `^A^X` or `^A^Z` (save+exit)
2. `serial.Start` breaks out of its loop
3. Serial goroutine calls `v.Close()`
4. `amd64VMM.Close()` → `machine.Close()`
5. Sets `stopped=1`, `ImmediateExit=1` on all RunData
6. `tgkill(SIGHUP)` to each vCPU thread tid
7. Each vCPU's `kvm.Run` returns EINTR → `isStopped()` → `ErrMachineStopped`
8. All goroutines return → `g.Wait()` unblocks → process exits

## Makefile Targets

```
make run           # ./gokvm boot -c 2 -i ./initrd (bzImage)
make run-cpu       # ./gokvm boot -c 1 -k ./kernel_cpu -i ./initrd
make qemu          # qemu-system-x86_64 with bzImage
make kernel_cpu    # download u-root/cpu kernel
make build-arm64   # GOARCH=arm64 go build ./...
make build-riscv64 # GOARCH=riscv64 go build ./...
make build-otherarch # both
```

## Save/Restore (paused)

- `^A^Z` triggers save but `kvm.GetRegs(fd)` deadlocks with running vCPU on same fd
- `Save()` sets `stopped=1` + `ImmediateExit=1` before `GetRegs` — race still possible
- Root fix needed: pause vCPUs before reading registers, or use KVM snapshot API
- Low priority — multi-arch work takes precedence

## Session State Checklist

- [x] Interface refactor
- [x] Resumable state — ^A^Z save, -R resume (GetRegs deadlock paused)
- [x] Boot test protocol — expect-based
- [x] Serial input fix
- [x] Boot time — `kunit.enable=0`; `kernel_cpu` preferred
- [x] Architecture factoring — `_amd64.go` files; no build tags
- [x] Runner interface — Init/Setup/Boot/Close/Info
- [x] VMInfo struct — arch-neutral with CPUID on amd64
- [x] Close() — tgkill SIGHUP to vCPU threads (WORKING)
- [x] arm64 stub — Close()/Info()
- [x] riscv64 stub — Close()/Info()
- [x] ^A^X / ^A^Z — serial breaks, calls Close(), clean exit
- [x] kernel_cpu — Linux 6.0 from u-root/cpu; make run-cpu
- [ ] Save/restore deadlock fix
- [ ] ppc64le/s390x stubs (trivial, same pattern)

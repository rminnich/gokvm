# Agent: Go Systems Architect

## Current Objective

Refactoring an amd64-exclusive project to use standard library interfaces, improving modularity and testability without changing runtime behavior.

## Context

- **Project**: gokvm
- **Architecture**: amd64 primary; arm64 build-time verified
- **Repo branch**: main
- **HEAD commit**: `5564fc2` arch: factor amd64 deps into _amd64.go files; vmm uses Runner interface; arm64 build target

## Coding Conventions

- No line wrapping unless line exceeds 256 characters
- Exception: map literals and struct slice literals may be wrapped freely
- After each change: run `make run` and `make qemu` boot tests, then commit
- Commit messages: concise, no fluff or praise
- **Architecture-specific code**: use `runtime.GOARCH` checks at runtime where possible; else use `_GOARCH.go` file naming (e.g. `_amd64.go`). No `//go:build` tags.

## Boot Test Protocol

Both tests must pass before committing. 30s timeout is sufficient. KVM on this machine is flaky — retry up to 3 times on failure.

**make qemu:**
```
expect -c '
set timeout 30
spawn qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm --append "root=/dev/ram rw console=ttyS0 rdinit=/init"
expect "Setting console log level"
sleep 1
send "echo booted\r"
expect {
    "booted" { puts "\nPASS: make qemu"; exit 0 }
    timeout  { puts "\nFAIL: make qemu"; exit 1 }
}
'
```

**make run (./gokvm boot -c 2):**
```
expect -c '
set timeout 30
spawn ./gokvm boot -c 2 -i ./initrd
expect "Setting console log level"
sleep 3
send "echo booted\r"
expect {
    "booted" { puts "\nPASS: make run"; exit 0 }
    timeout  { puts "\nFAIL: make run"; exit 1 }
}
'
```

## Architecture Refactor — What Was Done

### Strategy
- `runtime.GOARCH` check at runtime for functions that compile everywhere but only work on amd64 (e.g. `kvm.IRQLineStatus`, `probe.KVMCapabilities`, `probe.CPUID`)
- `_amd64.go` file naming for code that cannot compile on non-amd64 (x86asm imports, KVM x86 ioctls, x86 register structs, assembly)
- No `//go:build` tags

### Files renamed to `_amd64.go`
- `kvm/registers.go`, `cpuid.go`, `msr.go`, `mce.go`, `apic.go`, `debug_ctl.go`
- `kvm/irq.go` → `irq_amd64.go` (content); `IRQLineStatus` moved to new `kvm/irq.go` with runtime check
- `kvm/kvm.go` split: x86 consts/funcs → `kvm_amd64.go`; `memory.go` split → `memory_amd64.go`
- `kvm/kvm_test.go` → `kvm/kvm_amd64_test.go`
- `machine/machine.go` split: arch-neutral scaffolding stays; x86 constants/New/init*/Load*/RunOnce/VCPU → `machine_amd64.go`
- `machine/state.go` → `machine/state_amd64.go`
- `pvh/`, `bootparam/`, `ebda/`, `cpuid/`: all files renamed to `_amd64.go`
- `probe/capabilities.go` → `capabilities_impl_amd64.go`; `probe/cpuid.go` → `cpuid_impl_amd64.go`

### vmm package — Runner interface
- `vmm.go`: defines `Runner` interface (`Init`, `Setup`, `Boot`), `Config`, `VMM` struct embedding `Runner`
- `vmm_amd64.go`: `amd64VMM` implements `Runner`; `New()` returns `*VMM` backed by `amd64VMM`
- `vmm_arm64.go`: `stubRunner` returns "not supported on this architecture" for all methods

### Makefile
- `make build-arm64`: `GOARCH=arm64 go build ./...`

## Key Findings (this session)

- `make run` uses `-c 2`; `-c 4` causes workqueue lockups
- `kunit.enable=0` in default kernel cmdline cuts boot time to ~6s
- Serial input works via LSR polling; IER=0x5 set when userspace opens `/dev/ttyS0`
- Duplicate `SingleStep` call after `SetRawMode` in vmm.Boot was a race — removed
- `serial.Start()` takes a `save func() error` 4th parameter for ^A^Z triggered save
- KVM on this machine is flaky — retry tests on failure

## Session State Checklist

- [x] Interface refactor — Replace concrete types with standard library interfaces
- [x] Resumable state implementation — Save triggered by SIGHUP and ^A^Z; resume via `-R <file>`
- [x] Boot test protocol — expect-based, 30s timeout, `echo booted` → `booted`
- [x] Serial input fix — removed duplicate SingleStep race; confirmed input works with expect
- [x] Boot time improvement — `kunit.enable=0` in default cmdline
- [x] Architecture factoring — `_amd64.go` file naming; `Runner` interface in vmm; arm64 build verified
- [ ] Next: TBD

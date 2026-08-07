# Agent: Go Systems Architect

## Current Objective

Fix save/restore: `^A^Z` save blocks in `kvm.GetRegs` because vCPU goroutines hold the vcpu fds in `kvm.Run`.

## Context

- **Project**: gokvm
- **Architecture**: amd64 primary; arm64 build-time verified
- **Repo branch**: main
- **HEAD commit**: `2352c86` serial: ^AX exits via os.Exit; ^AZ saves synchronously then exits; save sets stopped+ImmediateExit before GetRegs

## Coding Conventions

- No line wrapping unless line exceeds 256 characters
- Exception: map literals and struct slice literals may be wrapped freely
- After each change: run `make run` and `make qemu` boot tests, then commit
- Commit messages: concise, no fluff or praise
- **Architecture-specific code**: `runtime.GOARCH` at runtime where possible; `_GOARCH.go` file naming otherwise. No `//go:build` tags.
- **agents.md always in its own separate commit**

## Boot Test Protocol

Both tests must pass before committing. 30s timeout. KVM on this machine is flaky — retry up to 3 times.

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

**make run:**
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

**save/restore test:**
```
expect scripts/save-restore-test.expect 2>/dev/null | grep -E 'PASS|FAIL|==='
```

## Current Problem: Save blocks in kvm.GetRegs

**Root cause**: `machine.Save()` calls `kvm.GetRegs(fd)` and `kvm.GetSregs(fd)` on vcpu fds. Those same fds are held by vCPU goroutines blocked in `kvm.Run` ioctl (via `LockOSThread`). Both are ioctls on the same fd — the kernel serializes them, so `GetRegs` blocks until `kvm.Run` returns.

**What was tried**:
- Setting `ImmediateExit=1` on RunData regions — `kvm.Run` should return but there's a race: goroutine re-enters `kvm.Run` before `GetRegs` can run
- Setting `stopped=1` + `ImmediateExit=1` — same race
- Closing vcpu fds — causes `kvm.Run` to return with error but breaks subsequent ioctls
- SIGHUP/SIGUSR1 to process group — signal delivery unreliable with LockOSThread goroutines

**Evidence**: `Saving state to /tmp/...` log line appears (save started), but process never exits and no file is created within 90s. `kvm.GetRegs` is blocking.

**Possible fixes to investigate**:
1. Use `KVM_IMMEDIATE_EXIT` ioctl (separate from the RunData field) to force vCPU exit — but this IS `ImmediateExit` in RunData
2. Use `tgkill(pid, tid, SIGURG)` to interrupt each specific OS thread — requires tracking tids of each vCPU goroutine at startup (store in Machine struct)
3. Don't save registers at all — resume from RAM state only, re-init registers on resume
4. Use KVM's `KVM_GET_VCPU_EVENTS` which may not block, then get registers after
5. Save in a separate process that ptrace-stops the vCPU threads first

**Simplest viable fix**: track each vCPU goroutine's OS thread ID (tid) via `syscall.Gettid()` at the start of `RunInfiniteLoop`, store them, then send `SIGURG` (which Go uses internally and handles safely) to each tid via `tgkill` to interrupt the `kvm.Run` syscall with `EINTR`.

**`SIGURG` note**: Go's runtime uses SIGURG for goroutine preemption. Sending it to a specific thread tid should cause the `kvm.Run` ioctl to return `EINTR`, then `kvm.Run()` returns nil (EINTR is handled), then `isStopped()` returns true, goroutine exits loop.

## Current serial.go exit logic

```go
// ^A^X: restore tty and exit immediately
if before == 0x1 && b == 'x' {
    restoreMode()
    os.Exit(0)          // clean exit, no deadlock
}

// ^A^Z: save synchronously then exit
if before == 0x1 && b == 0x1a {
    if err := save(); err != nil {  // BLOCKS on kvm.GetRegs
        log.Printf("save: %v", err)
    }
    restoreMode()
    os.Exit(0)
}
```

## Key Findings

- `make run` uses `-c 2`; `-c 4` causes workqueue lockups
- `kunit.enable=0` in default kernel cmdline cuts boot time to ~6s
- Serial input works via LSR polling; IER=0x5 when userspace opens `/dev/ttyS0`
- `^A^X` now exits cleanly via `os.Exit(0)` in `serial.Start`
- Host has Linux 7.0 kernel at `/home/rminnich/badgokvm/vmlinuz` (root-only, 17M) — may boot faster and more stably than the test 5.14.3 bzImage
- gob encoding 1G takes ~640ms — fast enough, not the bottleneck

## Session State Checklist

- [x] Interface refactor
- [x] Resumable state — Save via ^A^Z; resume via `-R <file>`
- [x] Boot test protocol — expect-based, 30s timeout
- [x] Serial input fix
- [x] Boot time improvement — `kunit.enable=0`
- [x] Architecture factoring — `_amd64.go`; `Runner` interface; arm64 build
- [x] Clean exit on ^A^X — DONE via `os.Exit(0)` in serial.Start
- [ ] Save/restore test — BLOCKED: kvm.GetRegs deadlocks with running vCPUs
- [ ] Better kernel — Linux 7.0 available but root-only; investigate

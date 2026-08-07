# Agent: Go Systems Architect

## Current Objective

Fix clean VM exit when ^A^X is pressed, then get save/restore test working.

## Context

- **Project**: gokvm
- **Architecture**: amd64 primary; arm64 build-time verified
- **Repo branch**: main
- **HEAD commit**: `22c8578` machine: close vcpu fds on Close() to unblock kvm.Run; serial exit calls Close()

## Coding Conventions

- No line wrapping unless line exceeds 256 characters
- Exception: map literals and struct slice literals may be wrapped freely
- After each change: run `make run` and `make qemu` boot tests, then commit
- Commit messages: concise, no fluff or praise
- **Architecture-specific code**: use `runtime.GOARCH` checks at runtime where possible; else use `_GOARCH.go` file naming. No `//go:build` tags.

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

**clean exit test:**
```
expect -c '
set timeout 30
spawn ./gokvm boot -c 2 -i ./initrd
expect "Setting console log level"
sleep 3
send "echo booted\r"
expect "booted"
send "\x01x"
expect {
    "All cpus done" { puts "PASS: clean exit"; exit 0 }
    eof             { puts "PASS: clean exit (eof)"; exit 0 }
    timeout         { puts "FAIL: did not exit"; exit 1 }
}
'
```

## Current Problem: ^A^X does not exit cleanly

**Symptom**: after `^A^X`, serial goroutine exits and calls `m.Close()`, but `g.Wait()` in `vmm.Boot()` never returns because vCPU goroutines are stuck in `kvm.Run` ioctl.

**What we know**:
- `Close()` sets `m.stopped=1` and `ImmediateExit=1` on all RunData regions
- `ImmediateExit=1` alone does not wake up the blocked `kvm.Run` ioctl
- SIGHUP/SIGUSR1 sent to process/group doesn't work reliably due to Go signal delivery issues with LockOSThread goroutines
- Closing the vcpu fds in `Close()` was tried (current code) — result unknown, disconnected before test completed

**Current `Close()` in machine/machine.go:**
```go
func (m *Machine) Close() error {
    atomic.StoreUint32(&m.stopped, 1)
    for _, r := range m.runs {
        r.ImmediateExit = 1
    }
    // Close all vCPU fds to unblock kvm.Run
    for _, fd := range m.vcpuFds {
        syscall.Close(int(fd))
    }
    for _, d := range m.pci.Devices {
        if c, ok := d.(io.Closer); ok {
            c.Close()
        }
    }
    return nil
}
```

**Next step**: test if closing vcpu fds works. If not, try closing vmFd or kvmFd. Also consider using `tgkill` to send signal to each specific OS thread tid.

**Save/restore test** (`scripts/save-restore-test.expect`): deferred until exit is fixed. The test uses `^A^Z` to trigger save, polls for state file stability, exits via `^A^X`, then restarts with `-R`.

## Architecture Refactor — What Was Done

### Strategy
- `runtime.GOARCH` check at runtime for functions that compile everywhere
- `_amd64.go` file naming for code that cannot compile on non-amd64
- No `//go:build` tags

### vmm package — Runner interface
- `vmm.go`: defines `Runner` interface (`Init`, `Setup`, `Boot`), `Config`, `VMM` struct
- `vmm_amd64.go`: `amd64VMM` implements `Runner`; `Boot()` calls `m.Close()` when serial exits
- `vmm_arm64.go`: `stubRunner` returns "not supported on this architecture"

## Key Findings

- `make run` uses `-c 2`; `-c 4` causes workqueue lockups
- `kunit.enable=0` in default kernel cmdline cuts boot time to ~6s
- Serial input works via LSR polling; IER=0x5 when userspace opens `/dev/ttyS0`
- Duplicate `SingleStep` race after `SetRawMode` was removed
- SIGHUP-based save deferred — signal delivery unreliable with LockOSThread goroutines
- Save via `^A^Z` (in-band serial) works when expect drives pty

## Session State Checklist

- [x] Interface refactor
- [x] Resumable state — Save via ^A^Z; resume via `-R <file>`
- [x] Boot test protocol — expect-based, 30s timeout
- [x] Serial input fix
- [x] Boot time improvement — `kunit.enable=0`
- [x] Architecture factoring — `_amd64.go`; `Runner` interface; arm64 build
- [ ] Clean exit on ^A^X — IN PROGRESS (closing vcpu fds, result unknown)
- [ ] Save/restore test — blocked on clean exit

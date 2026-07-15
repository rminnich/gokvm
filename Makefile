export GOPATH := $(shell go env GOPATH)
export PATH := $(GOPATH)/bin:$(PATH)

# GOLANGCI_LINT_VERSION is the pinned golangci-lint release (see the
# golangci-lint-tool target below for why it's pinned). Bump it
# deliberately, and re-check .golangci.yaml's disable list against
# https://golangci-lint.run/product/roadmap/#linter-deprecation-cycle
# when you do.
GOLANGCI_LINT_VERSION := v1.64.8

$(GOPATH)/bin/stringer:
	go install golang.org/x/tools/cmd/stringer@latest

# golangci-lint-tool installs the pinned golangci-lint version if it
# isn't already installed at that exact version. Unlike a plain file
# rule on $(GOPATH)/bin/golangci-lint, this re-installs whenever a
# *different* version is already present (e.g. a stale binary from a
# previous @latest install, or one installed manually/by another tool),
# so `make golangci` behaves identically on every machine instead of
# silently reusing whatever happened to already be on PATH.
#
# golangci-lint's own go.mod pins an older toolchain via a "toolchain"
# directive; GOTOOLCHAIN=go1.26.0 forces `go install` to build it with
# the same Go version this project requires (see go.mod), since
# golangci-lint refuses to run against a project whose "go" directive
# is newer than the Go version it was built with.
.PHONY: golangci-lint-tool
golangci-lint-tool:
	@installed=$$($(GOPATH)/bin/golangci-lint version 2>/dev/null | sed -n 's/.*version \(v[0-9][0-9.]*\).*/\1/p'); \
	if [ "$$installed" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "installing golangci-lint $(GOLANGCI_LINT_VERSION) (found: $${installed:-none})"; \
		GOTOOLCHAIN=go1.26.0 go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

$(GOPATH)/src/github.com/u-root/u-root:
	git clone --depth=1 --branch v0.13.1 \
	  https://github.com/u-root/u-root.git $@

$(GOPATH)/bin/u-root: $(GOPATH)/src/github.com/u-root/u-root
	cd $(GOPATH)/src/github.com/u-root/u-root \
	  && go install .

gokvm: $(wildcard *.go) $(wildcard */*.go)
	$(MAKE) generate
	go build .

vda.img:
	$(eval dir = $(shell mktemp -d))
	echo "index.html: this message is from /dev/vda in guest" > ${dir}/index.html
	genext2fs -b 1024 -d ${dir} $@
	file $@

virt.dtb: $(wildcard dtb/*.go) $(wildcard cmd/dtbgen/*.go)
	go run ./cmd/dtbgen -o $@

# checkbinaries runs which on all the commands we want to include.
# Be sure to keep it up to date if you add new commands to the initrd
# rule below.
checkbinaries:
	@which ethtool
	@which lspci
	@which lsblk
	@which hexdump
	@which mount
	@which bash
	@which nohup
	@which clear
	@which tic
	@which awk
	@which grep
	@which cut

initrd: checkbinaries ./scripts/get_initrd.bash .bashrc \
  $(GOPATH)/bin/u-root \
  $(GOPATH)/src/github.com/u-root/u-root
	./scripts/get_initrd.bash

bzImage vmlinux: linux.config ./scripts/get_kernel.bash
	./scripts/get_kernel.bash

bzImage_PVH vmlinux_PVH CLOUDHV.fd: linux_pvh.config ./scripts/get_kernel.bash
	./scripts/get_kernel.bash \
		bzImage_PVH \
		vmlinux_PVH \
		linux_pvh.config

.PHONY: run
run: initrd bzImage
	$(MAKE) generate
	go run . boot -c 4 -i "./initrd"

.PHONY: dtb
dtb: virt.dtb

.PHONY: runpvh
runpvh: initrd vmlinux_PVH
	$(MAKE) generate
	go run . boot -c 4 -k "./vmlinuz_PVH" -i "./initrd"

.PHONY: run-system-kernel
run-system-kernel:
	$(MAKE) generate
	# Implemented based on fedora's default path.
	# Other distributions need to be considered.
	go run . boot -k $(shell ls -t /boot/vmlinuz*.x86_64 | head -n 1) \
		-p "console=ttyS0 pci=off earlyprintk=serial nokaslr rdinit=/bin/sh" \
		-i $(shell ls -t /boot/initramfs*.x86_64.img | head -n 1)

.PHONY: generate
generate: $(GOPATH)/bin/stringer
	go generate ./...

.PHONY: golangci
golangci: golangci-lint-tool
	$(MAKE) generate
	GOOS=linux GOARCH=amd64 golangci-lint run ./...
	GOOS=linux GOARCH=arm64 golangci-lint run ./...

.PHONY: test
test: bzImage vmlinux vmlinux_PVH initrd vda.img CLOUDHV.fd
	$(MAKE) generate
	$(MAKE) golangci
	unshare --user --net --map-root-user go test -timeout 30m -coverprofile c.out ./...
	go mod tidy && git diff --no-patch --exit-code go.sum

.PHONY: clean
clean:
	rm -rf ./gokvm bzImage* vmlinux* CLOUDHV.fd _linux *_string.go virt.dtb

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/init" --enable-kvm

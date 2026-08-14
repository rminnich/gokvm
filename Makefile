GOLANGCI_LINT_VERSION = v1.54.2

export GOPATH := $(shell go env GOPATH)
export PATH := $(GOPATH)/bin:$(PATH)

$(GOPATH)/bin/stringer:
	go install golang.org/x/tools/cmd/stringer@latest

$(GOPATH)/src/github.com/u-root/u-root:
	git clone --depth=1 --branch v0.16.0 \
	  https://github.com/u-root/u-root.git $@

$(GOPATH)/bin/u-root: $(GOPATH)/src/github.com/u-root/u-root
	cd $(GOPATH)/src/github.com/u-root/u-root \
	  && go install .

gokvm: $(wildcard *.go) $(wildcard */*.go)
	$(MAKE) generate
	go build .

golangci-lint:
	curl --retry 5 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b . $(GOLANGCI_LINT_VERSION)

vda.img:
	$(eval dir = $(shell mktemp -d))
	echo "index.html: this message is from /dev/vda in guest" > ${dir}/index.html
	genext2fs -b 1024 -d ${dir} $@
	file $@

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
	@which iperf3

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
	go run . boot -c 2 -i "./initrd"

kernel_cpu:
	curl -s -O -L -C - --retry 5 \
		https://github.com/u-root/cpu/raw/main/vm/kernel_linux_amd64
	mv kernel_linux_amd64 kernel_cpu

.PHONY: run-cpu
run-cpu: initrd kernel_cpu
	$(MAKE) generate
	go run . boot -c 1 -k ./kernel_cpu -i "./initrd"

.PHONY: runpvh
runpvh: initrd vmlinux_PVH
	$(MAKE) generate
	go run . boot -c 2 -k "./vmlinuz_PVH" -i "./initrd"

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
golangci: golangci-lint
	$(MAKE) generate
	echo ./golangci-lint run ./...

.PHONY: test
test: bzImage vmlinux vmlinux_PVH initrd vda.img CLOUDHV.fd
	$(MAKE) generate
	$(MAKE) golangci
	go test -v -timeout 30m -coverprofile c.out ./...
	go mod tidy && git diff --no-patch --exit-code go.sum

.PHONY: build-arm64
build-arm64:
	GOARCH=arm64 go build ./...

.PHONY: build-riscv64
build-riscv64:
	GOARCH=riscv64 go build ./...

.PHONY: build-otherarch
build-otherarch: build-arm64 build-riscv64

.PHONY: test-save-restore
test-save-restore: gokvm initrd kernel_cpu
	expect scripts/save-restore-test.expect

# Kernel command line used by the net-test targets.
NETTEST_CMDLINE := console=ttyS0 earlyprintk=serial noapic noacpi pci=conf1 \
	reboot=k panic=1 i8042.direct=1 i8042.dumbkbd=1 i8042.nopnp=1 \
	i8042.noaux=1 mitigations=off pci=realloc=off virtio_pci.force_legacy=1 \
	rdinit=/init init=/init kunit.enable=0 gokvm.ipv4_addr=192.168.20.1/24

.PHONY: net-test
net-test: gokvm initrd kernel_cpu
	./scripts/net-test.sh gokvm \
		./gokvm boot -c 1 -k ./kernel_cpu -i ./initrd \
		-t tap0 -p "$(NETTEST_CMDLINE)"

.PHONY: net-test-lkvm
net-test-lkvm: initrd kernel_cpu
	./scripts/net-test.sh lkvm \
		~/bin/lkvm run -k ./kernel_cpu -i ./initrd \
		-m 1024 -c 1 -n mode=tap,tapif=tap0 -p "$(NETTEST_CMDLINE)"

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint bzImage* vmlinux* CLOUDHV.fd _linux *_string.go

.PHONY: lkvm
lkvm: initrd kernel_cpu
	~/bin/lkvm run -k ./kernel_cpu -i ./initrd

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/init" --enable-kvm

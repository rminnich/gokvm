#!/bin/sh
# Network performance test for a KVM VMM.
#
# Usage: net-test.sh <label> <vmm command...>
#
# Sets up a private network namespace with a tap interface, then runs
# scripts/net-test.expect inside it. Running expect inside the netns lets
# host-side iperf3 clients reach the guest through the tap.

set -e

label=$1
shift

unshare --user --net --map-root-user sh -c '
    ip link set lo up
    ip tuntap add tap0 mode tap
    ip addr add 192.168.20.2/24 dev tap0
    ip link set tap0 up
    exec expect "$0" "$@"
' scripts/net-test.expect "$label" "$@"

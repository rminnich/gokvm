package kvm

const (
	kvmGetSupportedCPUID = 0x05

	kvmGetEmulatedCPUID       = 0x09
	kvmGetMSRIndexList        = 0x02
	kvmGetMSRFeatureIndexList = 0x0A

	kvmSetTSSAddr         = 0x47
	kvmSetIdentityMapAddr = 0x48

	kvmCreateIRQChip = 0x60
	kvmGetIRQChip    = 0x62
	kvmSetIRQChip    = 0x63

	kvmReinjectControl = 0x71
	kvmCreatePIT2      = 0x77

	kvmGetRegs  = 0x81
	kvmSetRegs  = 0x82
	kvmGetSregs = 0x83
	kvmSetSregs = 0x84

	kvmInterrupt = 0x86

	kvmGetMSRS = 0x88
	kvmSetMSRS = 0x89

	kvmGetLAPIC = 0x8e
	kvmSetLAPIC = 0x8f

	kvmSetCPUID2          = 0x90
	kvmGetCPUID2          = 0x91
	kvmTRPAccessReporting = 0x92

	kvmX86SetupMCE           = 0x9C
	kvmX86GetMCECapSupported = 0x9D

	kvmGetPIT2 = 0x9F
	kvmSetPIT2 = 0xA0

	kvmGetDebugRegs = 0xA1
	kvmSetDebugRegs = 0xA2

	kvmSetTSCKHz = 0xA2
	kvmGetTSCKHz = 0xA3

	kvmGetXCRS = 0xA6
	kvmSetXCRS = 0xA7

	kvmSMI = 0xB7

	kvmGetSRegs2 = 0xCC
	kvmSetSRegs2 = 0xCD
)

// SetTSCKHz sets the TSC frequency in kHz (x86 only).
func SetTSCKHz(vcpuFd uintptr, freq uint64) error {
	_, err := Ioctl(vcpuFd, IIO(kvmSetTSCKHz), uintptr(freq))

	return err
}

// GetTSCKHz gets the TSC frequency in kHz (x86 only).
func GetTSCKHz(vcpuFd uintptr) (uint64, error) {
	ret, err := Ioctl(vcpuFd, IIO(kvmGetTSCKHz), 0)
	if err != nil {
		return 0, err
	}

	return uint64(ret), nil
}

// PutSMI queues an SMI on the thread's vcpu (x86 only).
func PutSMI(vcpuFd uintptr) error {
	_, err := Ioctl(vcpuFd, IIO(kvmSMI), 0)

	return err
}

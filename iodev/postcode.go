package iodev

import "fmt"

type PostCode struct{}

// postCodeIOPort/postCodeSize are this device's IO port address and size.
const (
	postCodeIOPort = 0x80
	postCodeSize   = 0x1
)

func (p *PostCode) Read(port uint64, data []byte) error {
	return nil
}

func (p *PostCode) Write(port uint64, data []byte) error {
	if len(data) != 1 {
		return errDataLenInvalid
	}

	if data[0] == '\000' {
		fmt.Printf("\r\n")
	} else {
		fmt.Printf("%c", data[0])
	}

	return nil
}

func (p *PostCode) IOPort() uint64 {
	return postCodeIOPort
}

func (p *PostCode) Size() uint64 {
	return postCodeSize
}

//go:build linux

package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	loopSetFd       = 0x4C00
	loopClrFd       = 0x4C01
	loopSetStatus64 = 0x4C04
	loopGetStatus64 = 0x4C05
	loopCtlGetFree  = 0x4C82

	loFlagsReadOnly  = 1
	loFlagsAutoClear = 4
)

type loopInfo64 struct {
	Device         uint64
	Inode          uint64
	Rdevice        uint64
	Offset         uint64
	SizeLimit      uint64
	Number         uint32
	EncryptType    uint32
	EncryptKeySize uint32
	Flags          uint32
	FileName       [64]byte
	CryptName      [64]byte
	EncryptKey     [32]byte
	Init           [2]uint64
}

// Create attaches an image file to a free loop device and returns the device path.
func (m *Manager) Create(imagePath string, readOnly bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctlFd, err := os.OpenFile("/dev/loop-control", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("open loop-control: %w", err)
	}
	defer ctlFd.Close()

	devNum, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ctlFd.Fd(), loopCtlGetFree, 0)
	if errno != 0 {
		return "", fmt.Errorf("LOOP_CTL_GET_FREE: %w", errno)
	}

	devPath := fmt.Sprintf("/dev/loop%d", devNum)

	loopFd, err := os.OpenFile(devPath, os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", devPath, err)
	}
	defer loopFd.Close()

	flags := os.O_RDWR
	if readOnly {
		flags = os.O_RDONLY
	}
	imgFd, err := os.OpenFile(imagePath, flags, 0)
	if err != nil {
		return "", fmt.Errorf("open image %s: %w", imagePath, err)
	}
	defer imgFd.Close()

	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, loopFd.Fd(), loopSetFd, imgFd.Fd())
	if errno != 0 {
		return "", fmt.Errorf("LOOP_SET_FD: %w", errno)
	}

	var info loopInfo64
	if readOnly {
		info.Flags |= loFlagsReadOnly
	}
	info.Flags |= loFlagsAutoClear
	copy(info.FileName[:], imagePath)

	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, loopFd.Fd(), loopSetStatus64,
		uintptr(unsafe.Pointer(&info)))
	if errno != 0 {
		syscall.Syscall(syscall.SYS_IOCTL, loopFd.Fd(), loopClrFd, 0)
		return "", fmt.Errorf("LOOP_SET_STATUS64: %w", errno)
	}

	return devPath, nil
}

// Detach removes the loop device association.
func (m *Manager) Detach(devPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fd, err := os.OpenFile(devPath, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", devPath, err)
	}
	defer fd.Close()

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), loopClrFd, 0)
	if errno != 0 {
		return fmt.Errorf("LOOP_CLR_FD on %s: %w", devPath, errno)
	}
	return nil
}

func (m *Manager) findByFileImpl(imagePath string) ([]string, error) {
	entries, err := filepath.Glob("/dev/loop[0-9]*")
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, dev := range entries {
		base := filepath.Base(dev)
		if strings.ContainsRune(base[4:], 'p') {
			continue
		}

		backing, err := m.getBackingFile(dev)
		if err != nil {
			continue
		}
		if backing == imagePath {
			matches = append(matches, dev)
		}
	}
	return matches, nil
}

func (m *Manager) getBackingFile(devPath string) (string, error) {
	fd, err := os.OpenFile(devPath, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer fd.Close()

	var info loopInfo64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), loopGetStatus64,
		uintptr(unsafe.Pointer(&info)))
	if errno != 0 {
		return "", errno
	}

	name := string(info.FileName[:])
	if idx := strings.IndexByte(name, 0); idx >= 0 {
		name = name[:idx]
	}
	return name, nil
}

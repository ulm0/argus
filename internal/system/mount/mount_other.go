//go:build !linux

package mount

import (
	"fmt"
	"os/exec"
)

func (m *Manager) mountLoopReadOnlyUserImpl(source, target, fsType string, uid, gid int) error {
	return fmt.Errorf("mount not supported on this platform (Linux only)")
}

func (m *Manager) mountImpl(source, target, fsType string, readOnly bool) error {
	return fmt.Errorf("mount not supported on this platform (Linux only)")
}

func (m *Manager) unmountImpl(target string, lazy bool) error {
	cmd := exec.Command("umount", target)
	return cmd.Run()
}

func syncFS() {
	_ = exec.Command("sync").Run()
}

package samba

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

type Manager struct {
	cfg *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

// CloseSambaShare forces Samba to close and reload a specific share for immediate file visibility.
func (m *Manager) CloseSambaShare(shareName string) error {
	cmds := [][]string{
		{"sudo", "-n", "smbcontrol", "all", "close-share", shareName},
		{"sudo", "-n", "smbcontrol", "all", "reload-config"},
	}
	for _, args := range cmds {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			logger.L.WithError(err).WithField("cmd", strings.Join(args, " ")).Warn("smbcontrol failed")
		}
	}
	return nil
}

// RestartSambaServices restarts smbd and nmbd.
func (m *Manager) RestartSambaServices() error {
	for _, svc := range []string{"smbd", "nmbd"} {
		if err := exec.Command("sudo", "-n", "systemctl", "restart", svc).Run(); err != nil {
			return fmt.Errorf("restart %s: %w", svc, err)
		}
	}
	return nil
}

// GenerateConfig writes a Samba configuration file with the USB partition shares.
func (m *Manager) GenerateConfig() error {
	const smbTmpl = `[global]
   workgroup = WORKGROUP
   server string = Argus
   security = user
   map to guest = never
   server role = standalone server
   log file = /var/log/samba/log.%m
   max log size = 50
   dns proxy = no
   server min protocol = SMB2

[{{.Part1Label}}]
   comment = TeslaCam Drive
   path = {{.MountDir}}/part1
   browseable = yes
   read only = no
   valid users = {{.User}}
   create mask = 0644
   directory mask = 0755

[{{.Part2Label}}]
   comment = LightShow Drive
   path = {{.MountDir}}/part2
   browseable = yes
   read only = no
   valid users = {{.User}}
   create mask = 0644
   directory mask = 0755
{{if .MusicEnabled}}
[{{.Part3Label}}]
   comment = Music Drive
   path = {{.MountDir}}/part3
   browseable = yes
   read only = no
   valid users = {{.User}}
   create mask = 0644
   directory mask = 0755
{{end}}`

	tmpl, err := template.New("smb").Parse(smbTmpl)
	if err != nil {
		return fmt.Errorf("parse smb template: %w", err)
	}

	f, err := os.Create(m.cfg.System.SambaConf)
	if err != nil {
		return fmt.Errorf("create smb.conf: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, map[string]any{
		"User":         m.cfg.Installation.TargetUser,
		"MountDir":     m.cfg.Installation.MountDir,
		"Part1Label":   "gadget_part1",
		"Part2Label":   "gadget_part2",
		"Part3Label":   "gadget_part3",
		"MusicEnabled": m.cfg.DiskImages.MusicEnabled,
	})
}

// SetPassword sets the Samba password for the target user.
func (m *Manager) SetPassword(password string) error {
	cmd := exec.Command("sudo", "-n", "smbpasswd", "-s", "-a", m.cfg.Installation.TargetUser)
	cmd.Stdin = strings.NewReader(password + "\n" + password + "\n")
	return cmd.Run()
}

// ShareNameForPartition returns the Samba share name for a partition key.
func ShareNameForPartition(partition string) string {
	return "gadget_" + partition
}

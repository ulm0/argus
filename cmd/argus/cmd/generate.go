package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func NewGenerateCmd() *cobra.Command {
	var (
		output string
		force   bool
	)

	c := &cobra.Command{
		Use:   "generate",
		Short: "Generate an initial config.yaml template",
		Long: `Generate a modifiable initial config.yaml file.

This command does not install anything; it only writes the template configuration.
After editing the file, run "sudo argus setup" to install the system.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			targetUser := currentUser()
			content := buildGenerateContent(targetUser)

			resolvedOut, err := resolveGenerateOutputPath(output)
			if err != nil {
				return err
			}

			wrote, err := writeGenerateConfig(resolvedOut, content, force)
			if err != nil {
				return err
			}

			if wrote {
				fmt.Printf("[+] Config generated at %s\n", resolvedOut)
				printGenerateNotice(resolvedOut)
				return nil
			}

			fmt.Printf("[!] Config already exists at %s — not overwritten.\n", resolvedOut)
			fmt.Println("    Use --force to overwrite.")
			return nil
		},
	}

	c.Flags().StringVarP(&output, "output", "o", "", "output path for config.yaml (default: <dir>/config.yaml)")
	c.Flags().BoolVarP(&force, "force", "f", false, "overwrite config.yaml if it already exists")
	return c
}

func resolveGenerateOutputPath(output string) (string, error) {
	if output != "" {
		return expandPath(output), nil
	}

	installDir := setupDefaultDir()
	return filepath.Join(installDir, "config.yaml"), nil
}

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	p = os.ExpandEnv(p)
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}

// buildGenerateContent renders the initial config.yaml template for the given target user.
func buildGenerateContent(targetUser string) string {
	return strings.Replace(defaultConfigYAML, "target_user: pi", "target_user: "+targetUser, 1)
}

func printGenerateNotice(outPath string) {
	fmt.Printf("\n  Edit the file to set your credentials and preferences:\n"+
		"    - samba_password            (network section)\n"+
		"    - offline_ap.ssid / passphrase\n"+
		"    - telegram.bot_token / chat_id (optional)\n"+
		"\n  When ready, run:\n"+
		"    sudo argus setup\n")
}

// writeGenerateConfig writes content to path.
// It is atomic (temp file + rename) and never requires root privileges.
//
// Returns wrote=true when the file was created/overwritten, wrote=false when it already existed and force=false.
func writeGenerateConfig(path, content string, force bool) (wrote bool, err error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		} else if err != nil && !os.IsNotExist(err) {
			return false, err
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, fmt.Errorf("create output directory: %w", err)
	}

	base := filepath.Base(path)
	tmpPrefix := "." + base + ".tmp-"
	f, err := os.CreateTemp(dir, tmpPrefix)
	if err != nil {
		return false, fmt.Errorf("create temp file: %w", err)
	}

	tmpName := f.Name()
	defer func() {
		// If rename succeeded, tmpName no longer exists, and os.Remove becomes a no-op.
		_ = os.Remove(tmpName)
	}()

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, 0644); err != nil {
		return false, fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return false, fmt.Errorf("rename temp file: %w", err)
	}

	return true, nil
}


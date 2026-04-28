//go:build !linux

package watchdog

// ApplyDaemon is a no-op outside Linux.
func ApplyDaemon(enabled bool, timeoutSec int) error {
	return nil
}

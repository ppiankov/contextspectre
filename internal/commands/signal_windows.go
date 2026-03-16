//go:build windows

package commands

import "os"

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

package watchdog

import "syscall"

// syscallSignal converts an int to a syscall.Signal for process liveness checks.
func syscallSignal(sig int) syscall.Signal {
	return syscall.Signal(sig)
}

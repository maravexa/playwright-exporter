//go:build linux

package main

import (
	"os"
	"syscall"
)

// processTreeIsolation creates an unprivileged user namespace and a PID namespace.
// The direct runner becomes PID1: its exit makes the kernel kill remaining
// processes in that namespace. This does not isolate files, network or secrets.
func processTreeIsolation() (*syscall.SysProcAttr, error) {
	return &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: os.Getuid(), HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: os.Getgid(), HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}, nil
}

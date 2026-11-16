//go:build !linux

package main

import (
	"fmt"
	"syscall"
)

func processTreeIsolation() (*syscall.SysProcAttr, error) {
	return nil, fmt.Errorf("process-tree isolation requires Linux")
}

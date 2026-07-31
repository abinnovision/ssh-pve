// Package main is the ssh-pve entry point: a TUI for listing Proxmox VE VMs
// across the whole cluster and shelling into them via SSH.
package main

import (
	"fmt"
	"os"

	"github.com/pneugebala/ssh-pve/tui"
)

func main() {
	if err := tui.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ssh-pve: %v\n", err)
		os.Exit(1)
	}
}

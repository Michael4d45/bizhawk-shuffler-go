//go:build !windows

package client

import "log"

func (c *BizHawkController) checkAndInstallVCRedist() {
	log.Printf("Skipping VC++ redistributable check on non-Windows platform")
}

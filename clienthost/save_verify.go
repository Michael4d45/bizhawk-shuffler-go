package clienthost

import (
	"fmt"
	"os"

	"github.com/michael4d45/bizshuffle/savestate"
)

const clientSaveMaxBytes = 32 << 20

// verifySaveFileBytes checks that data is a valid BizHawk savestate.
func verifySaveFileBytes(data []byte) error {
	result := savestate.VerifyBizHawkSavestate(data, savestate.VerifyOptions{MaxFileBytes: clientSaveMaxBytes})
	if !result.OK {
		return fmt.Errorf("invalid save (%s): %s", result.Code, result.Message)
	}
	return nil
}

// verifySaveFilePath reads and validates a local .state file.
func verifySaveFilePath(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return verifySaveFileBytes(data)
}

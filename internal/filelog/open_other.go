//go:build !windows

package filelog

import "os"

func openLogFile(path string) (*os.File, error) {
	return os.Open(path)
}

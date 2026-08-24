package filelog

import "testing"

func writeFile(t *testing.T, path string, content string) {
	writeLogFile(t, path, content)
}

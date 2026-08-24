package filelog

import (
	"errors"
	"io"
	"os"
)

const directoryReadBatchSize = 128

// visitDirectoryLimited 分批读取目录，避免 os.ReadDir 一次加载超大目录。
// visitor 返回 false 时立即停止；达到 maxEntries 时返回 truncated=true。
func visitDirectoryLimited(
	directoryPath string,
	maxEntries int,
	visitor func(os.DirEntry) bool,
) (visited int, truncated bool, err error) {
	if maxEntries <= 0 {
		return 0, true, nil
	}
	directory, err := os.Open(directoryPath)
	if err != nil {
		return 0, false, err
	}
	defer directory.Close()

	for visited < maxEntries {
		remaining := maxEntries - visited
		batchSize := directoryReadBatchSize
		if remaining < batchSize {
			batchSize = remaining
		}
		entries, readErr := directory.ReadDir(batchSize)
		for _, entry := range entries {
			visited++
			if !visitor(entry) {
				return visited, false, nil
			}
		}
		if errors.Is(readErr, io.EOF) {
			return visited, false, nil
		}
		if readErr != nil {
			return visited, false, readErr
		}
	}
	return visited, true, nil
}

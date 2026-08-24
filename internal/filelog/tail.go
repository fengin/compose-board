package filelog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	tailReadChunkSize = 64 * 1024
	maxTailReadBytes  = 32 * 1024 * 1024
	maxFileLogLine    = 1024 * 1024
)

var ErrLineTooLong = errors.New("日志单行超过 1 MiB")

// ReadTailLines 从当前文件末尾按块读取最近 N 行，并返回读取时的文件末尾偏移。
func ReadTailLines(file *os.File, tail int) ([]string, int64, error) {
	if tail <= 0 {
		tail = 100
	}
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	endOffset := info.Size()
	if endOffset == 0 {
		return []string{}, 0, nil
	}

	var data []byte
	position := endOffset
	for position > 0 && int64(len(data)) < maxTailReadBytes {
		readSize := int64(tailReadChunkSize)
		if readSize > position {
			readSize = position
		}
		remaining := maxTailReadBytes - int64(len(data))
		if readSize > remaining {
			readSize = remaining
		}
		position -= readSize
		chunk := make([]byte, readSize)
		if _, err := file.ReadAt(chunk, position); err != nil && !errors.Is(err, io.EOF) {
			return nil, 0, err
		}
		data = append(chunk, data...)
		if bytes.Count(data, []byte{'\n'}) > tail {
			break
		}
	}

	rawLines := bytes.Split(data, []byte{'\n'})
	if len(rawLines) > 0 && len(rawLines[len(rawLines)-1]) == 0 {
		rawLines = rawLines[:len(rawLines)-1]
	}
	if len(rawLines) > tail {
		rawLines = rawLines[len(rawLines)-tail:]
	}
	lines := make([]string, 0, len(rawLines))
	for _, raw := range rawLines {
		if len(raw) > maxFileLogLine {
			return nil, 0, ErrLineTooLong
		}
		lines = append(lines, strings.TrimSuffix(string(raw), "\r"))
	}
	return lines, endOffset, nil
}

func normalizeTail(value int) (int, error) {
	if value == 0 {
		return 100, nil
	}
	if value < 10 || value > 5000 {
		return 0, fmt.Errorf("tail 必须在 10 到 5000 之间")
	}
	return value, nil
}

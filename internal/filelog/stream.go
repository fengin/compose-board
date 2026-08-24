package filelog

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"time"
)

const (
	fileFollowPollInterval = 500 * time.Millisecond
	fileFollowRetryDelay   = 1200 * time.Millisecond
	fileHeartbeatInterval  = 15 * time.Second
)

// StreamEvent 文件日志跟随事件。
type StreamEvent struct {
	Type  string
	State string
	Line  string
}

// Follow 持续跟随授权普通文件，自动处理暂时缺失、截断和轮转重建。
func (m *Manager) Follow(
	ctx context.Context,
	rootID string,
	relativePath string,
	tail int,
	emit func(StreamEvent) error,
) error {
	root, err := m.getFollowBase(rootID, relativePath)
	if err != nil {
		return err
	}
	tail, err = normalizeTail(tail)
	if err != nil {
		return err
	}

	var current *followedFile
	initialOpen := true
	lastState := ""
	emitState := func(state string) error {
		if state == lastState {
			return nil
		}
		lastState = state
		return emit(StreamEvent{Type: "status", State: state})
	}
	defer func() {
		if current != nil {
			_ = current.file.Close()
		}
	}()

	pollTicker := time.NewTicker(fileFollowPollInterval)
	heartbeatTicker := time.NewTicker(fileHeartbeatInterval)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if current == nil {
			opened, openErr := openFollowedFile(root, relativePath, initialOpen, tail, emit)
			if openErr != nil {
				if errors.Is(openErr, os.ErrNotExist) {
					if err := emitState("waiting"); err != nil {
						return err
					}
					if !waitFileLogRetry(ctx, fileFollowRetryDelay) {
						return ctx.Err()
					}
					continue
				}
				return openErr
			}
			current = opened
			initialOpen = false
			if err := emitState("streaming"); err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeatTicker.C:
			if err := emit(StreamEvent{Type: "heartbeat"}); err != nil {
				return err
			}
		case <-pollTicker.C:
			rotated, readErr := current.readChanges(root, relativePath, emit)
			if readErr != nil {
				if errors.Is(readErr, os.ErrNotExist) {
					if err := emitState("waiting"); err != nil {
						return err
					}
					_ = current.file.Close()
					current = nil
					continue
				}
				return readErr
			}
			if rotated {
				if err := emitState("rotating"); err != nil {
					return err
				}
				_ = current.file.Close()
				current = nil
			}
		}
	}
}

type followedFile struct {
	file    *os.File
	info    os.FileInfo
	offset  int64
	partial []byte
}

func openFollowedFile(
	root *Base,
	relativePath string,
	initial bool,
	tail int,
	emit func(StreamEvent) error,
) (*followedFile, error) {
	file, info, err := secureOpenRegularFile(root, relativePath)
	if err != nil {
		return nil, err
	}
	state := &followedFile{file: file, info: info}
	if initial {
		lines, offset, err := ReadTailLines(file, tail)
		if err != nil {
			file.Close()
			return nil, err
		}
		state.offset = offset
		for _, line := range lines {
			if err := emit(StreamEvent{Type: "line", Line: line}); err != nil {
				file.Close()
				return nil, err
			}
		}
	}
	return state, nil
}

func (f *followedFile) readChanges(
	root *Base,
	relativePath string,
	emit func(StreamEvent) error,
) (bool, error) {
	candidate, info, err := secureOpenRegularFile(root, relativePath)
	if err != nil {
		return false, err
	}
	_ = candidate.Close()
	if !os.SameFile(f.info, info) {
		return true, nil
	}
	if info.Size() < f.offset {
		if _, err := f.file.Seek(0, io.SeekStart); err != nil {
			return false, err
		}
		f.offset = 0
		f.partial = nil
	}
	if info.Size() == f.offset {
		f.info = info
		return false, nil
	}

	if _, err := f.file.Seek(f.offset, io.SeekStart); err != nil {
		return false, err
	}
	buffer := make([]byte, tailReadChunkSize)
	for f.offset < info.Size() {
		limit := info.Size() - f.offset
		if int64(len(buffer)) > limit {
			buffer = buffer[:limit]
		}
		readCount, readErr := f.file.Read(buffer)
		if readCount > 0 {
			f.offset += int64(readCount)
			f.partial = append(f.partial, buffer[:readCount]...)
			if err := f.emitCompleteLines(emit); err != nil {
				return false, err
			}
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return false, readErr
		}
		if readCount == 0 {
			break
		}
	}
	f.info = info
	return false, nil
}

func (f *followedFile) emitCompleteLines(emit func(StreamEvent) error) error {
	for {
		index := bytes.IndexByte(f.partial, '\n')
		if index < 0 {
			if len(f.partial) > maxFileLogLine {
				return ErrLineTooLong
			}
			return nil
		}
		line := f.partial[:index]
		f.partial = f.partial[index+1:]
		if len(line) > maxFileLogLine {
			return ErrLineTooLong
		}
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if err := emit(StreamEvent{Type: "line", Line: string(line)}); err != nil {
			return err
		}
	}
}

func waitFileLogRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

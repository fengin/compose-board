package filelog

import (
	"context"
	"os"
	"path/filepath"
)

// appendArchiveDirectories 在已确定的服务日志目录内部有限发现归档子目录。
// 它不会改变选中目录，也不会越出当前 service 的已匹配路径。
func (m *Manager) appendArchiveDirectories(
	ctx context.Context,
	directories []DirectoryInfo,
	selected DirectoryInfo,
) []DirectoryInfo {
	base, err := m.getBase(selected.BaseID)
	if err != nil {
		return directories
	}
	startPath, err := resolveDirectory(base, selected.Path)
	if err != nil {
		return directories
	}

	known := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		known[directory.BaseID+"\x00"+directory.Path] = struct{}{}
	}
	type pendingDirectory struct {
		path  string
		depth int
	}
	queue := []pendingDirectory{{path: startPath, depth: 0}}
	visited := 0
	for len(queue) > 0 && visited < 500 && ctx.Err() == nil {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= 2 {
			continue
		}
		remaining := 500 - visited
		count, _, _ := visitDirectoryLimited(current.path, remaining, func(entry os.DirEntry) bool {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return true
			}
			childPath := filepath.Join(current.path, entry.Name())
			queue = append(queue, pendingDirectory{path: childPath, depth: current.depth + 1})
			if directoryHasAllowedFiles(childPath, base) {
				relative, ok := relativeWithin(base.Path, childPath)
				key := base.ID + "\x00" + relative
				if ok {
					if _, exists := known[key]; !exists {
						known[key] = struct{}{}
						directories = append(directories, DirectoryInfo{
							BaseID: base.ID, BaseName: base.Name, Path: relative,
							DisplayName: relative, MatchMethod: "archive_directory",
						})
					}
				}
			}
			return true
		})
		visited += count
	}
	return directories
}

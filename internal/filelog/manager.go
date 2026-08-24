// filelog 包实现授权基准目录内的宿主机文件日志发现、映射、读取、跟随和下载。
package filelog

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fengin/composeboard/internal/config"
	"github.com/fengin/composeboard/internal/docker"
	"github.com/fengin/composeboard/internal/service"
)

var (
	baseIDPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	ErrDisabled        = errors.New("文件日志功能未启用")
	ErrBaseNotFound    = errors.New("日志安全基准目录不存在")
	ErrInvalidPath     = errors.New("日志路径不合法")
	ErrFileNotAllowed  = errors.New("文件类型不在授权范围内")
	ErrServiceNotFound = errors.New("Compose 服务不存在")
)

const browsePageSize = 100

// Base 一个启动时固定的宿主机安全基准目录。
// 它只限制可访问范围，不会被自动全量扫描。
type Base struct {
	ID                 string
	Name               string
	Path               string
	FollowExtensions   map[string]struct{}
	DownloadExtensions map[string]struct{}
}

// BaseInfo 文件日志能力和安全基准目录状态。
type BaseInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// DirectoryInfo 当前服务可选择的一个日志目录。
type DirectoryInfo struct {
	BaseID      string `json:"base_id"`
	BaseName    string `json:"base_name"`
	Path        string `json:"path"`
	DisplayName string `json:"display_name"`
	Recommended bool   `json:"recommended"`
	MatchMethod string `json:"match_method,omitempty"`
	Manual      bool   `json:"manual"`
	Score       int    `json:"-"`
}

// FileInfo 授权目录中的日志文件元数据。
type FileInfo struct {
	BaseID       string    `json:"base_id"`
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modified_at"`
	Followable   bool      `json:"followable"`
	Downloadable bool      `json:"downloadable"`
	Archived     bool      `json:"archived"`
}

// ServiceSource 描述服务的自动发现、人工映射和最终选择状态。
type ServiceSource struct {
	Service            string             `json:"service"`
	Mode               string             `json:"mode"` // automatic | manual | unmatched | invalid_manual
	Directories        []DirectoryInfo    `json:"directories"`
	Selected           *DirectoryInfo     `json:"selected,omitempty"`
	Mapping            *ServiceLogMapping `json:"mapping,omitempty"`
	DiscoveryTruncated bool               `json:"discovery_truncated"`
	Reason             string             `json:"reason,omitempty"`
}

// MappingValidation 人工相对路径的安全检测结果。
type MappingValidation struct {
	Valid          bool   `json:"valid"`
	BaseID         string `json:"base_id"`
	BaseName       string `json:"base_name,omitempty"`
	RelativePath   string `json:"relative_path"`
	ResolvedPath   string `json:"resolved_path,omitempty"`
	FileCount      int    `json:"file_count"`
	DirectoryCount int    `json:"directory_count"`
	Error          string `json:"error,omitempty"`
}

// BrowseEntry 安全基准目录下一层目录项。
type BrowseEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// BrowseResult 懒加载目录浏览结果。
type BrowseResult struct {
	BaseID    string        `json:"base_id"`
	Path      string        `json:"path"`
	Parent    string        `json:"parent"`
	Entries   []BrowseEntry `json:"entries"`
	Truncated bool          `json:"truncated"`
}

type discoveryLimits struct {
	maxDepth   int
	maxEntries int
	timeout    time.Duration
	cacheTTL   time.Duration
}

type cachedSource struct {
	signature string
	expiresAt time.Time
	value     ServiceSource
}

// Manager 文件日志领域服务。
type Manager struct {
	enabled        bool
	projectDir     string
	serviceManager *service.ServiceManager
	dockerClient   *docker.Client
	bases          map[string]*Base
	baseOrder      []string
	discovery      discoveryLimits
	mappings       *MappingStore

	cacheMu sync.Mutex
	cache   map[string]cachedSource
}

// NewManager 创建文件日志管理器。无有效安全基准目录时保持禁用。
func NewManager(
	cfg config.FileLogsConfig,
	projectDir string,
	serviceManager *service.ServiceManager,
	dockerClient *docker.Client,
) *Manager {
	manager := &Manager{
		projectDir:     projectDir,
		serviceManager: serviceManager,
		dockerClient:   dockerClient,
		bases:          make(map[string]*Base),
		cache:          make(map[string]cachedSource),
		discovery: discoveryLimits{
			maxDepth:   cfg.Discovery.MaxDepth,
			maxEntries: cfg.Discovery.MaxEntries,
			timeout:    time.Duration(cfg.Discovery.TimeoutMS) * time.Millisecond,
			cacheTTL:   time.Duration(cfg.Discovery.CacheTTLSeconds) * time.Second,
		},
	}
	if !cfg.Enabled {
		return manager
	}

	followExtensions := normalizeExtensions(cfg.FollowExtensions)
	downloadExtensions := normalizeExtensions(cfg.DownloadExtensions)
	for _, raw := range cfg.AllowedBases {
		base, err := normalizeBase(raw, followExtensions, downloadExtensions)
		if err != nil {
			log.Printf("[FILELOG] 忽略无效安全基准目录 id=%s: %v", raw.ID, err)
			continue
		}
		if _, exists := manager.bases[base.ID]; exists {
			log.Printf("[FILELOG] 忽略重复安全基准目录 id=%s", base.ID)
			continue
		}
		manager.bases[base.ID] = base
		manager.baseOrder = append(manager.baseOrder, base.ID)
	}
	manager.enabled = len(manager.bases) > 0
	if manager.enabled {
		manager.mappings = NewMappingStore(projectDir)
	}
	return manager
}

// Enabled 返回是否存在有效安全基准目录。
func (m *Manager) Enabled() bool {
	return m != nil && m.enabled
}

// Bases 返回安全基准目录状态，不直接列出其全部内容。
func (m *Manager) Bases() []BaseInfo {
	if !m.Enabled() {
		return []BaseInfo{}
	}
	result := make([]BaseInfo, 0, len(m.baseOrder))
	for _, id := range m.baseOrder {
		base := m.bases[id]
		info, err := os.Stat(base.Path)
		result = append(result, BaseInfo{
			ID:        base.ID,
			Name:      base.Name,
			Available: err == nil && info.IsDir(),
		})
	}
	return result
}

// GetServiceSource 返回人工配置或当前服务的有限自动发现结果。
func (m *Manager) GetServiceSource(ctx context.Context, serviceName string, refresh bool) (ServiceSource, error) {
	if !m.Enabled() {
		return ServiceSource{}, ErrDisabled
	}
	if !m.serviceExists(serviceName) {
		return ServiceSource{}, ErrServiceNotFound
	}

	if mapping, ok, err := m.mappings.Get(serviceName); err != nil {
		return ServiceSource{}, err
	} else if ok {
		directories, mappingErr := m.mappingDirectories(mapping)
		if mappingErr == nil && len(directories) > 0 {
			selected := directories[0]
			directories = m.appendArchiveDirectories(ctx, directories, selected)
			return ServiceSource{
				Service:     serviceName,
				Mode:        "manual",
				Directories: directories,
				Selected:    &selected,
				Mapping:     &mapping,
			}, nil
		}
		automatic, err := m.discoverServiceSource(ctx, serviceName, refresh)
		if err != nil {
			return ServiceSource{}, err
		}
		automatic.Mode = "invalid_manual"
		automatic.Mapping = &mapping
		automatic.Selected = nil
		if mappingErr != nil {
			automatic.Reason = "已保存的日志目录失效: " + mappingErr.Error()
		}
		return automatic, nil
	}
	return m.discoverServiceSource(ctx, serviceName, refresh)
}

// SaveMapping 验证并持久化服务的人工日志目录。
func (m *Manager) SaveMapping(serviceName string, directories []MappingDirectory) (ServiceLogMapping, error) {
	if !m.Enabled() {
		return ServiceLogMapping{}, ErrDisabled
	}
	if !m.serviceExists(serviceName) {
		return ServiceLogMapping{}, ErrServiceNotFound
	}
	if len(directories) == 0 {
		return ServiceLogMapping{}, fmt.Errorf("至少配置一个日志目录")
	}
	seenIDs := make(map[string]struct{}, len(directories))
	for index := range directories {
		directory := &directories[index]
		if strings.TrimSpace(directory.ID) == "" {
			directory.ID = fmt.Sprintf("directory-%d", index+1)
		}
		if _, exists := seenIDs[directory.ID]; exists {
			return ServiceLogMapping{}, fmt.Errorf("日志目录 id 重复: %s", directory.ID)
		}
		seenIDs[directory.ID] = struct{}{}
		validation := m.ValidateMapping(directory.BaseID, directory.RelativePath)
		if !validation.Valid {
			return ServiceLogMapping{}, errors.New(validation.Error)
		}
		directory.RelativePath = validation.RelativePath
		if strings.TrimSpace(directory.Name) == "" {
			directory.Name = filepath.Base(filepath.FromSlash(directory.RelativePath))
		}
	}
	mapping := ServiceLogMapping{Directories: directories, UpdatedAt: time.Now()}
	if err := m.mappings.Set(serviceName, mapping); err != nil {
		return ServiceLogMapping{}, err
	}
	m.InvalidateService(serviceName)
	return mapping, nil
}

// DeleteMapping 删除人工配置并恢复自动发现。
func (m *Manager) DeleteMapping(serviceName string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	if err := m.mappings.Delete(serviceName); err != nil {
		return err
	}
	m.InvalidateService(serviceName)
	return nil
}

// ValidateMapping 检测一个 base_id + 相对路径是否安全可用。
func (m *Manager) ValidateMapping(baseID string, relativePath string) MappingValidation {
	result := MappingValidation{BaseID: baseID, RelativePath: filepath.ToSlash(filepath.Clean(filepath.FromSlash(relativePath)))}
	base, err := m.getBase(baseID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.BaseName = base.Name
	directoryPath, err := resolveDirectory(base, relativePath)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	cleaned, _ := cleanRelativePath(relativePath, true)
	result.RelativePath = filepath.ToSlash(cleaned)
	result.ResolvedPath = directoryPath
	_, _, err = visitDirectoryLimited(directoryPath, 2000, func(entry os.DirEntry) bool {
		if entry.Type()&os.ModeSymlink != 0 {
			return true
		}
		if entry.IsDir() {
			result.DirectoryCount++
			return true
		}
		if extensionAllowed(base.FollowExtensions, entry.Name()) || extensionAllowed(base.DownloadExtensions, entry.Name()) {
			result.FileCount++
		}
		return true
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Valid = true
	return result
}

// BrowseDirectories 按层浏览安全基准目录，绝不递归。
func (m *Manager) BrowseDirectories(baseID string, relativePath string) (BrowseResult, error) {
	base, err := m.getBase(baseID)
	if err != nil {
		return BrowseResult{}, err
	}
	directoryPath, err := resolveDirectory(base, relativePath)
	if err != nil {
		return BrowseResult{}, err
	}
	cleaned, _ := cleanRelativePath(relativePath, true)
	current := filepath.ToSlash(cleaned)
	parent := ""
	if current != "" {
		parentPath := filepath.Dir(filepath.FromSlash(current))
		if parentPath != "." {
			parent = filepath.ToSlash(parentPath)
		}
	}
	result := BrowseResult{BaseID: baseID, Path: current, Parent: parent, Entries: []BrowseEntry{}}
	stoppedAtPageSize := false
	_, hitEntryLimit, err := visitDirectoryLimited(directoryPath, 2000, func(entry os.DirEntry) bool {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return true
		}
		if len(result.Entries) >= browsePageSize {
			stoppedAtPageSize = true
			return false
		}
		childPath := entry.Name()
		if current != "" {
			childPath = filepath.ToSlash(filepath.Join(filepath.FromSlash(current), entry.Name()))
		}
		result.Entries = append(result.Entries, BrowseEntry{Name: entry.Name(), Path: childPath})
		return true
	})
	if err != nil {
		return BrowseResult{}, err
	}
	result.Truncated = stoppedAtPageSize || hitEntryLimit
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Name < result.Entries[j].Name })
	return result, nil
}

// ListFiles 列出指定授权目录内的普通日志文件，不递归。
func (m *Manager) ListFiles(baseID string, directory string) ([]FileInfo, error) {
	base, err := m.getBase(baseID)
	if err != nil {
		return nil, err
	}
	directoryPath, err := resolveDirectory(base, directory)
	if err != nil {
		return nil, err
	}

	result := make([]FileInfo, 0)
	_, _, err = visitDirectoryLimited(directoryPath, 5000, func(entry os.DirEntry) bool {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return true
		}
		entryInfo, infoErr := entry.Info()
		if infoErr != nil || !entryInfo.Mode().IsRegular() {
			return true
		}
		followable := extensionAllowed(base.FollowExtensions, entry.Name())
		downloadable := extensionAllowed(base.DownloadExtensions, entry.Name())
		if !followable && !downloadable {
			return true
		}
		relative, relativeErr := filepath.Rel(base.Path, filepath.Join(directoryPath, entry.Name()))
		if relativeErr != nil {
			return true
		}
		result = append(result, FileInfo{
			BaseID:       base.ID,
			Path:         filepath.ToSlash(relative),
			Name:         entry.Name(),
			Size:         entryInfo.Size(),
			ModifiedAt:   entryInfo.ModTime(),
			Followable:   followable,
			Downloadable: downloadable,
			Archived:     strings.EqualFold(filepath.Ext(entry.Name()), ".gz"),
		})
		return true
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Followable != result[j].Followable {
			return result[i].Followable
		}
		if strings.EqualFold(result[i].Name, "info.log") != strings.EqualFold(result[j].Name, "info.log") {
			return strings.EqualFold(result[i].Name, "info.log")
		}
		if !result[i].ModifiedAt.Equal(result[j].ModifiedAt) {
			return result[i].ModifiedAt.After(result[j].ModifiedAt)
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// OpenDownload 安全打开允许下载的普通文件。
func (m *Manager) OpenDownload(baseID string, relativePath string) (*os.File, os.FileInfo, error) {
	base, err := m.getBase(baseID)
	if err != nil {
		return nil, nil, err
	}
	if !extensionAllowed(base.DownloadExtensions, relativePath) {
		return nil, nil, ErrFileNotAllowed
	}
	return secureOpenRegularFile(base, relativePath)
}

func (m *Manager) getFollowBase(baseID string, relativePath string) (*Base, error) {
	base, err := m.getBase(baseID)
	if err != nil {
		return nil, err
	}
	if !extensionAllowed(base.FollowExtensions, relativePath) {
		return nil, ErrFileNotAllowed
	}
	if _, err := cleanRelativePath(relativePath, false); err != nil {
		return nil, err
	}
	return base, nil
}

func (m *Manager) getBase(baseID string) (*Base, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	base, ok := m.bases[baseID]
	if !ok {
		return nil, ErrBaseNotFound
	}
	return base, nil
}

func (m *Manager) serviceExists(serviceName string) bool {
	if serviceName == "" || m.serviceManager == nil {
		return false
	}
	project := m.serviceManager.GetProject()
	return project != nil && project.Services[serviceName] != nil
}

func (m *Manager) mappingDirectories(mapping ServiceLogMapping) ([]DirectoryInfo, error) {
	directories := make([]DirectoryInfo, 0, len(mapping.Directories))
	for _, item := range mapping.Directories {
		validation := m.ValidateMapping(item.BaseID, item.RelativePath)
		if !validation.Valid {
			return nil, errors.New(validation.Error)
		}
		displayName := item.Name
		if displayName == "" {
			displayName = validation.RelativePath
		}
		directories = append(directories, DirectoryInfo{
			BaseID:      item.BaseID,
			BaseName:    validation.BaseName,
			Path:        validation.RelativePath,
			DisplayName: displayName,
			Recommended: true,
			MatchMethod: "manual",
			Manual:      true,
			Score:       1000,
		})
	}
	return directories, nil
}

// InvalidateService 清除指定服务自动发现缓存。
func (m *Manager) InvalidateService(serviceName string) {
	m.cacheMu.Lock()
	delete(m.cache, serviceName)
	m.cacheMu.Unlock()
}

// InvalidateAll 清除全部自动发现缓存。
func (m *Manager) InvalidateAll() {
	m.cacheMu.Lock()
	m.cache = make(map[string]cachedSource)
	m.cacheMu.Unlock()
}

func normalizeBase(raw config.FileLogBaseConfig, followExtensions, downloadExtensions map[string]struct{}) (*Base, error) {
	if !baseIDPattern.MatchString(raw.ID) {
		return nil, fmt.Errorf("id 只能包含字母、数字及 . _ -")
	}
	if strings.TrimSpace(raw.Path) == "" || !filepath.IsAbs(raw.Path) {
		return nil, fmt.Errorf("path 必须是绝对路径")
	}
	absolutePath := filepath.Clean(raw.Path)
	if filepath.Dir(absolutePath) == absolutePath {
		return nil, fmt.Errorf("不能授权文件系统根目录")
	}
	if resolved, err := filepath.EvalSymlinks(absolutePath); err == nil {
		absolutePath = filepath.Clean(resolved)
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		name = raw.ID
	}
	return &Base{
		ID:                 raw.ID,
		Name:               name,
		Path:               absolutePath,
		FollowExtensions:   cloneExtensionSet(followExtensions),
		DownloadExtensions: cloneExtensionSet(downloadExtensions),
	}, nil
}

func normalizeExtensions(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		result[value] = struct{}{}
	}
	return result
}

func cloneExtensionSet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for value := range source {
		result[value] = struct{}{}
	}
	return result
}

func extensionAllowed(extensions map[string]struct{}, fileName string) bool {
	_, ok := extensions[strings.ToLower(filepath.Ext(fileName))]
	return ok
}

func resolveDirectory(base *Base, relativePath string) (string, error) {
	cleaned, err := cleanRelativePath(relativePath, true)
	if err != nil {
		return "", err
	}
	fullPath := base.Path
	if cleaned != "" {
		fullPath = filepath.Join(base.Path, cleaned)
	}
	if err := ensurePathComponentsSafe(base.Path, fullPath); err != nil {
		return "", err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrInvalidPath
	}
	return fullPath, nil
}

func secureOpenRegularFile(base *Base, relativePath string) (*os.File, os.FileInfo, error) {
	cleaned, err := cleanRelativePath(relativePath, false)
	if err != nil {
		return nil, nil, err
	}
	fullPath := filepath.Join(base.Path, cleaned)
	if err := ensurePathComponentsSafe(base.Path, fullPath); err != nil {
		return nil, nil, err
	}
	file, err := openLogFile(fullPath)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, ErrInvalidPath
	}
	return file, info, nil
}

func cleanRelativePath(value string, allowBase bool) (string, error) {
	if strings.ContainsRune(value, '\x00') || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return "", ErrInvalidPath
	}
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if cleaned == "." {
		if allowBase {
			return "", nil
		}
		return "", ErrInvalidPath
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", ErrInvalidPath
	}
	return cleaned, nil
}

func ensurePathComponentsSafe(basePath string, targetPath string) error {
	relative, err := filepath.Rel(basePath, targetPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ErrInvalidPath
	}
	current := basePath
	if relative == "." {
		return nil
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidPath
		}
	}
	return nil
}

func relativeWithin(basePath string, targetPath string) (string, bool) {
	relative, err := filepath.Rel(filepath.Clean(basePath), filepath.Clean(targetPath))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	if relative == "." {
		return "", true
	}
	return filepath.ToSlash(relative), true
}

package filelog

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/docker"
)

var logVariablePattern = regexp.MustCompile(`(?i)(^|_)LOGS?(_|$)`)

type discoveredMount struct {
	source         string
	target         string
	declaredSource string
}

type identifierCandidate struct {
	value  string
	method string
	score  int
}

type scanDirectory struct {
	path  string
	depth int
}

func (m *Manager) discoverServiceSource(ctx context.Context, serviceName string, refresh bool) (ServiceSource, error) {
	var runtimeInfo *docker.ServiceLogRuntime
	if m.dockerClient != nil {
		runtimeInfo, _ = m.dockerClient.GetServiceLogRuntime(ctx, serviceName)
	}
	signature := m.discoverySignature(serviceName, runtimeInfo)
	if !refresh {
		m.cacheMu.Lock()
		cached, ok := m.cache[serviceName]
		m.cacheMu.Unlock()
		if ok && cached.signature == signature && time.Now().Before(cached.expiresAt) {
			return cached.value, nil
		}
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, m.discovery.timeout)
	defer cancel()
	directories, truncated := m.collectServiceDirectories(discoveryCtx, serviceName, runtimeInfo)
	sort.Slice(directories, func(i, j int) bool {
		if directories[i].Score != directories[j].Score {
			return directories[i].Score > directories[j].Score
		}
		if directories[i].BaseName != directories[j].BaseName {
			return directories[i].BaseName < directories[j].BaseName
		}
		return directories[i].Path < directories[j].Path
	})

	result := ServiceSource{
		Service:            serviceName,
		Mode:               "unmatched",
		Directories:        directories,
		DiscoveryTruncated: truncated || discoveryCtx.Err() != nil,
	}
	if len(directories) == 0 {
		result.Reason = "未发现该服务位于安全基准目录内的文件日志"
	} else {
		hasCompetingHighConfidence := false
		for _, directory := range directories[1:] {
			if directory.Score >= 60 && !directoriesShareLogTree(directories[0], directory) {
				hasCompetingHighConfidence = true
				break
			}
		}
		secondScore := -1
		if len(directories) > 1 {
			secondScore = directories[1].Score
		}
		uniqueHighConfidenceTree := !hasCompetingHighConfidence
		clearlyStronger := secondScore < 0 || directories[0].Score-secondScore >= 20
		if directories[0].Score >= 60 && (uniqueHighConfidenceTree || clearlyStronger) {
			directories[0].Recommended = true
			selected := directories[0]
			result.Directories = m.appendArchiveDirectories(ctx, directories, selected)
			result.Selected = &selected
			result.Mode = "automatic"
		} else {
			result.Reason = "发现多个或低可信日志目录，请选择或人工设置"
		}
	}

	m.cacheMu.Lock()
	m.cache[serviceName] = cachedSource{
		signature: signature,
		expiresAt: time.Now().Add(m.discovery.cacheTTL),
		value:     result,
	}
	m.cacheMu.Unlock()
	return result, nil
}

func (m *Manager) discoverySignature(serviceName string, runtimeInfo *docker.ServiceLogRuntime) string {
	projectFileTime := int64(0)
	envFileTime := int64(0)
	if m.serviceManager != nil {
		if project := m.serviceManager.GetProject(); project != nil {
			if info, err := os.Stat(project.FilePath); err == nil {
				projectFileTime = info.ModTime().UnixNano()
			}
		}
	}
	if info, err := os.Stat(filepath.Join(m.projectDir, ".env")); err == nil {
		envFileTime = info.ModTime().UnixNano()
	}
	containerID := ""
	if runtimeInfo != nil {
		containerID = runtimeInfo.ContainerID
	}
	return fmt.Sprintf("%s|%s|%d|%d", serviceName, containerID, projectFileTime, envFileTime)
}

func (m *Manager) collectServiceDirectories(
	ctx context.Context,
	serviceName string,
	runtimeInfo *docker.ServiceLogRuntime,
) ([]DirectoryInfo, bool) {
	project := m.serviceManager.GetProject()
	definition := project.Services[serviceName]
	envVars, _ := compose.ReadEnvVars(filepath.Join(m.projectDir, ".env"))
	mounts := make([]discoveredMount, 0)
	// 声明态优先，保留 ${...} 变量名这一通用日志语义；运行态补齐真实挂载。
	if declared, err := compose.ReadServiceVolumes(m.projectDir, serviceName); err == nil {
		for _, mount := range declared {
			source := expandVarsRecursive(mount.Source, envVars)
			source = normalizeDeclaredMountSource(m.projectDir, source, mount.Type)
			if source == "" {
				continue
			}
			mounts = append(mounts, discoveredMount{
				source:         source,
				target:         mount.Target,
				declaredSource: mount.Source,
			})
		}
	}
	if runtimeInfo != nil {
		for _, mount := range runtimeInfo.Mounts {
			mounts = append(mounts, discoveredMount{source: mount.Source, target: mount.Destination})
		}
	}

	identifiers := serviceIdentifiers(serviceName, definition.Image, definition.Environment, envVars, runtimeInfo)
	candidates := make(map[string]DirectoryInfo)
	truncated := false
	seenMounts := make(map[string]struct{})
	for _, mount := range mounts {
		if err := ctx.Err(); err != nil {
			truncated = true
			break
		}
		mountKey := filepath.Clean(mount.source) + "\x00" + path.Clean(strings.ReplaceAll(mount.target, "\\", "/"))
		if _, seen := seenMounts[mountKey]; seen {
			continue
		}
		seenMounts[mountKey] = struct{}{}
		base, mountRelative, ok := m.baseForPath(mount.source)
		if !ok {
			continue
		}
		logLike := mountHasLogSemantics(mount)

		if runtimeInfo != nil {
			for _, key := range []string{"LOGGING_PATH", "LOG_PATH", "LOG_DIR", "LOG_HOME"} {
				if relative, translated := translateContainerPath(base.Path, mount.source, mount.target, runtimeInfo.LogHints[key]); translated {
					m.addDirectoryCandidate(candidates, base, relative, "runtime_log_path", 100, true)
				}
			}
		}
		for _, hint := range environmentPathHints(definition.Environment, envVars) {
			if relative, translated := translateContainerPath(base.Path, mount.source, mount.target, hint); translated {
				m.addDirectoryCandidate(candidates, base, relative, "compose_log_path", 95, true)
			}
		}

		for _, identifier := range identifiers {
			relative := joinRelative(mountRelative, identifier.value)
			if relative == "" {
				continue
			}
			fullPath := filepath.Join(base.Path, filepath.FromSlash(relative))
			if directoryHasAllowedFiles(fullPath, base) {
				m.addDirectoryCandidate(candidates, base, relative, identifier.method, identifier.score, false)
			}
		}

		if directoryHasAllowedFiles(mount.source, base) {
			score := 55
			method := "mounted_log_files"
			if logLike {
				score = 80
				method = "log_mount"
			}
			m.addDirectoryCandidate(candidates, base, mountRelative, method, score, false)
		} else if logLike && directoryExists(mount.source) {
			m.addDirectoryCandidate(candidates, base, mountRelative, "log_mount", 65, true)
		}

		scanned, wasTruncated := m.scanMountShallow(ctx, base, mount.source, identifiers, logLike)
		if wasTruncated {
			truncated = true
		}
		for _, candidate := range scanned {
			m.addDirectoryCandidate(candidates, base, candidate.Path, candidate.MatchMethod, candidate.Score, false)
		}
	}
	// 通用兜底：服务声明引用的日志语义变量可直接指向安全基准目录下的宿主机路径。
	// 仍然不枚举具体变量名，也不接受未被该 service 引用的项目全局变量。
	for _, variableName := range definition.VarRefs {
		if !logVariablePattern.MatchString(variableName) {
			continue
		}
		candidatePath := expandVarsRecursive(envVars[variableName], envVars)
		if candidatePath == "" || !filepath.IsAbs(candidatePath) {
			continue
		}
		base, relative, ok := m.baseForPath(candidatePath)
		if !ok {
			continue
		}
		m.addDirectoryCandidate(candidates, base, relative, "log_variable", 60, true)
	}

	result := make([]DirectoryInfo, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate)
	}
	return result, truncated
}

func (m *Manager) scanMountShallow(
	ctx context.Context,
	base *Base,
	mountSource string,
	identifiers []identifierCandidate,
	logLike bool,
) ([]DirectoryInfo, bool) {
	queue := []scanDirectory{{path: mountSource, depth: 0}}
	entriesSeen := 0
	truncated := false
	matched := make([]DirectoryInfo, 0)
	generic := make([]DirectoryInfo, 0)
	identifierScores := make(map[string]identifierCandidate)
	for _, identifier := range identifiers {
		identifierScores[normalizeIdentifier(identifier.value)] = identifier
	}

	for len(queue) > 0 {
		if ctx.Err() != nil {
			return matched, true
		}
		current := queue[0]
		queue = queue[1:]
		if current.depth > 0 && directoryHasAllowedFiles(current.path, base) {
			relative, ok := relativeWithin(base.Path, current.path)
			if ok {
				name := normalizeIdentifier(filepath.Base(current.path))
				if identifier, found := identifierScores[name]; found {
					matched = append(matched, DirectoryInfo{Path: relative, MatchMethod: identifier.method, Score: identifier.score - 5})
				} else {
					generic = append(generic, DirectoryInfo{Path: relative, MatchMethod: "shallow_scan", Score: 50})
				}
			}
		}
		if current.depth >= m.discovery.maxDepth {
			continue
		}
		remaining := m.discovery.maxEntries - entriesSeen
		visited, hitLimit, err := visitDirectoryLimited(current.path, remaining, func(entry os.DirEntry) bool {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return true
			}
			queue = append(queue, scanDirectory{path: filepath.Join(current.path, entry.Name()), depth: current.depth + 1})
			return true
		})
		entriesSeen += visited
		if err != nil {
			continue
		}
		if hitLimit {
			truncated = true
			queue = nil
		}
	}
	if len(matched) == 0 && len(generic) == 1 {
		if logLike {
			generic[0].Score = 65
			generic[0].MatchMethod = "single_log_directory"
		}
		matched = append(matched, generic[0])
	}
	return matched, truncated
}

func (m *Manager) addDirectoryCandidate(
	candidates map[string]DirectoryInfo,
	base *Base,
	relative string,
	method string,
	score int,
	allowEmpty bool,
) {
	cleaned, err := cleanRelativePath(relative, true)
	if err != nil {
		return
	}
	fullPath := base.Path
	if cleaned != "" {
		fullPath = filepath.Join(base.Path, cleaned)
	}
	if !directoryExists(fullPath) {
		return
	}
	if !allowEmpty && !directoryHasAllowedFiles(fullPath, base) {
		return
	}
	relative = filepath.ToSlash(cleaned)
	key := base.ID + "\x00" + relative
	if existing, found := candidates[key]; found && existing.Score >= score {
		return
	}
	displayName := relative
	if displayName == "" {
		displayName = base.Name
	}
	candidates[key] = DirectoryInfo{
		BaseID:      base.ID,
		BaseName:    base.Name,
		Path:        relative,
		DisplayName: displayName,
		MatchMethod: method,
		Score:       score,
	}
}

func (m *Manager) baseForPath(candidatePath string) (*Base, string, bool) {
	for _, id := range m.baseOrder {
		base := m.bases[id]
		if relative, ok := relativeWithin(base.Path, candidatePath); ok {
			return base, relative, true
		}
	}
	return nil, "", false
}

func serviceIdentifiers(
	serviceName string,
	declaredImage string,
	environment []string,
	envVars map[string]string,
	runtimeInfo *docker.ServiceLogRuntime,
) []identifierCandidate {
	result := make([]identifierCandidate, 0)
	seen := make(map[string]struct{})
	add := func(value, method string, score int) {
		value = validIdentifier(value)
		if value == "" {
			return
		}
		normalized := normalizeIdentifier(value)
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		result = append(result, identifierCandidate{value: value, method: method, score: score})
	}
	if runtimeInfo != nil {
		add(runtimeInfo.LogHints["SPRING_APPLICATION_NAME"], "runtime_service_name", 100)
		add(runtimeInfo.LogHints["SERVICE_NAME"], "runtime_service_name", 100)
		add(imageBaseName(runtimeInfo.Image), "image_name", 90)
	}
	for key, value := range environmentNameHints(environment, envVars) {
		add(value, strings.ToLower(key), 95)
	}
	add(imageBaseName(expandVarsRecursive(declaredImage, envVars)), "image_name", 90)
	add(serviceName, "service_name", 80)
	if runtimeInfo != nil {
		add(strings.TrimPrefix(runtimeInfo.ContainerName, "/"), "container_name", 70)
	}
	return result
}

func environmentPathHints(environment []string, envVars map[string]string) []string {
	hints := environmentHints(environment, envVars)
	result := make([]string, 0, 4)
	for _, key := range []string{"LOGGING_PATH", "LOG_PATH", "LOG_DIR", "LOG_HOME"} {
		if hints[key] != "" {
			result = append(result, hints[key])
		}
	}
	return result
}

func environmentNameHints(environment []string, envVars map[string]string) map[string]string {
	hints := environmentHints(environment, envVars)
	return map[string]string{
		"SPRING_APPLICATION_NAME": hints["SPRING_APPLICATION_NAME"],
		"SERVICE_NAME":            hints["SERVICE_NAME"],
	}
}

func environmentHints(environment []string, envVars map[string]string) map[string]string {
	result := make(map[string]string)
	for _, raw := range environment {
		key, value, ok := strings.Cut(raw, "=")
		key = strings.ToUpper(strings.TrimSpace(key))
		if !ok {
			value = envVars[key]
		}
		switch key {
		case "LOGGING_PATH", "LOG_PATH", "LOG_DIR", "LOG_HOME", "SPRING_APPLICATION_NAME", "SERVICE_NAME":
			result[key] = expandVarsRecursive(value, envVars)
		}
	}
	return result
}

func expandVarsRecursive(value string, vars map[string]string) string {
	for i := 0; i < 8; i++ {
		next := compose.ExpandVars(value, vars)
		if next == value {
			return next
		}
		value = next
	}
	return value
}

func normalizeDeclaredMountSource(projectDir string, source string, mountType string) string {
	if source == "" || mountType == "volume" {
		return ""
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join(projectDir, source)
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return ""
	}
	return filepath.Clean(absolute)
}

func translateContainerPath(basePath string, mountSource string, mountTarget string, containerPath string) (string, bool) {
	if mountSource == "" || mountTarget == "" || containerPath == "" {
		return "", false
	}
	target := path.Clean(strings.ReplaceAll(mountTarget, "\\", "/"))
	hint := path.Clean(strings.ReplaceAll(containerPath, "\\", "/"))
	relative := ""
	if hint != target {
		prefix := strings.TrimSuffix(target, "/") + "/"
		if !strings.HasPrefix(hint, prefix) {
			return "", false
		}
		relative = strings.TrimPrefix(hint, prefix)
	}
	hostPath := filepath.Join(mountSource, filepath.FromSlash(relative))
	return relativeWithin(basePath, hostPath)
}

func mountHasLogSemantics(mount discoveredMount) bool {
	if pathHasLogSegment(mount.source) || pathHasLogSegment(mount.target) {
		return true
	}
	for _, match := range regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)`).FindAllStringSubmatch(mount.declaredSource, -1) {
		if len(match) > 1 && logVariablePattern.MatchString(match[1]) {
			return true
		}
	}
	return false
}

func pathHasLogSegment(value string) bool {
	value = strings.ReplaceAll(value, "\\", "/")
	for _, segment := range strings.Split(value, "/") {
		segment = strings.ToLower(strings.TrimSpace(segment))
		if segment == "log" || segment == "logs" {
			return true
		}
	}
	return false
}

func directoryHasAllowedFiles(directoryPath string, base *Base) bool {
	found := false
	_, _, _ = visitDirectoryLimited(directoryPath, 512, func(entry os.DirEntry) bool {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return true
		}
		if extensionAllowed(base.FollowExtensions, entry.Name()) || extensionAllowed(base.DownloadExtensions, entry.Name()) {
			found = true
			return false
		}
		return true
	})
	return found
}

func directoryExists(directoryPath string) bool {
	info, err := os.Stat(directoryPath)
	return err == nil && info.IsDir()
}

func imageBaseName(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	if digest := strings.Index(image, "@"); digest >= 0 {
		image = image[:digest]
	}
	if slash := strings.LastIndex(image, "/"); slash >= 0 {
		image = image[slash+1:]
	}
	if colon := strings.LastIndex(image, ":"); colon >= 0 {
		image = image[:colon]
	}
	return image
}

func validIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return ""
	}
	return value
}

func normalizeIdentifier(value string) string {
	value = strings.ToLower(value)
	return strings.NewReplacer("-", "", "_", "", ".", "").Replace(value)
}

func directoriesShareLogTree(left, right DirectoryInfo) bool {
	if left.BaseID != right.BaseID {
		return false
	}
	leftPath := strings.Trim(left.Path, "/")
	rightPath := strings.Trim(right.Path, "/")
	if leftPath == "" || rightPath == "" || leftPath == rightPath {
		return true
	}
	return strings.HasPrefix(leftPath, rightPath+"/") || strings.HasPrefix(rightPath, leftPath+"/")
}

func joinRelative(base string, name string) string {
	name = validIdentifier(name)
	if name == "" {
		return ""
	}
	if base == "" {
		return name
	}
	return path.Join(base, name)
}

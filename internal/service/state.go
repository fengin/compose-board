// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// state.go 管理 .composeboard-state.json 状态文件，
// 记录每个服务上次已生效的 image + env 配置。
// 首次启动视为基线，不产生漂移告警。
package service

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fengin/composeboard/internal/compose"
)

// ComposeBoardState .composeboard-state.json 顶层结构
type ComposeBoardState struct {
	Version  int                          `json:"version"`
	Services map[string]ServiceStateEntry `json:"services"`
	Profiles map[string]ProfileStateEntry `json:"profiles"`
}

// ServiceStateEntry 单个服务上次已生效的状态
type ServiceStateEntry struct {
	Image       string            `json:"image,omitempty"`        // 已生效的展开镜像
	Env         map[string]string `json:"env,omitempty"`          // 已生效的 env 变量值
	ComposeHash string            `json:"compose_hash,omitempty"` // 服务定义的 SHA256（检测 compose 结构变更）
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ProfileStateEntry Profile 的配置启用态。
// 注意：这里只表达“是否启用这个 profile 配置”，不表达下属服务是否全部运行。
type ProfileStateEntry struct {
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	stateFileVersion = 3
	stateFileName    = ".composeboard-state.json"
)

// StateManager 状态文件管理器
type StateManager struct {
	projectDir string
	manager    *ServiceManager

	mu sync.RWMutex
}

// NewStateManager 创建状态管理器
func NewStateManager(projectDir string, manager *ServiceManager) *StateManager {
	return &StateManager{
		projectDir: projectDir,
		manager:    manager,
	}
}

// EnsureState 确保状态文件存在
// 首次启动：以当前 .env 和声明态为基线创建
func (s *StateManager) EnsureState() {
	s.mu.Lock()
	defer s.mu.Unlock()

	statePath := s.getStatePath()

	// 已存在：补齐缺失的 profile 配置态基线（升级场景）
	if _, err := os.Stat(statePath); err == nil {
		state, loadErr := s.loadStateLocked()
		if loadErr != nil {
			log.Printf("[STATE] 读取已存在状态文件失败，重新初始化: %v", loadErr)
			state = s.buildCurrentState()
			if err := s.writeStateLocked(state); err != nil {
				log.Printf("[STATE] 重建状态文件失败: %v", err)
			}
			return
		}

		if s.ensureProfileEntriesLocked(state) {
			if err := s.writeStateLocked(state); err != nil {
				log.Printf("[STATE] 补齐 Profile 基线失败: %v", err)
			}
		}
		return
	}

	// 首次启动：构造基线状态
	state := s.buildCurrentState()
	if err := s.writeStateLocked(state); err != nil {
		log.Printf("[STATE] 初始化失败: %v", err)
		return
	}
	log.Printf("[STATE] 初始化基线: %d 个服务", len(state.Services))
}

// UpdateServiceState 更新单个服务的已生效状态（升级/重建后调用）
func (s *StateManager) UpdateServiceState(serviceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadStateLocked()
	if err != nil {
		log.Printf("[STATE] 读取状态失败: %v", err)
		state = s.buildCurrentState()
	}

	envVars := s.manager.GetEnvVars()
	project := s.manager.GetProject()

	if project != nil {
		if decl, ok := project.Services[serviceName]; ok {
			state.Services[serviceName] = s.buildServiceEntry(decl, envVars)
		}
	}

	if err := s.writeStateLocked(state); err != nil {
		log.Printf("[STATE] 更新服务 %s 失败: %v", serviceName, err)
		return
	}
	log.Printf("[STATE] 已更新: %s", serviceName)
}

// IsProfileEnabled 返回 Profile 是否处于启用配置态。
func (s *StateManager) IsProfileEnabled(profileName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadStateLocked()
	if err != nil {
		state = s.buildCurrentState()
	}
	changed := s.ensureProfileEntriesLocked(state)
	entry, ok := state.Profiles[profileName]
	if changed {
		if writeErr := s.writeStateLocked(state); writeErr != nil {
			log.Printf("[STATE] 回写 Profile 基线失败: %v", writeErr)
		}
	}
	return ok && entry.Enabled
}

// SetProfileEnabled 更新 Profile 配置启用态。
func (s *StateManager) SetProfileEnabled(profileName string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadStateLocked()
	if err != nil {
		log.Printf("[STATE] 读取状态失败: %v", err)
		state = s.buildCurrentState()
	}
	s.ensureProfileEntriesLocked(state)
	state.Profiles[profileName] = ProfileStateEntry{
		Enabled:   enabled,
		UpdatedAt: time.Now(),
	}
	if err := s.writeStateLocked(state); err != nil {
		log.Printf("[STATE] 更新 Profile %s 失败: %v", profileName, err)
		return
	}
	log.Printf("[STATE] 已更新 Profile 配置态: %s = %t", profileName, enabled)
}

// BackfillMissingComposeHashes 用当前已解析的 Compose 声明为缺失 compose_hash 的历史状态补基线。
// 该方法应在保存新的 Compose 文件之前调用，确保旧版本状态文件能从下一次配置保存开始参与差异检测。
func (s *StateManager) BackfillMissingComposeHashes() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	project := s.manager.GetProject()
	if project == nil {
		return nil
	}

	state, err := s.loadStateLocked()
	if err != nil {
		log.Printf("[STATE] 读取状态失败，按当前配置补齐 Compose hash 基线: %v", err)
		state = s.buildCurrentState()
	}
	if state.Services == nil {
		state.Services = make(map[string]ServiceStateEntry)
	}

	envVars := s.manager.GetEnvVars()
	changed := false
	for _, decl := range project.Services {
		entry, ok := state.Services[decl.Name]
		if !ok {
			if runtimeEntry, recovered := s.manager.BuildRuntimeStateEntry(decl.Name); recovered {
				runtimeEntry.ComposeHash = decl.Hash
				s.backfillMissingEnvVars(&runtimeEntry, decl, envVars)
				state.Services[decl.Name] = runtimeEntry
			} else {
				state.Services[decl.Name] = s.buildServiceEntry(decl, envVars)
			}
			changed = true
			continue
		}

		if entry.ComposeHash == "" {
			entry.ComposeHash = decl.Hash
			changed = true
		}
		if s.backfillMissingEnvVars(&entry, decl, envVars) {
			changed = true
		}
		state.Services[decl.Name] = entry
	}

	if !changed {
		return nil
	}
	if err := s.writeStateLocked(state); err != nil {
		return err
	}
	log.Printf("[STATE] 已补齐 Compose hash 基线")
	return nil
}

// GetPendingEnvChanges 返回每个服务受影响的未生效变更变量
func (s *StateManager) GetPendingEnvChanges() map[string][]string {
	s.mu.RLock()
	state, err := s.loadStateLocked()
	if err != nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	currentEnv := s.manager.GetEnvVars()
	project := s.manager.GetProject()
	if project == nil {
		return nil
	}

	stateChanged := false
	result := make(map[string][]string)

	for _, decl := range project.Services {
		applied, ok := state.Services[decl.Name]
		if !ok {
			runtimeEntry, recovered := s.manager.BuildRuntimeStateEntry(decl.Name)
			if recovered {
				applied = runtimeEntry
				state.Services[decl.Name] = runtimeEntry
				stateChanged = true
			} else {
				continue
			}
		}

		var affected []string
		// 检查变量中引用的 env 是否变更
		for _, varName := range decl.VarRefs {
			currentVal := currentEnv[varName]
			appliedVal, exists := applied.Env[varName]

			// 兼容处理：v1.0.0 状态中没有 ComposeHash，且提取的变量不全（如遗漏了 volumes 中的变量）。
			// 如果该变量在旧状态中不存在，直接忽略，避免全量服务误报“配置已变更”。
			if !exists && applied.ComposeHash == "" {
				continue
			}

			if currentVal != appliedVal {
				affected = append(affected, varName)
			}
		}

		if len(affected) > 0 {
			result[decl.Name] = affected
		}
	}

	if len(result) == 0 {
		if stateChanged {
			s.persistRecoveredState(state)
		}
		return nil
	}
	if stateChanged {
		s.persistRecoveredState(state)
	}
	return result
}

// GetPendingConfigChanges 返回 Compose 配置有结构性变更的服务名集合。
// 通过对比服务声明的 hash 与上次已生效 hash 来检测。
func (s *StateManager) GetPendingConfigChanges() map[string]bool {
	s.mu.RLock()
	state, err := s.loadStateLocked()
	if err != nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	project := s.manager.GetProject()
	if project == nil {
		return nil
	}

	result := make(map[string]bool)

	for _, decl := range project.Services {
		applied, ok := state.Services[decl.Name]
		if !ok {
			// 新服务（state 中不存在），不标记 config_diff（它本身就是 not_deployed）
			continue
		}

		// compose_hash 为空视为历史基线，不告警（向后兼容）
		if applied.ComposeHash == "" {
			continue
		}

		if decl.Hash != applied.ComposeHash {
			result[decl.Name] = true
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// --- 内部实现 ---

func (s *StateManager) getStatePath() string {
	return filepath.Join(s.projectDir, stateFileName)
}

// buildCurrentState 按当前声明态 + .env 构建全量状态
func (s *StateManager) buildCurrentState() *ComposeBoardState {
	state := &ComposeBoardState{
		Version:  stateFileVersion,
		Services: make(map[string]ServiceStateEntry),
		Profiles: make(map[string]ProfileStateEntry),
	}

	project := s.manager.GetProject()
	if project == nil {
		return state
	}

	envVars := s.manager.GetEnvVars()

	for _, decl := range project.Services {
		state.Services[decl.Name] = s.buildServiceEntry(decl, envVars)
	}

	s.ensureProfileEntriesLocked(state)

	return state
}

// buildServiceEntry 构建单个服务的状态条目
func (s *StateManager) buildServiceEntry(decl *compose.DeclaredService, envVars map[string]string) ServiceStateEntry {
	entry := ServiceStateEntry{
		Env:       make(map[string]string),
		UpdatedAt: time.Now(),
	}

	// 展开后的镜像
	if decl.Image != "" {
		entry.Image = compose.ExpandVars(decl.Image, envVars)
	}

	// 记录引用的变量当前值
	for _, varName := range decl.VarRefs {
		if val, ok := envVars[varName]; ok {
			entry.Env[varName] = val
		}
	}

	if len(entry.Env) == 0 {
		entry.Env = nil
	}

	// 记录已生成的 Hash（检测结构性变更）
	entry.ComposeHash = decl.Hash

	return entry
}

func (s *StateManager) backfillMissingEnvVars(entry *ServiceStateEntry, decl *compose.DeclaredService, envVars map[string]string) bool {
	changed := false
	for _, varName := range decl.VarRefs {
		val, ok := envVars[varName]
		if !ok {
			continue
		}
		if entry.Env == nil {
			entry.Env = make(map[string]string)
		}
		if _, exists := entry.Env[varName]; !exists {
			entry.Env[varName] = val
			changed = true
		}
	}
	if len(entry.Env) == 0 {
		entry.Env = nil
	}
	return changed
}

// loadStateLocked 加载状态文件（调用方需持有锁）
func (s *StateManager) loadStateLocked() (*ComposeBoardState, error) {
	data, err := os.ReadFile(s.getStatePath())
	if err != nil {
		return nil, err
	}

	var state ComposeBoardState
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("[STATE] 状态文件损坏，按当前配置重建: %v", err)
		rebuilt := s.buildCurrentState()
		_ = s.writeStateLocked(rebuilt)
		return rebuilt, nil
	}

	if state.Services == nil {
		state.Services = make(map[string]ServiceStateEntry)
	}
	if state.Profiles == nil {
		state.Profiles = make(map[string]ProfileStateEntry)
	}

	return &state, nil
}

// writeStateLocked 原子写入状态文件（调用方需持有锁）
func (s *StateManager) writeStateLocked(state *ComposeBoardState) error {
	if state.Services == nil {
		state.Services = make(map[string]ServiceStateEntry)
	}
	if state.Profiles == nil {
		state.Profiles = make(map[string]ProfileStateEntry)
	}
	state.Version = stateFileVersion

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	statePath := s.getStatePath()
	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	// M-8: POSIX 上 os.Rename 是原子覆盖，不需 Remove
	// Windows 上 Rename 不能覆盖目标，需先 Remove
	if runtime.GOOS == "windows" {
		_ = os.Remove(statePath)
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func (s *StateManager) persistRecoveredState(state *ComposeBoardState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeStateLocked(state); err != nil {
		log.Printf("[STATE] 回填缺失服务基线失败: %v", err)
	}
}

// ensureProfileEntriesLocked 确保状态文件中的 Profile 配置态与当前 Compose 声明同步。
// 缺失条目会根据“当前是否存在任一下属容器”推断初始 enabled，避免升级后所有 Profile 突然显示禁用。
func (s *StateManager) ensureProfileEntriesLocked(state *ComposeBoardState) bool {
	project := s.manager.GetProject()
	if project == nil {
		return false
	}

	if state.Profiles == nil {
		state.Profiles = make(map[string]ProfileStateEntry)
	}

	changed := false
	profileMap := project.GetProfiles()
	validProfiles := make(map[string]struct{}, len(profileMap))

	deployedServices := make(map[string]bool)
	if s.manager != nil && s.manager.cache != nil {
		containers := s.manager.cache.Get()
		deployedServices = make(map[string]bool, len(containers))
		for _, ctr := range containers {
			deployedServices[ctr.ServiceName] = true
		}
	}

	for profileName, serviceNames := range profileMap {
		validProfiles[profileName] = struct{}{}
		if _, ok := state.Profiles[profileName]; ok {
			continue
		}

		enabled := false
		for _, serviceName := range serviceNames {
			if deployedServices[serviceName] {
				enabled = true
				break
			}
		}

		state.Profiles[profileName] = ProfileStateEntry{
			Enabled:   enabled,
			UpdatedAt: time.Now(),
		}
		changed = true
	}

	for profileName := range state.Profiles {
		if _, ok := validProfiles[profileName]; !ok {
			delete(state.Profiles, profileName)
			changed = true
		}
	}

	return changed
}

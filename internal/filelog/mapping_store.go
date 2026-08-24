package filelog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const (
	mappingFileName    = ".composeboard-file-logs.json"
	mappingFileVersion = 1
)

// MappingDirectory 一个可复制到其他项目的相对日志目录。
type MappingDirectory struct {
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	BaseID       string `json:"base_id"`
	RelativePath string `json:"relative_path"`
}

// ServiceLogMapping 单个 Compose service 的人工日志目录配置。
type ServiceLogMapping struct {
	Directories []MappingDirectory `json:"directories"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

type mappingDocument struct {
	Version  int                          `json:"version"`
	Services map[string]ServiceLogMapping `json:"services"`
}

// MappingStore 管理项目级、可复制的文件日志映射。
type MappingStore struct {
	path string
	mu   sync.Mutex
}

func NewMappingStore(projectDir string) *MappingStore {
	return &MappingStore{path: filepath.Join(projectDir, mappingFileName)}
}

func (s *MappingStore) Get(serviceName string) (ServiceLogMapping, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.loadLocked()
	if err != nil {
		return ServiceLogMapping{}, false, err
	}
	mapping, ok := document.Services[serviceName]
	return mapping, ok, nil
}

func (s *MappingStore) Set(serviceName string, mapping ServiceLogMapping) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.loadLocked()
	if err != nil {
		return err
	}
	document.Services[serviceName] = mapping
	return s.writeLocked(document)
}

func (s *MappingStore) Delete(serviceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.loadLocked()
	if err != nil {
		return err
	}
	if _, exists := document.Services[serviceName]; !exists {
		return nil
	}
	delete(document.Services, serviceName)
	return s.writeLocked(document)
}

func (s *MappingStore) loadLocked() (*mappingDocument, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return &mappingDocument{Version: mappingFileVersion, Services: make(map[string]ServiceLogMapping)}, nil
	}
	if err != nil {
		return nil, err
	}
	var document mappingDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document.Services == nil {
		document.Services = make(map[string]ServiceLogMapping)
	}
	return &document, nil
}

func (s *MappingStore) writeLocked(document *mappingDocument) error {
	document.Version = mappingFileVersion
	if document.Services == nil {
		document.Services = make(map[string]ServiceLogMapping)
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporaryPath := s.path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0600); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(s.path)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}

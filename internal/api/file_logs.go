package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fengin/composeboard/internal/filelog"
	"github.com/gin-gonic/gin"
)

type fileLogMappingRequest struct {
	Directories []filelog.MappingDirectory `json:"directories" binding:"required"`
}

type fileLogValidationRequest struct {
	BaseID       string `json:"base_id" binding:"required"`
	RelativePath string `json:"relative_path"`
}

// GetFileLogBases GET /api/file-logs/bases
func (h *Handler) GetFileLogBases(c *gin.Context) {
	if h.FileLogs == nil || !h.FileLogs.Enabled() {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "bases": []filelog.BaseInfo{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "bases": h.FileLogs.Bases()})
}

// GetServiceFileLogSource GET /api/file-logs/services/:name/source
func (h *Handler) GetServiceFileLogSource(c *gin.Context) {
	if h.FileLogs == nil {
		respondFileLogError(c, filelog.ErrDisabled)
		return
	}
	refresh := c.Query("refresh") == "true"
	source, err := h.FileLogs.GetServiceSource(c.Request.Context(), c.Param("name"), refresh)
	if err != nil {
		respondFileLogError(c, err)
		return
	}
	c.JSON(http.StatusOK, source)
}

// SaveServiceFileLogMapping PUT /api/file-logs/services/:name/mapping
func (h *Handler) SaveServiceFileLogMapping(c *gin.Context) {
	if h.FileLogs == nil {
		respondFileLogError(c, filelog.ErrDisabled)
		return
	}
	var request fileLogMappingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_LOG_MAPPING", "error": "日志目录配置格式不正确"})
		return
	}
	mapping, err := h.FileLogs.SaveMapping(c.Param("name"), request.Directories)
	if err != nil {
		respondFileLogError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"mapping": mapping})
}

// DeleteServiceFileLogMapping DELETE /api/file-logs/services/:name/mapping
func (h *Handler) DeleteServiceFileLogMapping(c *gin.Context) {
	if h.FileLogs == nil {
		respondFileLogError(c, filelog.ErrDisabled)
		return
	}
	if err := h.FileLogs.DeleteMapping(c.Param("name")); err != nil {
		respondFileLogError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已恢复自动匹配"})
}

// ValidateFileLogMapping POST /api/file-logs/mapping/validate
func (h *Handler) ValidateFileLogMapping(c *gin.Context) {
	if h.FileLogs == nil {
		respondFileLogError(c, filelog.ErrDisabled)
		return
	}
	var request fileLogValidationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_LOG_MAPPING", "error": "请输入安全基准目录和相对路径"})
		return
	}
	validation := h.FileLogs.ValidateMapping(request.BaseID, request.RelativePath)
	status := http.StatusOK
	if !validation.Valid {
		status = http.StatusUnprocessableEntity
	}
	c.JSON(status, validation)
}

// BrowseFileLogDirectories GET /api/file-logs/browse?base=:id&path=:path
func (h *Handler) BrowseFileLogDirectories(c *gin.Context) {
	if h.FileLogs == nil {
		respondFileLogError(c, filelog.ErrDisabled)
		return
	}
	result, err := h.FileLogs.BrowseDirectories(c.Query("base"), c.Query("path"))
	if err != nil {
		respondFileLogError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// ListFileLogFiles GET /api/file-logs/files?base=:base&directory=:directory
func (h *Handler) ListFileLogFiles(c *gin.Context) {
	if h.FileLogs == nil {
		respondFileLogError(c, filelog.ErrDisabled)
		return
	}
	files, err := h.FileLogs.ListFiles(c.Query("base"), c.Query("directory"))
	if err != nil {
		respondFileLogError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

// StreamFileLog GET /api/file-logs/stream?base=:base&path=:path&tail=100
func (h *Handler) StreamFileLog(c *gin.Context) {
	if h.FileLogs == nil {
		respondFileLogError(c, filelog.ErrDisabled)
		return
	}
	tail, err := strconv.Atoi(c.DefaultQuery("tail", "100"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_TAIL", "error": "tail 必须是整数"})
		return
	}
	if tail < 10 || tail > 5000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_TAIL", "error": "tail 必须在 10 到 5000 之间"})
		return
	}
	if err := h.FileLogs.ValidateFollow(c.Query("base"), c.Query("path"), tail); err != nil {
		respondFileLogError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "STREAM_UNSUPPORTED", "error": "不支持流式输出"})
		return
	}

	emit := func(event filelog.StreamEvent) error {
		switch event.Type {
		case "status":
			payload, marshalErr := json.Marshal(map[string]string{"state": event.State})
			if marshalErr != nil {
				return marshalErr
			}
			if _, writeErr := fmt.Fprintf(c.Writer, "event: status\ndata: %s\n\n", payload); writeErr != nil {
				return writeErr
			}
		case "heartbeat":
			if _, writeErr := fmt.Fprint(c.Writer, ": heartbeat\n\n"); writeErr != nil {
				return writeErr
			}
		case "line":
			if _, writeErr := fmt.Fprintf(c.Writer, "data: %s\n\n", event.Line); writeErr != nil {
				return writeErr
			}
		}
		flusher.Flush()
		return nil
	}

	err = h.FileLogs.Follow(c.Request.Context(), c.Query("base"), c.Query("path"), tail, emit)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("[FILELOG] 日志流已中止: base=%s path=%s err=%v", c.Query("base"), c.Query("path"), err)
	}
}

// DownloadFileLog GET /api/file-logs/download?base=:base&path=:path
func (h *Handler) DownloadFileLog(c *gin.Context) {
	if h.FileLogs == nil {
		respondFileLogError(c, filelog.ErrDisabled)
		return
	}
	file, info, err := h.FileLogs.OpenDownload(c.Query("base"), c.Query("path"))
	if err != nil {
		respondFileLogError(c, err)
		return
	}
	defer file.Close()

	fileName := filepath.Base(info.Name())
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
	if strings.EqualFold(filepath.Ext(fileName), ".gz") {
		contentType = "application/gzip"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, fileName, info.ModTime(), file)
}

func respondFileLogError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, filelog.ErrDisabled):
		c.JSON(http.StatusNotFound, gin.H{"code": "FILE_LOGS_DISABLED", "error": "文件日志功能未启用"})
	case errors.Is(err, filelog.ErrBaseNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "LOG_BASE_NOT_FOUND", "error": "日志安全基准目录不存在"})
	case errors.Is(err, filelog.ErrServiceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "SERVICE_NOT_FOUND", "error": "Compose 服务不存在"})
	case errors.Is(err, filelog.ErrInvalidPath):
		c.JSON(http.StatusForbidden, gin.H{"code": "INVALID_LOG_PATH", "error": "日志路径不合法"})
	case errors.Is(err, filelog.ErrFileNotAllowed):
		c.JSON(http.StatusForbidden, gin.H{"code": "LOG_FILE_NOT_ALLOWED", "error": "该文件类型不允许执行此操作"})
	case errors.Is(err, filelog.ErrLineTooLong):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"code": "LOG_LINE_TOO_LONG", "error": err.Error()})
	case errors.Is(err, os.ErrNotExist), errors.Is(err, context.Canceled):
		c.JSON(http.StatusNotFound, gin.H{"code": "LOG_FILE_NOT_FOUND", "error": "日志文件或目录不存在"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": "FILE_LOG_ERROR", "error": err.Error()})
	}
}

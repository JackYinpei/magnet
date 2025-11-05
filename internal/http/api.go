package http

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"magnet-player/internal/domain"
	"magnet-player/internal/downloader"
	"magnet-player/internal/service"
	"magnet-player/internal/storage"
)

// Handler wires HTTP routes to domain services.
type Handler struct {
	tasks    service.TaskService
	manager  downloader.Manager
	storage  storage.Service
	bucket   string
	dataRoot string
}

func NewHandler(tasks service.TaskService, manager downloader.Manager, store storage.Service, bucket, dataRoot string) *Handler {
	return &Handler{
		tasks:    tasks,
		manager:  manager,
		storage:  store,
		bucket:   bucket,
		dataRoot: dataRoot,
	}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.Use(corsMiddleware())

	api := router.Group("/api")
	{
		api.POST("/tasks", h.createTask)
		api.GET("/tasks", h.listTasks)
		api.GET("/tasks/:id", h.getTask)
		api.DELETE("/tasks/:id", h.deleteTask)
		api.GET("/storage/objects", h.listObjects)
		api.GET("/health", func(ctx *gin.Context) {
			ctx.JSON(http.StatusAccepted, gin.H{"ok": "ok"})
		})
	}
}

type createTaskRequest struct {
	Magnet string `json:"magnet" binding:"required"`
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func (h *Handler) createTask(c *gin.Context) {
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.tasks.CreateTask(c.Request.Context(), req.Magnet, h.dataRoot)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.manager.Enqueue(c.Request.Context(), task.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, taskToResponse(*task))
}

func (h *Handler) listTasks(c *gin.Context) {
	tasks, err := h.tasks.ListTasks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := make([]TaskResponse, len(tasks))
	for i := range tasks {
		resp[i] = taskToResponse(tasks[i])
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) getTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}

	task, err := h.tasks.GetTask(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, taskToResponse(*task))
}

func (h *Handler) deleteTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}

	deleteRemote, err := strconv.ParseBool(c.DefaultQuery("delete_remote", "false"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid flag delete_remote"})
		return
	}

	task, err := h.tasks.GetTask(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var warnings []string
	if h.manager != nil {
		cancelCtx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		if err := h.manager.Cancel(cancelCtx, task.ID); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			warnings = append(warnings, fmt.Sprintf("cancel task: %v", err))
		}
	}

	if deleteRemote {
		if h.storage == nil || h.bucket == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "storage service not configured"})
			return
		}
		if task.S3Location != "" {
			prefix, err := extractS3Prefix(task.S3Location, h.bucket)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if prefix != "" {
				remoteCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
				defer cancel()
				if err := h.storage.DeletePrefix(remoteCtx, h.bucket, prefix); err != nil {
					warnings = append(warnings, fmt.Sprintf("delete remote data: %v", err))
				}
			}
		}
	}

	warnings = append(warnings, h.cleanupLocalData(task)...)

	if err := h.tasks.DeleteTask(c.Request.Context(), task.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{"deleted": task.ID}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) listObjects(c *gin.Context) {
	if h.storage == nil || h.bucket == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage service not configured"})
		return
	}

	prefix := c.Query("prefix")
	objects, err := h.storage.ListObjects(c.Request.Context(), h.bucket, prefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := make([]StorageObjectResponse, len(objects))
	for i := range objects {
		resp[i] = objectToResponse(objects[i])
	}
	c.JSON(http.StatusOK, resp)
}

type TaskResponse struct {
	ID               int64              `json:"id"`
	Magnet           string             `json:"magnet"`
	Status           domain.TaskStatus  `json:"status"`
	Progress         int                `json:"progress"`
	Speed            int64              `json:"speed"`
	DownloadedBytes  int64              `json:"downloaded_bytes"`
	TotalSize        int64              `json:"total_size"`
	TotalPeers       int                `json:"total_peers"`
	ActivePeers      int                `json:"active_peers"`
	PendingPeers     int                `json:"pending_peers"`
	ConnectedSeeders int                `json:"connected_seeders"`
	HalfOpenPeers    int                `json:"half_open_peers"`
	TorrentName      string             `json:"torrent_name"`
	LocalPath        string             `json:"local_path"`
	S3Location       string             `json:"s3_location"`
	ErrorMessage     string             `json:"error_message"`
	CreatedAt        string             `json:"created_at"`
	UpdatedAt        string             `json:"updated_at"`
	DownloadedAt     *string            `json:"downloaded_at,omitempty"`
	UploadedAt       *string            `json:"uploaded_at,omitempty"`
	Files            []TaskFileResponse `json:"files"`
}

func (h *Handler) cleanupLocalData(task *domain.Task) []string {
	root := filepath.Clean(h.dataRoot)
	seen := make(map[string]struct{})
	var warnings []string

	addPath := func(p string, restrictToRoot bool) {
		if p == "" {
			return
		}
		clean := filepath.Clean(p)
		if clean == "" || clean == "." {
			return
		}
		if restrictToRoot {
			if root == "" {
				return
			}
			if rel, err := filepath.Rel(root, clean); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				return
			}
		} else if root != "" && clean == root {
			return
		}
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		if err := os.RemoveAll(clean); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("remove local data %s: %v", clean, err))
		}
	}

	addPath(task.LocalPath, false)
	if infoHash, err := infoHashFromMagnet(task.MagnetURI); err == nil {
		addPath(filepath.Join(root, infoHash), true)
	}

	return warnings
}

func infoHashFromMagnet(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "magnet" {
		return "", fmt.Errorf("invalid magnet URI scheme")
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", err
	}

	for _, xt := range values["xt"] {
		if !strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
			continue
		}
		hash := strings.TrimSpace(xt[len("urn:btih:"):])
		if len(hash) == 0 {
			continue
		}
		if len(hash) == 40 {
			if _, err := hex.DecodeString(hash); err == nil {
				return strings.ToLower(hash), nil
			}
		}

		encoding := base32.StdEncoding.WithPadding(base32.NoPadding)
		base32Value := strings.TrimRight(strings.ToUpper(hash), "=")
		decoded, err := encoding.DecodeString(base32Value)
		if err != nil || len(decoded) != 20 {
			continue
		}
		return hex.EncodeToString(decoded), nil
	}

	return "", fmt.Errorf("btih magnet xt not present")
}

type TaskFileResponse struct {
	ID       int64  `json:"id"`
	TaskID   int64  `json:"task_id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Priority int    `json:"priority"`
}

type StorageObjectResponse struct {
	Key          string  `json:"key"`
	Size         int64   `json:"size"`
	LastModified *string `json:"last_modified,omitempty"`
}

func objectToResponse(obj storage.ObjectInfo) StorageObjectResponse {
	resp := StorageObjectResponse{
		Key:  obj.Key,
		Size: obj.Size,
	}
	if obj.LastModified != nil && !obj.LastModified.IsZero() {
		v := obj.LastModified.Format(time.RFC3339)
		resp.LastModified = &v
	}
	return resp
}

func taskToResponse(task domain.Task) TaskResponse {
	resp := TaskResponse{
		ID:               task.ID,
		Magnet:           task.MagnetURI,
		Status:           task.Status,
		Progress:         task.Progress,
		Speed:            task.Speed,
		DownloadedBytes:  task.DownloadedBytes,
		TotalSize:        task.TotalSize,
		TotalPeers:       task.TotalPeers,
		ActivePeers:      task.ActivePeers,
		PendingPeers:     task.PendingPeers,
		ConnectedSeeders: task.ConnectedSeeders,
		HalfOpenPeers:    task.HalfOpenPeers,
		TorrentName:      task.TorrentName,
		LocalPath:        task.LocalPath,
		S3Location:       task.S3Location,
		ErrorMessage:     task.ErrorMessage,
		CreatedAt:        task.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        task.UpdatedAt.Format(time.RFC3339),
		Files:            make([]TaskFileResponse, len(task.Files)),
	}
	if task.DownloadedAt != nil {
		v := task.DownloadedAt.Format(time.RFC3339)
		resp.DownloadedAt = &v
	}
	if task.UploadedAt != nil {
		v := task.UploadedAt.Format(time.RFC3339)
		resp.UploadedAt = &v
	}

	for i := range task.Files {
		resp.Files[i] = TaskFileResponse{
			ID:       task.Files[i].ID,
			TaskID:   task.Files[i].TaskID,
			Name:     task.Files[i].Name,
			Path:     task.Files[i].Path,
			Size:     task.Files[i].Size,
			Priority: task.Files[i].Priority,
		}
	}
	return resp
}

func extractS3Prefix(location, bucket string) (string, error) {
	if !strings.HasPrefix(location, "s3://") {
		return "", fmt.Errorf("invalid s3 location")
	}
	rest := strings.TrimPrefix(location, "s3://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", fmt.Errorf("invalid s3 location")
	}
	if bucket != "" && parts[0] != bucket {
		return "", fmt.Errorf("s3 bucket mismatch")
	}
	if len(parts) == 1 {
		return "", fmt.Errorf("s3 prefix missing")
	}
	return strings.TrimPrefix(parts[1], "/"), nil
}

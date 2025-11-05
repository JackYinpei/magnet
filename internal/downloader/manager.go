package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/sirupsen/logrus"

	"magnet-player/internal/domain"
	"magnet-player/internal/service"
	"magnet-player/internal/storage"
)

// Manager coordinates torrent downloads, progress tracking, and upload lifecycle.
type Manager interface {
	Start(ctx context.Context) error
	Shutdown()
	Enqueue(ctx context.Context, taskID int64) error
	Resume(ctx context.Context) error
	Cancel(ctx context.Context, taskID int64) error
}

type Config struct {
	DownloadRoot   string
	MaxConcurrent  int
	StatusInterval time.Duration
	TrackerList    []string
	UploadOptions  storage.UploadOptions
	Logger         *logrus.Logger
}

type manager struct {
	cfg         Config
	client      *torrent.Client
	taskService service.TaskService
	storage     storage.Service

	sem    chan struct{}
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	active map[int64]*taskHandle
}

type taskHandle struct {
	cancel  context.CancelFunc
	torrent *torrent.Torrent
	done    chan struct{}
}

func NewManager(cfg Config, taskService service.TaskService, storage storage.Service) Manager {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 3
	}
	if cfg.StatusInterval == 0 {
		cfg.StatusInterval = 2 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.New()
	}
	if len(cfg.TrackerList) == 0 {
		cfg.TrackerList = defaultTrackers()
	}
	return &manager{
		cfg:         cfg,
		taskService: taskService,
		storage:     storage,
		sem:         make(chan struct{}, cfg.MaxConcurrent),
		active:      make(map[int64]*taskHandle),
	}
}

func (m *manager) Start(ctx context.Context) error {
	if err := os.MkdirAll(m.cfg.DownloadRoot, 0o755); err != nil {
		return fmt.Errorf("create download root: %w", err)
	}

	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = m.cfg.DownloadRoot
	clientConfig.NoUpload = false
	clientConfig.Seed = false

	client, err := torrent.NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("create torrent client: %w", err)
	}

	m.client = client
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.cfg.Logger.Infof("download manager started, data dir: %s", m.cfg.DownloadRoot)
	return nil
}

func (m *manager) Shutdown() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	if m.client != nil {
		m.client.Close()
	}
	m.cfg.Logger.Info("download manager stopped")
}

func (m *manager) Enqueue(ctx context.Context, taskID int64) error {
	task, err := m.taskService.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	m.spawnTask(*task)
	return nil
}

func (m *manager) Resume(ctx context.Context) error {
	tasks, err := m.taskService.ListByStatuses(ctx,
		domain.TaskStatusPending,
		domain.TaskStatusDownloading,
		domain.TaskStatusDownloaded,
		domain.TaskStatusUploading,
	)
	if err != nil {
		return err
	}

	for i := range tasks {
		m.spawnTask(tasks[i])
	}
	return nil
}

func (m *manager) spawnTask(task domain.Task) {
	taskCtx, cancel := context.WithCancel(m.ctx)
	handle := &taskHandle{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	m.registerTask(task.ID, handle)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.unregisterTask(task.ID)
			close(handle.done)
		}()
		select {
		case <-m.ctx.Done():
			return
		case <-taskCtx.Done():
			return
		case m.sem <- struct{}{}:
			defer func() { <-m.sem }()
			m.handleTask(taskCtx, handle, &task)
		}
	}()
}

func (m *manager) registerTask(id int64, handle *taskHandle) {
	m.mu.Lock()
	m.active[id] = handle
	m.mu.Unlock()
}

func (m *manager) unregisterTask(id int64) {
	m.mu.Lock()
	delete(m.active, id)
	m.mu.Unlock()
}

func (m *manager) setTaskTorrent(id int64, t *torrent.Torrent) {
	m.mu.Lock()
	if handle, ok := m.active[id]; ok {
		handle.torrent = t
	}
	m.mu.Unlock()
}

func (m *manager) getTaskHandle(id int64) (*taskHandle, bool) {
	m.mu.Lock()
	handle, ok := m.active[id]
	m.mu.Unlock()
	return handle, ok
}

func (m *manager) Cancel(ctx context.Context, taskID int64) error {
	handle, ok := m.getTaskHandle(taskID)
	if !ok {
		return nil
	}

	handle.cancel()

	m.mu.Lock()
	t := handle.torrent
	m.mu.Unlock()
	if t != nil {
		t.Drop()
	}

	select {
	case <-handle.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *manager) handleTask(ctx context.Context, handle *taskHandle, task *domain.Task) {
	logger := m.cfg.Logger.WithField("task_id", task.ID)
	switch task.Status {
	case domain.TaskStatusCompleted:
		logger.Debug("task already completed, skipping")
		return
	case domain.TaskStatusDownloaded:
		logger.Info("task already downloaded, resuming upload")
		m.uploadAndCleanup(ctx, task)
		return
	case domain.TaskStatusUploading:
		logger.Info("task mid-upload, resuming upload")
		m.uploadAndCleanup(ctx, task)
		return
	}

	if err := m.taskService.UpdateStatus(ctx, task.ID, domain.TaskStatusDownloading, nil); err != nil {
		logger.Errorf("update status failed: %v", err)
		return
	}
	task.Status = domain.TaskStatusDownloading

	t, err := m.client.AddMagnet(task.MagnetURI)
	if err != nil {
		m.failTask(ctx, task.ID, fmt.Errorf("add magnet: %w", err))
		return
	}
	defer t.Drop()
	m.setTaskTorrent(task.ID, t)

	for _, tracker := range m.cfg.TrackerList {
		t.AddTrackers([][]string{{tracker}})
	}

	select {
	case <-ctx.Done():
		logger.Info("task cancelled before fetching metadata")
		return
	case <-t.GotInfo():
	}

	info := t.Info()
	if info == nil {
		m.failTask(ctx, task.ID, fmt.Errorf("missing torrent info"))
		return
	}

	totalLength := info.TotalLength()
	name := info.BestName()
	localPath := filepath.Join(m.cfg.DownloadRoot, name)
	task.LocalPath = localPath

	if err := m.taskService.UpdateDownloadInfo(ctx, task.ID, name, localPath, totalLength); err != nil {
		logger.Errorf("update download info: %v", err)
	}

	files := make([]domain.TaskFile, len(t.Files()))
	for i, file := range t.Files() {
		files[i] = domain.TaskFile{
			TaskID: task.ID,
			Name:   file.DisplayPath(),
			Path:   file.Path(),
			Size:   file.Length(),
			Priority: func() int {
				if file.Priority() > 0 {
					return int(file.Priority())
				}
				return 1
			}(),
		}
	}
	if err := m.taskService.ReplaceFiles(ctx, task.ID, files); err != nil {
		logger.Warnf("replace files: %v", err)
	}

	t.DownloadAll()

	lastBytes := int64(0)
	lastTime := time.Now()

	ticker := time.NewTicker(m.cfg.StatusInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("task cancelled")
			return
		case <-ticker.C:
			bytesCompleted := t.BytesCompleted()
			progress := 0
			if totalLength > 0 {
				progress = int((bytesCompleted * 100) / totalLength)
			}
			elapsed := time.Since(lastTime).Seconds()
			speed := int64(0)
			if elapsed > 0 {
				speed = (bytesCompleted - lastBytes) / int64(elapsed)
			}
			lastBytes = bytesCompleted
			lastTime = time.Now()

			stats := t.Stats()

			if err := m.taskService.UpdateProgress(ctx, task.ID, progress, speed, bytesCompleted, stats.TotalPeers, stats.ActivePeers, stats.PendingPeers, stats.ConnectedSeeders, stats.HalfOpenPeers); err != nil {
				logger.Warnf("update progress: %v", err)
			}

			if progress >= 100 || t.BytesMissing() == 0 {
				if err := m.taskService.MarkDownloaded(ctx, task.ID); err != nil {
					logger.Warnf("mark downloaded: %v", err)
				}
				task.Status = domain.TaskStatusDownloaded
				logger.Info("download completed")
				m.uploadAndCleanup(ctx, task)
				return
			}
		}
	}
}

func (m *manager) uploadAndCleanup(ctx context.Context, task *domain.Task) {
	logger := m.cfg.Logger.WithField("task_id", task.ID)

	if err := m.taskService.UpdateStatus(ctx, task.ID, domain.TaskStatusUploading, nil); err != nil {
		logger.Errorf("set uploading status: %v", err)
		return
	}
	task.Status = domain.TaskStatusUploading

	localPath := task.LocalPath
	if localPath == "" {
		localPath = filepath.Join(m.cfg.DownloadRoot, fmt.Sprintf("task-%d", task.ID))
	}
	info, err := os.Stat(localPath)
	if err != nil {
		fallback := filepath.Join(m.cfg.DownloadRoot, task.TorrentName)
		if fallback != "" && fallback != localPath {
			if fbInfo, fbErr := os.Stat(fallback); fbErr == nil {
				localPath = fallback
				task.LocalPath = fallback
				info = fbInfo
			} else {
				m.failTask(ctx, task.ID, fmt.Errorf("local data missing: %w", err))
				return
			}
		} else {
			m.failTask(ctx, task.ID, fmt.Errorf("local data missing: %w", err))
			return
		}
	}

	if !info.IsDir() {
		stagingDir := filepath.Join(m.cfg.DownloadRoot, fmt.Sprintf("task-%d", task.ID))
		if err := os.MkdirAll(stagingDir, 0o755); err != nil {
			m.failTask(ctx, task.ID, fmt.Errorf("create staging dir: %w", err))
			return
		}
		dest := filepath.Join(stagingDir, filepath.Base(localPath))
		if err := os.Rename(localPath, dest); err != nil {
			if copyErr := copyFile(localPath, dest); copyErr != nil {
				m.failTask(ctx, task.ID, fmt.Errorf("prepare upload data: %w", copyErr))
				return
			}
			if removeErr := os.Remove(localPath); removeErr != nil && !os.IsNotExist(removeErr) {
				logger.Warnf("remove original file after copy: %v", removeErr)
			}
		}
		localPath = stagingDir
		task.LocalPath = stagingDir
		if err := m.taskService.UpdateDownloadInfo(ctx, task.ID, task.TorrentName, stagingDir, task.TotalSize); err != nil {
			logger.Warnf("refresh local path: %v", err)
		}
	}

	opts := m.cfg.UploadOptions
	prefix := strings.Trim(opts.KeyPrefix, "/")
	taskPrefix := fmt.Sprintf("task-%d", task.ID)
	if prefix == "" {
		opts.KeyPrefix = taskPrefix
	} else {
		opts.KeyPrefix = fmt.Sprintf("%s/%s", prefix, taskPrefix)
	}

	progressLogger := newUploadProgressLogger(logger)
	opts.ProgressCallback = func(done, total int64) {
		progressLogger(done, total)
	}

	logger.Infof("upload started from %s", localPath)

	dest, err := m.storage.UploadDirectory(ctx, localPath, opts)
	if err != nil {
		m.failTask(ctx, task.ID, fmt.Errorf("upload: %w", err))
		return
	}

	if err := m.taskService.MarkUploaded(ctx, task.ID, dest); err != nil {
		logger.Errorf("mark uploaded: %v", err)
		return
	}
	task.Status = domain.TaskStatusCompleted

	if err := os.RemoveAll(localPath); err != nil {
		logger.Warnf("cleanup download dir: %v", err)
	}

	logger.Infof("task completed and uploaded to %s", dest)
}

func (m *manager) failTask(ctx context.Context, taskID int64, failErr error) {
	msg := failErr.Error()
	if err := m.taskService.UpdateStatus(ctx, taskID, domain.TaskStatusFailed, &msg); err != nil {
		m.cfg.Logger.WithField("task_id", taskID).Errorf("persist failure status: %v", err)
	}
	m.cfg.Logger.WithField("task_id", taskID).Error(msg)
}

func infoHashToDir(hash metainfo.Hash) string {
	return hash.HexString()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy file: %w", err)
	}

	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync destination: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination: %w", err)
	}
	return nil
}

func newUploadProgressLogger(logger *logrus.Entry) func(done, total int64) {
	var (
		lastLog time.Time
	)
	return func(done, total int64) {
		now := time.Now()
		if total == 0 {
			if now.Sub(lastLog) < 500*time.Millisecond && done != 0 {
				return
			}
			lastLog = now
			logger.Infof("upload progress: %s uploaded", formatBytes(done))
			return
		}

		percent := float64(done) / float64(total) * 100
		if now.Sub(lastLog) < 500*time.Millisecond && done != total {
			return
		}
		lastLog = now
		logger.Infof("upload progress: %.1f%% (%s/%s)", percent, formatBytes(done), formatBytes(total))
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB",
		float64(b)/float64(div),
		"KMGTPE"[exp],
	)
}

func defaultTrackers() []string {
	return []string{
		"udp://tracker.opentrackr.org:1337/announce",
		"udp://tracker.openbittorrent.com:6969/announce",
		"udp://open.stealth.si:80/announce",
		"udp://exodus.desync.com:6969/announce",
		"http://tracker.opentrackr.org:1337/announce",
		"http://tracker.openbittorrent.com:80/announce",
		"udp://tracker.torrent.eu.org:451/announce",
		"udp://tracker.moeking.me:6969/announce",
	}
}

var _ Manager = (*manager)(nil)

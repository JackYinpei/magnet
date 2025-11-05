package domain

import "time"

type TaskStatus string

const (
	TaskStatusPending     TaskStatus = "pending"
	TaskStatusDownloading TaskStatus = "downloading"
	TaskStatusPaused      TaskStatus = "paused"
	TaskStatusDownloaded  TaskStatus = "downloaded"
	TaskStatusUploading   TaskStatus = "uploading"
	TaskStatusCompleted   TaskStatus = "completed"
	TaskStatusFailed      TaskStatus = "failed"
)

// Task represents a magnet download task tracked by the system.
type Task struct {
	ID               int64
	MagnetURI        string
	Status           TaskStatus
	Progress         int
	Speed            int64
	DownloadedBytes  int64
	TotalSize        int64
	TotalPeers       int
	ActivePeers      int
	PendingPeers     int
	ConnectedSeeders int
	HalfOpenPeers    int
	TorrentName      string
	LocalPath        string
	S3Location       string
	ErrorMessage     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DownloadedAt     *time.Time
	UploadedAt       *time.Time
	Files            []TaskFile
}

// TaskFile captures an individual file discovered within a torrent.
type TaskFile struct {
	ID       int64
	TaskID   int64
	Name     string
	Size     int64
	Path     string
	Priority int
}

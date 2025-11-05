package repository

import (
	"context"
	"time"

	"magnet-player/internal/domain"
)

// TaskRepository exposes persistence operations for Task aggregates.
type TaskRepository interface {
	Init(ctx context.Context) error
	Create(ctx context.Context, task *domain.Task) (int64, error)
	Update(ctx context.Context, task *domain.Task) error
	UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus, errorMessage *string) error
	UpdateProgress(ctx context.Context, id int64, progress int, speed int64, downloaded int64, totalPeers, activePeers, pendingPeers, connectedSeeders, halfOpenPeers int) error
	UpdateDownloadInfo(ctx context.Context, id int64, name, localPath string, totalSize int64) error
	MarkDownloaded(ctx context.Context, id int64, completedAt time.Time) error
	MarkUploaded(ctx context.Context, id int64, s3Location string, uploadedAt time.Time) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*domain.Task, error)
	List(ctx context.Context) ([]domain.Task, error)
	ListByStatuses(ctx context.Context, statuses ...domain.TaskStatus) ([]domain.Task, error)
}

// TaskFileRepository manages torrent file metadata.
type TaskFileRepository interface {
	Init(ctx context.Context) error
	ReplaceForTask(ctx context.Context, taskID int64, files []domain.TaskFile) error
	ListByTask(ctx context.Context, taskID int64) ([]domain.TaskFile, error)
}

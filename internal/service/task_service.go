package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"magnet-player/internal/domain"
	"magnet-player/internal/repository"
)

// TaskService coordinates task level operations backed by repositories.
type TaskService interface {
	CreateTask(ctx context.Context, magnetURI, dataRoot string) (*domain.Task, error)
	GetTask(ctx context.Context, id int64) (*domain.Task, error)
	ListTasks(ctx context.Context) ([]domain.Task, error)
	ListByStatuses(ctx context.Context, statuses ...domain.TaskStatus) ([]domain.Task, error)
	UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus, errMsg *string) error
	UpdateDownloadInfo(ctx context.Context, id int64, torrentName, localPath string, totalSize int64) error
	UpdateProgress(ctx context.Context, id int64, progress int, speed, downloaded int64, totalPeers, activePeers, pendingPeers, connectedSeeders, halfOpenPeers int) error
	MarkDownloaded(ctx context.Context, id int64) error
	MarkUploaded(ctx context.Context, id int64, s3Location string) error
	DeleteTask(ctx context.Context, id int64) error
	ReplaceFiles(ctx context.Context, taskID int64, files []domain.TaskFile) error
}

type taskService struct {
	tasks repository.TaskRepository
	files repository.TaskFileRepository
}

func NewTaskService(tasks repository.TaskRepository, files repository.TaskFileRepository) TaskService {
	return &taskService{
		tasks: tasks,
		files: files,
	}
}

func (s *taskService) CreateTask(ctx context.Context, magnetURI, dataRoot string) (*domain.Task, error) {
	if magnetURI == "" {
		return nil, errors.New("magnet URI is required")
	}

	task := &domain.Task{
		MagnetURI: magnetURI,
		Status:    domain.TaskStatusPending,
		LocalPath: filepath.Join(dataRoot, fmt.Sprintf("task-%s", uuid.NewString())),
	}

	if _, err := s.tasks.Create(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *taskService) GetTask(ctx context.Context, id int64) (*domain.Task, error) {
	task, err := s.tasks.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	files, err := s.files.ListByTask(ctx, id)
	if err != nil {
		return nil, err
	}
	task.Files = files
	return task, nil
}

func (s *taskService) ListTasks(ctx context.Context) ([]domain.Task, error) {
	tasks, err := s.tasks.List(ctx)
	if err != nil {
		return nil, err
	}

	for i := range tasks {
		files, err := s.files.ListByTask(ctx, tasks[i].ID)
		if err != nil {
			return nil, err
		}
		tasks[i].Files = files
	}

	return tasks, nil
}

func (s *taskService) ListByStatuses(ctx context.Context, statuses ...domain.TaskStatus) ([]domain.Task, error) {
	tasks, err := s.tasks.ListByStatuses(ctx, statuses...)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		files, err := s.files.ListByTask(ctx, tasks[i].ID)
		if err != nil {
			return nil, err
		}
		tasks[i].Files = files
	}
	return tasks, nil
}

func (s *taskService) UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus, errMsg *string) error {
	return s.tasks.UpdateStatus(ctx, id, status, errMsg)
}

func (s *taskService) UpdateDownloadInfo(ctx context.Context, id int64, torrentName, localPath string, totalSize int64) error {
	return s.tasks.UpdateDownloadInfo(ctx, id, torrentName, localPath, totalSize)
}

func (s *taskService) UpdateProgress(ctx context.Context, id int64, progress int, speed, downloaded int64, totalPeers, activePeers, pendingPeers, connectedSeeders, halfOpenPeers int) error {
	return s.tasks.UpdateProgress(ctx, id, progress, speed, downloaded, totalPeers, activePeers, pendingPeers, connectedSeeders, halfOpenPeers)
}

func (s *taskService) MarkDownloaded(ctx context.Context, id int64) error {
	return s.tasks.MarkDownloaded(ctx, id, time.Now())
}

func (s *taskService) MarkUploaded(ctx context.Context, id int64, s3Location string) error {
	return s.tasks.MarkUploaded(ctx, id, s3Location, time.Now())
}

func (s *taskService) DeleteTask(ctx context.Context, id int64) error {
	return s.tasks.Delete(ctx, id)
}

func (s *taskService) ReplaceFiles(ctx context.Context, taskID int64, files []domain.TaskFile) error {
	return s.files.ReplaceForTask(ctx, taskID, files)
}

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"magnet-player/internal/domain"
	"magnet-player/internal/repository"
)

const (
	createTasksTable = `
CREATE TABLE IF NOT EXISTS tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	magnet_uri TEXT NOT NULL,
	status TEXT NOT NULL,
	progress INTEGER NOT NULL DEFAULT 0,
	speed INTEGER NOT NULL DEFAULT 0,
	downloaded_bytes INTEGER NOT NULL DEFAULT 0,
	total_size INTEGER NOT NULL DEFAULT 0,
	total_peers INTEGER NOT NULL DEFAULT 0,
	active_peers INTEGER NOT NULL DEFAULT 0,
	pending_peers INTEGER NOT NULL DEFAULT 0,
	connected_seeders INTEGER NOT NULL DEFAULT 0,
	half_open_peers INTEGER NOT NULL DEFAULT 0,
	torrent_name TEXT NOT NULL DEFAULT '',
	local_path TEXT NOT NULL DEFAULT '',
	s3_location TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	downloaded_at DATETIME NULL,
	uploaded_at DATETIME NULL
);
`
)

type TaskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) repository.TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Init(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, createTasksTable); err != nil {
		return fmt.Errorf("create tasks table: %w", err)
	}
	if err := r.ensureTaskColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (r *TaskRepository) ensureTaskColumns(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `PRAGMA table_info(tasks)`)
	if err != nil {
		return fmt.Errorf("describe tasks table: %w", err)
	}
	defer rows.Close()

	columns := map[string]struct{}{}
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan pragma table info: %w", err)
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pragma table info: %w", err)
	}

	addColumn := func(name, statement string) error {
		if _, exists := columns[name]; exists {
			return nil
		}
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("add column %s: %w", name, err)
		}
		return nil
	}

	if err := addColumn("total_peers", `ALTER TABLE tasks ADD COLUMN total_peers INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := addColumn("active_peers", `ALTER TABLE tasks ADD COLUMN active_peers INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := addColumn("pending_peers", `ALTER TABLE tasks ADD COLUMN pending_peers INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := addColumn("connected_seeders", `ALTER TABLE tasks ADD COLUMN connected_seeders INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := addColumn("half_open_peers", `ALTER TABLE tasks ADD COLUMN half_open_peers INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	return nil
}

func (r *TaskRepository) Create(ctx context.Context, task *domain.Task) (int64, error) {
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	res, err := r.db.ExecContext(ctx, `
INSERT INTO tasks (magnet_uri, status, progress, speed, downloaded_bytes, total_size, total_peers, active_peers, pending_peers, connected_seeders, half_open_peers, torrent_name, local_path, s3_location, error_message, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.MagnetURI,
		string(task.Status),
		task.Progress,
		task.Speed,
		task.DownloadedBytes,
		task.TotalSize,
		task.TotalPeers,
		task.ActivePeers,
		task.PendingPeers,
		task.ConnectedSeeders,
		task.HalfOpenPeers,
		task.TorrentName,
		task.LocalPath,
		task.S3Location,
		task.ErrorMessage,
		task.CreatedAt,
		task.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("insert task: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	task.ID = id
	return id, nil
}

func (r *TaskRepository) Update(ctx context.Context, task *domain.Task) error {
	task.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE tasks
SET magnet_uri=?, status=?, progress=?, speed=?, downloaded_bytes=?, total_size=?, total_peers=?, active_peers=?, pending_peers=?, connected_seeders=?, half_open_peers=?, torrent_name=?, local_path=?, s3_location=?, error_message=?, created_at=?, updated_at=?, downloaded_at=?, uploaded_at=?
WHERE id=?`,
		task.MagnetURI,
		string(task.Status),
		task.Progress,
		task.Speed,
		task.DownloadedBytes,
		task.TotalSize,
		task.TotalPeers,
		task.ActivePeers,
		task.PendingPeers,
		task.ConnectedSeeders,
		task.HalfOpenPeers,
		task.TorrentName,
		task.LocalPath,
		task.S3Location,
		task.ErrorMessage,
		task.CreatedAt.UTC(),
		task.UpdatedAt,
		nullTime(task.DownloadedAt),
		nullTime(task.UploadedAt),
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (r *TaskRepository) UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus, errorMessage *string) error {
	now := time.Now().UTC()
	msg := ""
	if errorMessage != nil {
		msg = *errorMessage
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE tasks
SET status=?, error_message=?, updated_at=?
WHERE id=?`,
		string(status),
		msg,
		now,
		id,
	)
	if err != nil {
		return fmt.Errorf("update task status: %w", err)
	}
	return nil
}

func (r *TaskRepository) UpdateProgress(ctx context.Context, id int64, progress int, speed int64, downloaded int64, totalPeers, activePeers, pendingPeers, connectedSeeders, halfOpenPeers int) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE tasks
SET progress=?, speed=?, downloaded_bytes=?, total_peers=?, active_peers=?, pending_peers=?, connected_seeders=?, half_open_peers=?, updated_at=?
WHERE id=?`,
		progress,
		speed,
		downloaded,
		totalPeers,
		activePeers,
		pendingPeers,
		connectedSeeders,
		halfOpenPeers,
		now,
		id,
	)
	if err != nil {
		return fmt.Errorf("update task progress: %w", err)
	}
	return nil
}

func (r *TaskRepository) UpdateDownloadInfo(ctx context.Context, id int64, name, localPath string, totalSize int64) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE tasks
SET torrent_name=?, local_path=?, total_size=?, updated_at=?
WHERE id=?`,
		name,
		localPath,
		totalSize,
		now,
		id,
	)
	if err != nil {
		return fmt.Errorf("update download info: %w", err)
	}
	return nil
}

func (r *TaskRepository) MarkDownloaded(ctx context.Context, id int64, completedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE tasks
SET status=?, downloaded_at=?, updated_at=?
WHERE id=?`,
		string(domain.TaskStatusDownloaded),
		completedAt.UTC(),
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("mark downloaded: %w", err)
	}
	return nil
}

func (r *TaskRepository) MarkUploaded(ctx context.Context, id int64, s3Location string, uploadedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE tasks
SET status=?, s3_location=?, uploaded_at=?, updated_at=?
WHERE id=?`,
		string(domain.TaskStatusCompleted),
		s3Location,
		uploadedAt.UTC(),
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("mark uploaded: %w", err)
	}
	return nil
}

func (r *TaskRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM task_files WHERE task_id=?`, id); err != nil {
		return fmt.Errorf("delete task files: %w", err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("task delete rows affected: %w", err)
	}
	if aff == 0 {
		return fmt.Errorf("task not found")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task delete: %w", err)
	}
	return nil
}

func (r *TaskRepository) Get(ctx context.Context, id int64) (*domain.Task, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, magnet_uri, status, progress, speed, downloaded_bytes, total_size, total_peers, active_peers, pending_peers, connected_seeders, half_open_peers, torrent_name, local_path, s3_location, error_message, created_at, updated_at, downloaded_at, uploaded_at
FROM tasks
WHERE id=?`,
		id,
	)

	task, err := scanTask(row)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (r *TaskRepository) List(ctx context.Context) ([]domain.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, magnet_uri, status, progress, speed, downloaded_bytes, total_size, total_peers, active_peers, pending_peers, connected_seeders, half_open_peers, torrent_name, local_path, s3_location, error_message, created_at, updated_at, downloaded_at, uploaded_at
FROM tasks
ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *task)
	}

	return tasks, rows.Err()
}

func (r *TaskRepository) ListByStatuses(ctx context.Context, statuses ...domain.TaskStatus) ([]domain.Task, error) {
	if len(statuses) == 0 {
		return []domain.Task{}, nil
	}

	placeholders := make([]string, len(statuses))
	args := make([]interface{}, len(statuses))
	for i, status := range statuses {
		placeholders[i] = "?"
		args[i] = string(status)
	}

	query := fmt.Sprintf(`
SELECT id, magnet_uri, status, progress, speed, downloaded_bytes, total_size, total_peers, active_peers, pending_peers, connected_seeders, half_open_peers, torrent_name, local_path, s3_location, error_message, created_at, updated_at, downloaded_at, uploaded_at
FROM tasks
WHERE status IN (%s)
ORDER BY id ASC`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tasks by status: %w", err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *task)
	}

	return tasks, rows.Err()
}

func scanTask(scanner interface {
	Scan(dest ...any) error
}) (*domain.Task, error) {
	var (
		task              domain.Task
		status            string
		createdAt         time.Time
		updatedAt         time.Time
		downloadedAtValid sql.NullTime
		uploadedAtValid   sql.NullTime
	)

	if err := scanner.Scan(
		&task.ID,
		&task.MagnetURI,
		&status,
		&task.Progress,
		&task.Speed,
		&task.DownloadedBytes,
		&task.TotalSize,
		&task.TotalPeers,
		&task.ActivePeers,
		&task.PendingPeers,
		&task.ConnectedSeeders,
		&task.HalfOpenPeers,
		&task.TorrentName,
		&task.LocalPath,
		&task.S3Location,
		&task.ErrorMessage,
		&createdAt,
		&updatedAt,
		&downloadedAtValid,
		&uploadedAtValid,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found")
		}
		return nil, fmt.Errorf("scan task: %w", err)
	}

	task.Status = domain.TaskStatus(status)
	task.CreatedAt = createdAt.Local()
	task.UpdatedAt = updatedAt.Local()
	if downloadedAtValid.Valid {
		t := downloadedAtValid.Time.Local()
		task.DownloadedAt = &t
	}
	if uploadedAtValid.Valid {
		t := uploadedAtValid.Time.Local()
		task.UploadedAt = &t
	}

	return &task, nil
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

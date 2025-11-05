package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"magnet-player/internal/domain"
	"magnet-player/internal/repository"
)

const createTaskFilesTable = `
CREATE TABLE IF NOT EXISTS task_files (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	size INTEGER NOT NULL,
	path TEXT NOT NULL,
	priority INTEGER NOT NULL DEFAULT 1,
	FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_task_files_task_id ON task_files(task_id);
`

type TaskFileRepository struct {
	db *sql.DB
}

func NewTaskFileRepository(db *sql.DB) repository.TaskFileRepository {
	return &TaskFileRepository{db: db}
}

func (r *TaskFileRepository) Init(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, createTaskFilesTable); err != nil {
		return fmt.Errorf("create task_files table: %w", err)
	}
	return nil
}

func (r *TaskFileRepository) ReplaceForTask(ctx context.Context, taskID int64, files []domain.TaskFile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // safe no-op on commit

	if _, err := tx.ExecContext(ctx, `DELETE FROM task_files WHERE task_id=?`, taskID); err != nil {
		return fmt.Errorf("delete files: %w", err)
	}

	for _, file := range files {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO task_files (task_id, name, size, path, priority)
VALUES (?, ?, ?, ?, ?)`,
			taskID,
			file.Name,
			file.Size,
			file.Path,
			file.Priority,
		); err != nil {
			return fmt.Errorf("insert file: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *TaskFileRepository) ListByTask(ctx context.Context, taskID int64) ([]domain.TaskFile, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, task_id, name, size, path, priority
FROM task_files
WHERE task_id=?
ORDER BY id ASC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("query task files: %w", err)
	}
	defer rows.Close()

	var files []domain.TaskFile
	for rows.Next() {
		var file domain.TaskFile
		if err := rows.Scan(&file.ID, &file.TaskID, &file.Name, &file.Size, &file.Path, &file.Priority); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

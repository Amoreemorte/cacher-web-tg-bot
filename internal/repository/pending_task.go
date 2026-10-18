package repository

import (
	"context"
	"time"
)

// PendingTask представляет отложенную задачу для повторной отправки в LLM.
type PendingTask struct {
	ID           int64
	UserID       int64
	RawText      string
	Attempts     int
	NextAttempt  time.Time
	CreatedAt    time.Time
}

// PendingTaskRepository определяет интерфейс для управления отложенными задачами LLM.
type PendingTaskRepository interface {
	CreatePendingTask(ctx context.Context, task *PendingTask) error
	ListDuePendingTasks(ctx context.Context, before time.Time) ([]*PendingTask, error)
	UpdatePendingTaskAttempts(ctx context.Context, id int64, attempts int, nextAttempt time.Time) error
	DeletePendingTask(ctx context.Context, id int64) error
}
package service

import (
	"context"
	"log"
	"time"

	"expense-bot/internal/llm"
	"expense-bot/internal/repository"
)

const (
	defaultMaxAttempts = 5
	defaultInterval    = 30 * time.Second
)

// PendingTaskProcessor обрабатывает отложенные задачи LLM:
// при старте бота и в фоновом режиме пытается повторно отправить задачи,
// для которых LLM была недоступна.
type PendingTaskProcessor struct {
	llmClient    llm.LLMClient
	expenseRepo  repository.ExpenseRepository
	pendingRepo  repository.PendingTaskRepository
	expenseSvc   *ExpenseService
	maxAttempts  int
	interval     time.Duration
}

// NewPendingTaskProcessor создаёт процессор отложенных задач.
func NewPendingTaskProcessor(
	llmClient llm.LLMClient,
	expenseRepo repository.ExpenseRepository,
	pendingRepo repository.PendingTaskRepository,
	expenseSvc *ExpenseService,
	maxAttempts int,
	interval time.Duration,
) *PendingTaskProcessor {
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	if interval <= 0 {
		interval = defaultInterval
	}
	return &PendingTaskProcessor{
		llmClient:   llmClient,
		expenseRepo: expenseRepo,
		pendingRepo: pendingRepo,
		expenseSvc:  expenseSvc,
		maxAttempts: maxAttempts,
		interval:    interval,
	}
}

// ProcessPending обрабатывает все отложенные задачи, срок которых наступил.
func (p *PendingTaskProcessor) ProcessPending(ctx context.Context) error {
	tasks, err := p.pendingRepo.ListDuePendingTasks(ctx, time.Now())
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if task.Attempts >= p.maxAttempts {
			// Превышен лимит попыток — удаляем задачу, чтобы не раздувать таблицу
			if delErr := p.pendingRepo.DeletePendingTask(ctx, task.ID); delErr != nil {
				log.Printf("failed to delete expired pending task %d: %v", task.ID, delErr)
			}
			continue
		}

		// Пытаемся выполнить задачу через LLM
		_, err := p.expenseSvc.ProcessText(ctx, task.UserID, task.RawText)
		if err != nil {
			if llm.IsLLMUnavailable(err) {
				// LLM всё ещё недоступна — сдвигаем время следующей попытки
				nextAttempt := time.Now().UTC().Add(p.interval * time.Duration(task.Attempts+1))
				if updErr := p.pendingRepo.UpdatePendingTaskAttempts(ctx, task.ID, task.Attempts+1, nextAttempt); updErr != nil {
					log.Printf("failed to update pending task %d: %v", task.ID, updErr)
				}
				log.Printf("pending task %d still unavailable, next attempt at %v", task.ID, nextAttempt)
			} else {
				// Не-LLM ошибка — удаляем задачу, чтобы не блокировать очередь
				log.Printf("pending task %d failed with non-LLM error: %v, deleting", task.ID, err)
				if delErr := p.pendingRepo.DeletePendingTask(ctx, task.ID); delErr != nil {
					log.Printf("failed to delete pending task %d: %v", task.ID, delErr)
				}
			}
			continue
		}

		// Успех — удаляем задачу из очереди
		if delErr := p.pendingRepo.DeletePendingTask(ctx, task.ID); delErr != nil {
			log.Printf("failed to delete completed pending task %d: %v", task.ID, delErr)
		}
		log.Printf("pending task %d completed successfully", task.ID)
	}

	return nil
}

// Start запускает фоновую горутину с тикером для периодической обработки задач.
func (p *PendingTaskProcessor) Start(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.ProcessPending(ctx); err != nil {
				log.Printf("pending processor tick error: %v", err)
			}
		}
	}
}
package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"expense-bot/internal/domain"
	"expense-bot/internal/llm"
	"expense-bot/internal/repository"
)

// ExpenseService orchestrates LLM processing and repository operations.
type ExpenseService struct {
	llmClient   llm.LLMClient
	repo        repository.ExpenseRepository
	pendingRepo repository.PendingTaskRepository
}

// NewExpenseService creates a new service.
func NewExpenseService(llmClient llm.LLMClient, repo repository.ExpenseRepository, pendingRepo repository.PendingTaskRepository) *ExpenseService {
	return &ExpenseService{
		llmClient:  llmClient,
		repo:       repo,
		pendingRepo: pendingRepo,
	}
}

// ProcessText handles an incoming text message from a user and returns a reply.
// It can be used both by the Telegram bot and by the pending task processor.
func (s *ExpenseService) ProcessText(ctx context.Context, userID int64, text string) (string, error) {
	// 1. Спрашиваем у LLM, что пользователь хочет сделать
	resp, err := s.llmClient.ProcessText(ctx, userID, text)
	if err != nil {
		// LLM недоступен — сохраняем в очередь
		_ = s.savePendingTask(ctx, userID, text)
		return "LLM не отвечает, попробуйте позже", nil
	}

	// 2. Выполняем действие в зависимости от ответа LLM
	switch resp.Action {
	case "add":
		return s.handleAdd(ctx, userID, resp)
	case "update":
		return s.handleUpdate(ctx, userID, resp)
	case "delete":
		return s.handleDelete(ctx, userID, resp)
	case "list":
		return s.handleList(ctx, userID, resp)
	default:
		return "Не понял запрос. Попробуйте написать иначе (например: «кофе 350»).", nil
	}
}

// ProcessMessage handles an incoming text message from a user (Telegram bot entry point).
func (s *ExpenseService) ProcessMessage(ctx context.Context, userID int64, text string) (string, error) {
	return s.ProcessText(ctx, userID, text)
}

func (s *ExpenseService) handleAdd(ctx context.Context, userID int64, resp *llm.LLMResponse) (string, error) {
	expense := &domain.Expense{
		UserID:   userID,
		Amount:   resp.Amount,
		Category: resp.Category,
		Comment:  resp.Comment,
	}

	// Парсим spent_at
	if resp.SpentAt != "" {
		t, err := time.Parse(time.RFC3339, resp.SpentAt)
		if err == nil {
			// Страховка: если LLM прислала полночь сегодняшнего дня (частая
			// ошибка моделей для «сегодня»/без даты), заменяем на текущее время.
			now := time.Now()
			if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 &&
				t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day() {
				expense.SpentAt = now
			} else {
				expense.SpentAt = t
			}
		} else {
			expense.SpentAt = time.Now().UTC()
		}
	} else {
		expense.SpentAt = time.Now().UTC()
	}

	if err := s.repo.Create(ctx, expense); err != nil {
		return "", fmt.Errorf("create expense: %w", err)
	}

	return fmt.Sprintf("Добавлено: %s %.2f на %s", expense.Category, expense.Amount, expense.SpentAt.Format("02.01.2006 15:04")), nil
}

func (s *ExpenseService) handleUpdate(ctx context.Context, userID int64, resp *llm.LLMResponse) (string, error) {
	if resp.ExpenseID == nil {
		return "Не указано, какую трату обновить.", nil
	}

	expense, err := s.repo.GetByID(ctx, *resp.ExpenseID)
	if err != nil {
		return "", fmt.Errorf("get expense by id: %w", err)
	}
	if expense == nil {
		return "Трата не найдена.", nil
	}
	if expense.UserID != userID {
		return "Чужие траты редактировать нельзя.", nil
	}

	if resp.Amount > 0 {
		expense.Amount = resp.Amount
	}
	if resp.Category != "" {
		expense.Category = resp.Category
	}
	if resp.Comment != "" {
		expense.Comment = resp.Comment
	}
	if resp.SpentAt != "" {
		if t, err := time.Parse(time.RFC3339, resp.SpentAt); err == nil {
			expense.SpentAt = t
		}
	}

	if err := s.repo.Update(ctx, expense); err != nil {
		return "", fmt.Errorf("update expense: %w", err)
	}

	return fmt.Sprintf("Обновлено: %s %.2f", expense.Category, expense.Amount), nil
}

func (s *ExpenseService) handleDelete(ctx context.Context, userID int64, resp *llm.LLMResponse) (string, error) {
	if resp.ExpenseID == nil {
		return "Не указано, какую трату удалить.", nil
	}

	expense, err := s.repo.GetByID(ctx, *resp.ExpenseID)
	if err != nil {
		return "", fmt.Errorf("get expense by id: %w", err)
	}
	if expense == nil {
		return "Трата не найдена.", nil
	}
	if expense.UserID != userID {
		return "Чужие траты удалять нельзя.", nil
	}

	if err := s.repo.Delete(ctx, *resp.ExpenseID); err != nil {
		return "", fmt.Errorf("delete expense: %w", err)
	}

	return "Трата удалена.", nil
}

func (s *ExpenseService) handleList(ctx context.Context, userID int64, resp *llm.LLMResponse) (string, error) {
	filter := domain.ExpenseFilter{
		UserID: &userID,
	}
	if resp.Category != "" {
		filter.Category = &resp.Category
	}
	if resp.SpentAt != "" {
		if t, err := time.Parse(time.RFC3339, resp.SpentAt); err == nil {
			filter.FromDate = &t
		}
	}

	expenses, err := s.repo.GetAll(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("get all expenses: %w", err)
	}

	if len(expenses) == 0 {
		return "Трат не найдено.", nil
	}

	var sb strings.Builder
	sb.WriteString("Ваши траты:\n")
	total := 0.0
	for _, e := range expenses {
		sb.WriteString(fmt.Sprintf("- %s %.2f (%s)\n", e.Category, e.Amount, e.SpentAt.Format("02.01.2006")))
		total += e.Amount
	}
	sb.WriteString(fmt.Sprintf("Итого: %.2f", total))

	return sb.String(), nil
}

func (s *ExpenseService) savePendingTask(ctx context.Context, userID int64, text string) error {
	task := &repository.PendingTask{
		UserID:     userID,
		RawText:    text,
		Attempts:   0,
		NextAttempt: time.Now().UTC().Add(time.Minute),
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.pendingRepo.CreatePendingTask(ctx, task); err != nil {
		log.Printf("failed to save pending task: %v", err)
		return err
	}
	return nil
}
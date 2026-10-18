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
func (s *ExpenseService) ProcessText(ctx context.Context, userID int64, rawText string) (string, error) {
	// 1. Обогащаем запрос списком последних трат — LLM нужен expense_id
	// для update/delete, а самого он взять негде (нет доступа к БД).
	llmText := s.withRecentExpenses(ctx, userID, rawText)

	// 2. Спрашиваем у LLM, что пользователь хочет сделать
	resp, err := s.llmClient.ProcessText(ctx, userID, llmText)
	if err != nil {
		// LLM недоступен — сохраняем в очередь исходный текст (без обогащения)
		_ = s.savePendingTask(ctx, userID, rawText)
		return "LLM не отвечает, попробуйте позже", nil
	}

	// 3. Выполняем действие в зависимости от ответа LLM
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

// withRecentExpenses добавляет к запросу список последних трат пользователя
// (максимум 10, самых новых), чтобы LLM могла выбрать expense_id для
// update/delete. Если трат нет — возвращает исходный текст без изменений.
func (s *ExpenseService) withRecentExpenses(ctx context.Context, userID int64, rawText string) string {
	filter := domain.ExpenseFilter{UserID: &userID, Limit: 10}
	expenses, err := s.repo.GetAll(ctx, filter)
	if err != nil || len(expenses) == 0 {
		return rawText
	}

	var sb strings.Builder
	sb.WriteString("Последние траты пользователя (выбирай expense_id ТОЛЬКО из этого списка; первая строка — самая новая):\n")
	for _, e := range expenses {
		fmt.Fprintf(&sb, "- id=%d | %.2f | %s | %s | %s\n",
			e.ID, e.Amount, e.Category, e.SpentAt.Format("2006-01-02"), e.Comment)
	}
	sb.WriteString("\nЗапрос пользователя: ")
	sb.WriteString(rawText)
	return sb.String()
}

// spentAtFromPeriod вычисляет время траты на сервере по семантической метке
// периода от LLM. Даты от LLM не принимаются вовсе — исключает галлюцинации:
//   - "" / "today" → текущее время;
//   - "yesterday" → текущее время вчерашнего дня;
//   - "week" / "month" (add без явной даты) → текущее время.
func spentAtFromPeriod(period string, now time.Time) time.Time {
	if period == "yesterday" {
		return now.AddDate(0, 0, -1)
	}
	return now
}

// periodRange вычисляет границы периода для фильтра списка на сервере.
// nil означает «без ограничения по дате».
func periodRange(period string, now time.Time) (from, to *time.Time) {
	dayStart := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
	switch period {
	case "today":
		f := dayStart(now)
		t := f.Add(24*time.Hour - time.Nanosecond)
		return &f, &t
	case "yesterday":
		f := dayStart(now.AddDate(0, 0, -1))
		t := f.Add(24*time.Hour - time.Nanosecond)
		return &f, &t
	case "week":
		f := now.AddDate(0, 0, -7)
		return &f, nil
	case "month":
		f := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		t := f.AddDate(0, 1, 0).Add(-time.Nanosecond)
		return &f, &t
	}
	return nil, nil
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

	// Дата траты вычисляется ТОЛЬКО на сервере — LLM дат не возвращает
	expense.SpentAt = spentAtFromPeriod(resp.Period, time.Now())

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
	// Дата траты при обновлении не меняется — период от LLM не влияет на update

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
	// Границы периода вычисляются на сервере по семантической метке от LLM
	from, to := periodRange(resp.Period, time.Now())
	filter.FromDate = from
	filter.ToDate = to

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
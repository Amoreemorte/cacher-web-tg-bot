package bot

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"expense-bot/internal/service"
)

// Sender defines the minimal interface for sending messages.
// It can be mocked in tests or replaced with another transport.
type Sender interface {
	Send(msg tgbotapi.Chattable) (tgbotapi.Message, error)
}

// UpdateProvider provides updates from Telegram.
type UpdateProvider interface {
	GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel
}

// Bot is the core bot logic. It depends only on interfaces,
// so any part can be replaced or mocked for testing.
type Bot struct {
	sender   Sender
	updates  UpdateProvider
	debug    bool
	service  *service.ExpenseService
}

// New creates a Bot with the given dependencies.
func New(sender Sender, updates UpdateProvider, debug bool, svc *service.ExpenseService) *Bot {
	return &Bot{
		sender:   sender,
		updates:  updates,
		debug:    debug,
		service:  svc,
	}
}

// Run starts long-polling and processes updates forever.
func (b *Bot) Run() error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.updates.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if update.Message.Text == "" {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Не получил задание, отправьте ещё раз")
			if _, sendErr := b.sender.Send(msg); sendErr != nil && b.debug {
				log.Printf("send error: %v", sendErr)
			}
			continue
		}

		log.Printf("Получено сообщение от пользователя %d: %s", update.Message.From.ID, update.Message.Text)

		reply, err := b.service.ProcessMessage(context.Background(), update.Message.From.ID, update.Message.Text)
		if err != nil {
			if b.debug {
				log.Printf("service error: %v", err)
			}
			reply = "Произошла внутренняя ошибка. Попробуйте позже."
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, reply)
		if _, sendErr := b.sender.Send(msg); sendErr != nil && b.debug {
			log.Printf("send error: %v", sendErr)
		}
	}

	return nil
}

// NewEchoBot creates a fully configured echo bot from the API object.
// It wraps tgbotapi.BotAPI into the Sender and UpdateProvider interfaces.
func NewEchoBot(api *tgbotapi.BotAPI, debug bool, svc *service.ExpenseService) *Bot {
	return New(api, api, debug, svc)
}

// Compile-time check that tgbotapi.BotAPI implements Sender and UpdateProvider.
var _ Sender = (*tgbotapi.BotAPI)(nil)
var _ UpdateProvider = (*tgbotapi.BotAPI)(nil)
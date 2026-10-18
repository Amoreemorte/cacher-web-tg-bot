package bot

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	handler  Handler
}

// Handler processes an incoming update and returns a reply text.
// Empty reply means no message should be sent.
type Handler interface {
	Handle(update tgbotapi.Update) (reply string, err error)
}

// echoHandler implements Handler: it echoes text messages and
// replies with a fallback message for non-text updates.
type echoHandler struct{}

func (h echoHandler) Handle(update tgbotapi.Update) (string, error) {
	if update.Message == nil {
		return "", nil
	}

	if update.Message.Text == "" {
		return "Не получил задание, отправьте ещё раз", nil
	}

	log.Printf("Получено сообщение от пользователя %d: %s", update.Message.From.ID, update.Message.Text)
	return update.Message.Text, nil
}

// New creates a Bot with the given dependencies.
func New(sender Sender, updates UpdateProvider, debug bool, handler Handler) *Bot {
	return &Bot{
		sender:   sender,
		updates:  updates,
		debug:    debug,
		handler:  handler,
	}
}

// Run starts long-polling and processes updates forever.
func (b *Bot) Run() error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.updates.GetUpdatesChan(u)

	for update := range updates {
		reply, err := b.handler.Handle(update)
		if err != nil {
			if b.debug {
				log.Printf("handler error: %v", err)
			}
			continue
		}

		if reply == "" {
			continue
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, reply)
		if _, sendErr := b.sender.Send(msg); sendErr != nil {
			if b.debug {
				log.Printf("send error: %v", sendErr)
			}
		}
	}

	return nil
}

// NewEchoBot creates a fully configured echo bot from the API object.
// It wraps tgbotapi.BotAPI into the Sender and UpdateProvider interfaces.
func NewEchoBot(api *tgbotapi.BotAPI, debug bool) *Bot {
	return New(api, api, debug, echoHandler{})
}

// Compile-time check that tgbotapi.BotAPI implements Sender and UpdateProvider.
var _ Sender = (*tgbotapi.BotAPI)(nil)
var _ UpdateProvider = (*tgbotapi.BotAPI)(nil)
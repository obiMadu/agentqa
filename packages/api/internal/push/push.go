package push

import (
	"context"
	"log"

	"github.com/agentqa/agentqa/packages/api/internal/config"
	"github.com/agentqa/agentqa/packages/api/internal/store"
)

type Message struct {
	UserID        string
	QuestionID    string
	InstallName   string
	QuestionCount int
	Preview       string
}

type PairingMessage struct {
	UserID      string
	InstallID   string
	InstallName string
}

type Sender interface {
	SendQuestion(ctx context.Context, msg Message) error
	SendPairingRequested(ctx context.Context, msg PairingMessage) error
	SendPairingPaired(ctx context.Context, msg PairingMessage) error
}

type DeviceStore interface {
	ListDevices(ctx context.Context, userID string) ([]store.Device, error)
	CountPendingQuestions(ctx context.Context, userID string) (int, error)
}

func New(cfg config.Config, devices DeviceStore, logger *log.Logger) Sender {
	if cfg.ExpoPushURL != "" && devices != nil {
		return newExpoSender(cfg.ExpoPushURL, devices, logger)
	}

	return &noopSender{logger: logger}
}

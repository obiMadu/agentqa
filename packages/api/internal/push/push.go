package push

import (
  "context"
  "log"

  "github.com/agentqa/agentqa/packages/api/internal/config"
)

type Message struct {
  UserID     string
  QuestionID string
  Preview    string
}

type Sender interface {
  SendQuestion(ctx context.Context, msg Message) error
}

func New(cfg config.Config, logger *log.Logger) Sender {
  if cfg.FCMServerKey != "" {
    return &fcmSender{serverKey: cfg.FCMServerKey, logger: logger}
  }

  return &noopSender{logger: logger}
}

package push

import (
  "context"
  "log"
)

type noopSender struct {
  logger *log.Logger
}

func (s *noopSender) SendQuestion(_ context.Context, msg Message) error {
  if s.logger != nil {
    s.logger.Printf("push disabled, skipping notify for %s", msg.QuestionID)
  }
  return nil
}

package push

import (
  "context"
  "log"
)

type fcmSender struct {
  serverKey string
  logger    *log.Logger
}

func (s *fcmSender) SendQuestion(_ context.Context, msg Message) error {
  if s.logger != nil {
    s.logger.Printf("fcm adapter is a stub, skipping notify for %s", msg.QuestionID)
  }
  return nil
}

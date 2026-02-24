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

func (s *noopSender) SendPairingRequested(_ context.Context, msg PairingMessage) error {
	if s.logger != nil {
		s.logger.Printf("push disabled, skipping pairing request for %s", msg.InstallID)
	}
	return nil
}

func (s *noopSender) SendPairingPaired(_ context.Context, msg PairingMessage) error {
	if s.logger != nil {
		s.logger.Printf("push disabled, skipping pairing success for %s", msg.InstallID)
	}
	return nil
}

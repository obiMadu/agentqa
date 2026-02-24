package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const expoMaxBatchSize = 100

type expoSender struct {
	endpoint string
	client   *http.Client
	devices  DeviceStore
	logger   *log.Logger
}

type expoPushMessage struct {
	To    string            `json:"to"`
	Title string            `json:"title,omitempty"`
	Body  string            `json:"body,omitempty"`
	Badge int               `json:"badge"`
	Data  map[string]string `json:"data,omitempty"`
}

type expoPushResponse struct {
	Data   []expoPushTicket `json:"data"`
	Errors []expoPushError  `json:"errors"`
}

type expoPushTicket struct {
	Status  string                 `json:"status"`
	ID      string                 `json:"id,omitempty"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type expoPushError struct {
	Code    string                 `json:"code,omitempty"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func newExpoSender(endpoint string, devices DeviceStore, logger *log.Logger) *expoSender {
	return &expoSender{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
		devices:  devices,
		logger:   logger,
	}
}

func (s *expoSender) SendQuestion(ctx context.Context, msg Message) error {
	return s.sendToUser(ctx, msg.UserID, questionTitle(msg.QuestionCount, msg.InstallName), msg.Preview, map[string]string{
		"question_id": msg.QuestionID,
	})
}

func (s *expoSender) SendPairingRequested(ctx context.Context, msg PairingMessage) error {
	name := displayAgentName(msg.InstallName)
	return s.sendToUser(ctx, msg.UserID, "Pairing request", fmt.Sprintf("%s wants to pair.", name), map[string]string{
		"event":      "pairing_requested",
		"install_id": msg.InstallID,
	})
}

func (s *expoSender) SendPairingPaired(ctx context.Context, msg PairingMessage) error {
	name := displayAgentName(msg.InstallName)
	return s.sendToUser(ctx, msg.UserID, "Paired", fmt.Sprintf("%s is now paired.", name), map[string]string{
		"event":      "paired",
		"install_id": msg.InstallID,
	})
}

func (s *expoSender) sendToUser(ctx context.Context, userID, title, body string, data map[string]string) error {
	if s.devices == nil {
		return errors.New("device store is required for expo push")
	}

	devices, err := s.devices.ListDevices(ctx, userID)
	if err != nil {
		return err
	}

	messages := make([]expoPushMessage, 0, len(devices))
	for _, device := range devices {
		token := strings.TrimSpace(device.PushToken)
		if token == "" || !isExpoPushToken(token) {
			continue
		}
		messages = append(messages, expoPushMessage{
			To:    token,
			Title: title,
			Body:  body,
			Data:  data,
		})
	}

	if len(messages) == 0 {
		return nil
	}

	badgeCount, err := s.devices.CountPendingQuestions(ctx, userID)
	if err != nil {
		return err
	}
	for i := range messages {
		messages[i].Badge = badgeCount
	}

	for start := 0; start < len(messages); start += expoMaxBatchSize {
		end := start + expoMaxBatchSize
		if end > len(messages) {
			end = len(messages)
		}
		if err := s.sendBatch(ctx, messages[start:end]); err != nil {
			return err
		}
	}

	return nil
}

func (s *expoSender) sendBatch(ctx context.Context, batch []expoPushMessage) error {
	if len(batch) == 0 {
		return nil
	}

	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("expo push request failed: %s", message)
	}

	if len(body) == 0 {
		return nil
	}

	var response expoPushResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	if len(response.Errors) > 0 {
		return fmt.Errorf("expo push error: %s", response.Errors[0].Message)
	}
	for _, ticket := range response.Data {
		if ticket.Status == "error" {
			if ticket.Message != "" {
				return fmt.Errorf("expo push error: %s", ticket.Message)
			}
			return errors.New("expo push error")
		}
	}

	return nil
}

func isExpoPushToken(token string) bool {
	return strings.HasPrefix(token, "ExponentPushToken[") || strings.HasPrefix(token, "ExpoPushToken[")
}

func displayAgentName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "Agent"
	}
	return trimmed
}

func questionTitle(count int, name string) string {
	agentName := displayAgentName(name)
	if count <= 0 {
		return fmt.Sprintf("New questions from %s", agentName)
	}
	if count == 1 {
		return fmt.Sprintf("1 new question from %s", agentName)
	}
	return fmt.Sprintf("%d new questions from %s", count, agentName)
}

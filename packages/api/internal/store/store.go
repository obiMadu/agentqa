package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrAlreadyResolved = errors.New("already resolved")
	ErrQuotaExceeded   = errors.New("quota exceeded")
)

type Store interface {
	UpsertUser(ctx context.Context, user User) (User, error)
	GetUser(ctx context.Context, userID string) (User, error)
	GetUserByOIDC(ctx context.Context, issuer, subject string) (User, error)
	AttachOIDCToUser(ctx context.Context, userID, issuer, subject string) error
	CreateAPIKey(ctx context.Context, userID, name, keyHash, keyPrefix, keyCiphertext, keyNonce string, scopes []string) (APIKey, error)
	ListAPIKeys(ctx context.Context, userID string) ([]APIKey, error)
	GetActiveAPIKey(ctx context.Context, userID string) (APIKey, error)
	RevokeAPIKey(ctx context.Context, userID, keyID string) error
	RevokeActiveAPIKeys(ctx context.Context, userID string) error
	LookupAPIKey(ctx context.Context, keyHash string) (APIKey, error)
	UpdateAPIKeyLastUsed(ctx context.Context, keyID string) error

	UpsertPluginInstall(ctx context.Context, install PluginInstall) error
	GetPluginInstall(ctx context.Context, installID string) (PluginInstall, error)
	ListPluginInstalls(ctx context.Context, userID string) ([]PluginInstall, error)
	UpdatePluginInstallActive(ctx context.Context, userID, installID string, active bool) error
	RequestPluginInstallPairing(ctx context.Context, userID, installID string) error
	PairPluginInstall(ctx context.Context, userID, installID string) error
	ActivatePluginInstallExclusive(ctx context.Context, userID, installID string) error
	EnforceSingleActivePluginInstall(ctx context.Context, userID string) (keptInstallID string, err error)
	UpsertDevice(ctx context.Context, device Device) error

	CreateQuestion(ctx context.Context, q Question, periodStart time.Time, monthlyLimit int) (created bool, err error)
	ListQuestions(ctx context.Context, userID string) ([]Question, error)
	GetQuestion(ctx context.Context, userID, questionID string) (Question, error)
	GetQuestionStatus(ctx context.Context, userID, questionID string) (string, error)
	GetAnswer(ctx context.Context, questionID string) (Answer, error)
	AnswerQuestion(ctx context.Context, questionID, userID string, answers [][]string) (Answer, error)
	RejectQuestion(ctx context.Context, questionID string, userID *string) error
	GetUIPreferences(ctx context.Context, userID string) (UIPreferences, error)
	UpsertUIPreferences(ctx context.Context, userID string, data UIPreferencesData) (UIPreferences, error)

	StoreSuperwallWebhookEvent(ctx context.Context, eventID string, payload []byte) (inserted bool, err error)
	GetBillingEntitlements(ctx context.Context, userID string) (BillingEntitlements, error)
	UpsertBillingEntitlements(ctx context.Context, userID string, proActive bool, proExpiresAt *time.Time, markTrialUsed bool) (BillingEntitlements, error)
	GetBillingUsageMonthly(ctx context.Context, userID string, periodStart time.Time) (BillingUsageMonthly, error)
	CountActivePluginInstalls(ctx context.Context, userID string) (int, error)
}

type User struct {
	ID           string
	Email        string
	Name         string
	AuthProvider string
	AuthIssuer   *string
	AuthSubject  *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type APIKey struct {
	ID            string
	UserID        string
	KeyHash       string
	KeyCiphertext *string
	KeyNonce      *string
	KeyPrefix     string
	Name          string
	Scopes        []string
	CreatedAt     time.Time
	RevokedAt     *time.Time
	LastUsedAt    *time.Time
}

type PluginInstall struct {
	InstallID          string
	UserID             string
	Name               string
	KeyID              *string
	Active             bool
	Paired             bool
	PairingRequestedAt *time.Time
	PairedAt           *time.Time
	CreatedAt          time.Time
	LastSeenAt         *time.Time
}

type Device struct {
	ID         string
	UserID     string
	Platform   string
	PushToken  string
	CreatedAt  time.Time
	LastSeenAt *time.Time
}

type Question struct {
	ID        string
	UserID    string
	InstallID string
	SessionID string
	Payload   []byte
	Status    string
	CreatedAt time.Time
}

type Answer struct {
	ID         string
	QuestionID string
	UserID     string
	Body       string
	CreatedAt  time.Time
}

type BillingEntitlements struct {
	UserID       string
	ProActive    bool
	ProExpiresAt *time.Time
	TrialUsedAt  *time.Time
	UpdatedAt    time.Time
}

type BillingUsageMonthly struct {
	UserID           string
	PeriodStart      time.Time
	QuestionRequests int
}

const (
	BackgroundModeShuffle = "shuffle"
	BackgroundModeFixed   = "fixed"
)

const (
	ThemeAccentGreen  = "green"
	ThemeAccentBlue   = "blue"
	ThemeAccentOrange = "orange"
	ThemeAccentRed    = "red"
	ThemeAccentTeal   = "teal"
)

var ValidThemeAccents = map[string]struct{}{
	ThemeAccentGreen:  {},
	ThemeAccentBlue:   {},
	ThemeAccentOrange: {},
	ThemeAccentRed:    {},
	ThemeAccentTeal:   {},
}

const (
	QuestionDisplayStacked   = "stacked"
	QuestionDisplayPager     = "pager"
	QuestionDisplayAccordion = "accordion"
)

var ValidQuestionDisplayModes = map[string]struct{}{
	QuestionDisplayStacked:   {},
	QuestionDisplayPager:     {},
	QuestionDisplayAccordion: {},
}

type UIPreferences struct {
	ID        string
	UserID    string
	Data      UIPreferencesData
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UIPreferencesData struct {
	Background BackgroundPreferences `json:"background"`
	Theme      *ThemePreferences     `json:"theme,omitempty"`
	Questions  *QuestionPreferences  `json:"questions,omitempty"`
}

type BackgroundPreferences struct {
	Mode      string `json:"mode"`
	PatternID string `json:"pattern_id,omitempty"`
}

type ThemePreferences struct {
	Accent string `json:"accent,omitempty"`
}

type QuestionPreferences struct {
	DisplayMode string `json:"display_mode,omitempty"`
}

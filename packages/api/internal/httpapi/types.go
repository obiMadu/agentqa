package httpapi

import "github.com/agentqa/agentqa/packages/api/internal/store"

type GoogleAuthRequest struct {
	IDToken string `json:"id_token"`
}

type AuthResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	User         UserPayload `json:"user"`
}

type UserPayload struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type APIKeyInfoResponse struct {
	KeyPrefix  string  `json:"key_prefix"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

type APIKeyRawResponse struct {
	APIKey string `json:"api_key"`
}

type ResetAPIKeyResponse struct {
	APIKey    string `json:"api_key"`
	KeyPrefix string `json:"key_prefix"`
	CreatedAt string `json:"created_at"`
}

type PluginRegisterRequest struct {
	InstallID string `json:"install_id"`
	Name      string `json:"name"`
}

type PluginInstallPayload struct {
	InstallID  string  `json:"install_id"`
	Name       string  `json:"name"`
	Active     bool    `json:"active"`
	CreatedAt  string  `json:"created_at"`
	LastSeenAt *string `json:"last_seen_at"`
}

type ListPluginInstallsResponse struct {
	Installs []PluginInstallPayload `json:"installs"`
}

type UpdatePluginInstallRequest struct {
	Active *bool `json:"active"`
}

type CreateQuestionRequest struct {
	RequestID string           `json:"request_id"`
	SessionID string           `json:"session_id"`
	InstallID string           `json:"install_id"`
	Questions []QuestionPrompt `json:"questions"`
	Tool      *QuestionTool    `json:"tool,omitempty"`
}

type QuestionTool struct {
	MessageID string `json:"messageID"`
	CallID    string `json:"callID"`
}

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type QuestionPrompt struct {
	Question string           `json:"question"`
	Header   string           `json:"header,omitempty"`
	Options  []QuestionOption `json:"options,omitempty"`
	Multiple *bool            `json:"multiple,omitempty"`
	Custom   *bool            `json:"custom,omitempty"`
}

type AnswerRequest struct {
	Answers [][]string `json:"answers"`
	Source  string     `json:"source"`
}

type QuestionResponse struct {
	ID        string           `json:"id"`
	Status    string           `json:"status"`
	SessionID string           `json:"session_id"`
	InstallID string           `json:"install_id"`
	Questions []QuestionPrompt `json:"questions"`
	Answers   [][]string       `json:"answers,omitempty"`
	CreatedAt string           `json:"created_at"`
}

type QuestionListItem struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	SessionID     string `json:"session_id"`
	InstallID     string `json:"install_id"`
	Preview       string `json:"preview"`
	QuestionCount int    `json:"question_count"`
	Source        string `json:"source"`
	CreatedAt     string `json:"created_at"`
}

type ListQuestionsResponse struct {
	Questions []QuestionListItem `json:"questions"`
}

type WaitResponse struct {
	Status     string     `json:"status"`
	Answers    [][]string `json:"answers,omitempty"`
	QuestionID string     `json:"question_id"`
}

type RegisterDeviceRequest struct {
	Platform  string `json:"platform"`
	PushToken string `json:"push_token"`
}

type UIPreferencesRequest struct {
	Preferences store.UIPreferencesData `json:"preferences"`
}

type UIPreferencesResponse struct {
	Preferences store.UIPreferencesData `json:"preferences"`
	UpdatedAt   string                  `json:"updated_at"`
}

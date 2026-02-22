package postgres

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type StringList []string

func (list StringList) Value() (driver.Value, error) {
	if list == nil {
		return []byte("[]"), nil
	}

	encoded, err := json.Marshal([]string(list))
	if err != nil {
		return nil, err
	}

	return encoded, nil
}

func (list *StringList) Scan(value interface{}) error {
	if value == nil {
		*list = nil
		return nil
	}

	switch typed := value.(type) {
	case []byte:
		return json.Unmarshal(typed, list)
	case string:
		return json.Unmarshal([]byte(typed), list)
	default:
		return fmt.Errorf("unsupported scopes type: %T", value)
	}
}

type userModel struct {
	ID           string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email        string    `gorm:"type:text;not null;uniqueIndex"`
	Name         string    `gorm:"type:text"`
	AuthProvider string    `gorm:"type:text;not null"`
	AuthIssuer   *string   `gorm:"type:text;uniqueIndex:users_auth_issuer_subject"`
	AuthSubject  *string   `gorm:"type:text;uniqueIndex:users_auth_issuer_subject"`
	CreatedAt    time.Time `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt    time.Time `gorm:"type:timestamptz;not null;default:now()"`
}

func (userModel) TableName() string {
	return "users"
}

type apiKeyModel struct {
	ID            string     `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID        string     `gorm:"type:uuid;not null;index"`
	User          userModel  `gorm:"constraint:OnDelete:CASCADE;"`
	KeyHash       string     `gorm:"type:text;not null;uniqueIndex"`
	KeyCiphertext *string    `gorm:"type:text"`
	KeyNonce      *string    `gorm:"type:text"`
	KeyPrefix     string     `gorm:"type:text;not null"`
	Name          string     `gorm:"type:text"`
	Scopes        StringList `gorm:"type:jsonb;not null;default:'[]'"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	RevokedAt     *time.Time `gorm:"type:timestamptz"`
	LastUsedAt    *time.Time `gorm:"type:timestamptz"`
}

func (apiKeyModel) TableName() string {
	return "api_keys"
}

type pluginInstallModel struct {
	InstallID          string       `gorm:"type:text;primaryKey"`
	UserID             string       `gorm:"type:uuid;not null;index"`
	User               userModel    `gorm:"constraint:OnDelete:CASCADE;"`
	Name               string       `gorm:"type:text"`
	KeyID              *string      `gorm:"type:uuid;index"`
	APIKey             *apiKeyModel `gorm:"foreignKey:KeyID;constraint:OnDelete:SET NULL;"`
	Active             bool         `gorm:"type:boolean;not null;default:true"`
	Paired             bool         `gorm:"type:boolean;not null;default:false"`
	PairingRequestedAt *time.Time   `gorm:"type:timestamptz"`
	PairedAt           *time.Time   `gorm:"type:timestamptz"`
	CreatedAt          time.Time    `gorm:"type:timestamptz;not null;default:now()"`
	LastSeenAt         *time.Time   `gorm:"type:timestamptz"`
}

func (pluginInstallModel) TableName() string {
	return "plugin_installs"
}

type deviceModel struct {
	ID         string     `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID     string     `gorm:"type:uuid;not null;index"`
	User       userModel  `gorm:"constraint:OnDelete:CASCADE;"`
	Platform   string     `gorm:"type:text;not null"`
	PushToken  string     `gorm:"type:text;not null;uniqueIndex"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	LastSeenAt *time.Time `gorm:"type:timestamptz"`
}

func (deviceModel) TableName() string {
	return "devices"
}

type questionModel struct {
	ID         string     `gorm:"type:text;primaryKey"`
	UserID     string     `gorm:"type:uuid;not null;index"`
	User       userModel  `gorm:"constraint:OnDelete:CASCADE;"`
	InstallID  *string    `gorm:"type:text"`
	SessionID  string     `gorm:"type:text"`
	Payload    []byte     `gorm:"type:jsonb;not null"`
	Status     string     `gorm:"type:text;not null;default:'pending';check:status IN ('pending','answered','rejected')"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	AnsweredAt *time.Time `gorm:"type:timestamptz"`
	RejectedAt *time.Time `gorm:"type:timestamptz"`
}

func (questionModel) TableName() string {
	return "questions"
}

type answerModel struct {
	ID         string        `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	QuestionID string        `gorm:"type:text;not null;uniqueIndex"`
	Question   questionModel `gorm:"constraint:OnDelete:CASCADE;"`
	UserID     string        `gorm:"type:uuid;not null;index"`
	User       userModel     `gorm:"constraint:OnDelete:CASCADE;"`
	Body       string        `gorm:"type:text;not null"`
	CreatedAt  time.Time     `gorm:"type:timestamptz;not null;default:now()"`
}

func (answerModel) TableName() string {
	return "answers"
}

type uiPreferenceModel struct {
	ID        string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex"`
	User      userModel `gorm:"constraint:OnDelete:CASCADE;"`
	Data      []byte    `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt time.Time `gorm:"type:timestamptz;not null;default:now()"`
}

func (uiPreferenceModel) TableName() string {
	return "ui_preferences"
}

func AutoMigrate(db *gorm.DB) error {
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		return err
	}

	if err := migrateScopesToJSONB(db); err != nil {
		return err
	}

	if err := db.AutoMigrate(
		&userModel{},
		&pluginInstallModel{},
		&apiKeyModel{},
		&deviceModel{},
		&questionModel{},
		&answerModel{},
		&uiPreferenceModel{},
	); err != nil {
		return err
	}

	return migrateSingleActiveAPIKey(db)
}

func migrateScopesToJSONB(db *gorm.DB) error {
	query := `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_name = 'api_keys'
					AND column_name = 'scopes'
					AND data_type = 'ARRAY'
			) THEN
				ALTER TABLE api_keys
					ALTER COLUMN scopes TYPE jsonb USING to_jsonb(scopes),
					ALTER COLUMN scopes SET DEFAULT '[]'::jsonb;
			END IF;
		END $$;
	`

	return db.Exec(query).Error
}

func migrateSingleActiveAPIKey(db *gorm.DB) error {
	if !db.Migrator().HasTable(&apiKeyModel{}) {
		return nil
	}

	indexQuery := `
		CREATE UNIQUE INDEX IF NOT EXISTS api_keys_user_active_unique
		ON api_keys(user_id)
		WHERE revoked_at IS NULL
	`
	return db.Exec(indexQuery).Error
}

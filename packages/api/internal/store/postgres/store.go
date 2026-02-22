package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agentqa/agentqa/packages/api/internal/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) UpsertUser(ctx context.Context, user store.User) (store.User, error) {
	record := userModel{
		Email:        user.Email,
		Name:         user.Name,
		AuthProvider: user.AuthProvider,
		AuthIssuer:   user.AuthIssuer,
		AuthSubject:  user.AuthSubject,
	}

	err := s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns: []clause.Column{{Name: "email"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"name":          user.Name,
				"auth_provider": user.AuthProvider,
				"updated_at":    gorm.Expr("now()"),
			}),
		},
		clause.Returning{},
	).Create(&record).Error
	if err != nil {
		return store.User{}, err
	}

	return store.User{
		ID:           record.ID,
		Email:        record.Email,
		Name:         record.Name,
		AuthProvider: record.AuthProvider,
		AuthIssuer:   record.AuthIssuer,
		AuthSubject:  record.AuthSubject,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}, nil
}

func (s *Store) GetUser(ctx context.Context, userID string) (store.User, error) {
	var record userModel
	err := s.db.WithContext(ctx).
		Where("id = ?", userID).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.User{}, store.ErrNotFound
		}
		return store.User{}, err
	}

	return store.User{
		ID:           record.ID,
		Email:        record.Email,
		Name:         record.Name,
		AuthProvider: record.AuthProvider,
		AuthIssuer:   record.AuthIssuer,
		AuthSubject:  record.AuthSubject,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}, nil
}

func (s *Store) GetUserByOIDC(ctx context.Context, issuer, subject string) (store.User, error) {
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	if issuer == "" || subject == "" {
		return store.User{}, fmt.Errorf("missing issuer or subject")
	}

	var record userModel
	err := s.db.WithContext(ctx).
		Where("auth_issuer = ? AND auth_subject = ?", issuer, subject).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.User{}, store.ErrNotFound
		}
		return store.User{}, err
	}

	return store.User{
		ID:           record.ID,
		Email:        record.Email,
		Name:         record.Name,
		AuthProvider: record.AuthProvider,
		AuthIssuer:   record.AuthIssuer,
		AuthSubject:  record.AuthSubject,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}, nil
}

func (s *Store) AttachOIDCToUser(ctx context.Context, userID, issuer, subject string) error {
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	if issuer == "" || subject == "" {
		return fmt.Errorf("missing issuer or subject")
	}

	result := s.db.WithContext(ctx).
		Model(&userModel{}).
		Where("id = ? AND auth_issuer IS NULL AND auth_subject IS NULL", userID).
		Updates(map[string]interface{}{
			"auth_issuer":  issuer,
			"auth_subject": subject,
			"updated_at":   gorm.Expr("now()"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var record userModel
	err := s.db.WithContext(ctx).
		Select("id", "auth_issuer", "auth_subject").
		Where("id = ?", userID).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.ErrNotFound
		}
		return err
	}

	if record.AuthIssuer != nil && record.AuthSubject != nil {
		if *record.AuthIssuer == issuer && *record.AuthSubject == subject {
			return nil
		}
	}

	return fmt.Errorf("user already linked to different OIDC identity")
}

func (s *Store) CreateAPIKey(ctx context.Context, userID, name, keyHash, keyPrefix, keyCiphertext, keyNonce string, scopes []string) (store.APIKey, error) {
	scopesList := StringList(scopes)
	if scopesList == nil {
		scopesList = StringList{}
	}

	var keyCiphertextPtr *string
	if keyCiphertext != "" {
		keyCiphertextPtr = &keyCiphertext
	}

	var keyNoncePtr *string
	if keyNonce != "" {
		keyNoncePtr = &keyNonce
	}

	record := apiKeyModel{
		UserID:        userID,
		KeyHash:       keyHash,
		KeyCiphertext: keyCiphertextPtr,
		KeyNonce:      keyNoncePtr,
		KeyPrefix:     keyPrefix,
		Name:          name,
		Scopes:        scopesList,
	}

	err := s.db.WithContext(ctx).Clauses(clause.Returning{}).Create(&record).Error
	if err != nil {
		return store.APIKey{}, err
	}

	return store.APIKey{
		ID:            record.ID,
		UserID:        record.UserID,
		KeyHash:       record.KeyHash,
		KeyCiphertext: record.KeyCiphertext,
		KeyNonce:      record.KeyNonce,
		KeyPrefix:     record.KeyPrefix,
		Name:          record.Name,
		Scopes:        []string(record.Scopes),
		CreatedAt:     record.CreatedAt,
		RevokedAt:     record.RevokedAt,
		LastUsedAt:    record.LastUsedAt,
	}, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, userID string) ([]store.APIKey, error) {
	var records []apiKeyModel
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("created_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	keys := make([]store.APIKey, 0, len(records))
	for _, record := range records {
		keys = append(keys, store.APIKey{
			ID:            record.ID,
			UserID:        record.UserID,
			KeyHash:       record.KeyHash,
			KeyCiphertext: record.KeyCiphertext,
			KeyNonce:      record.KeyNonce,
			KeyPrefix:     record.KeyPrefix,
			Name:          record.Name,
			Scopes:        []string(record.Scopes),
			CreatedAt:     record.CreatedAt,
			RevokedAt:     record.RevokedAt,
			LastUsedAt:    record.LastUsedAt,
		})
	}

	return keys, nil
}

func (s *Store) GetActiveAPIKey(ctx context.Context, userID string) (store.APIKey, error) {
	var record apiKeyModel
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("created_at DESC").
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.APIKey{}, store.ErrNotFound
		}
		return store.APIKey{}, err
	}

	return store.APIKey{
		ID:            record.ID,
		UserID:        record.UserID,
		KeyHash:       record.KeyHash,
		KeyCiphertext: record.KeyCiphertext,
		KeyNonce:      record.KeyNonce,
		KeyPrefix:     record.KeyPrefix,
		Name:          record.Name,
		Scopes:        []string(record.Scopes),
		CreatedAt:     record.CreatedAt,
		RevokedAt:     record.RevokedAt,
		LastUsedAt:    record.LastUsedAt,
	}, nil
}

func (s *Store) RevokeAPIKey(ctx context.Context, userID, keyID string) error {
	result := s.db.WithContext(ctx).
		Model(&apiKeyModel{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", keyID, userID).
		Update("revoked_at", gorm.Expr("now()"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) RevokeActiveAPIKeys(ctx context.Context, userID string) error {
	result := s.db.WithContext(ctx).
		Model(&apiKeyModel{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", gorm.Expr("now()"))
	return result.Error
}

func (s *Store) LookupAPIKey(ctx context.Context, keyHash string) (store.APIKey, error) {
	var record apiKeyModel
	err := s.db.WithContext(ctx).
		Where("key_hash = ? AND revoked_at IS NULL", keyHash).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.APIKey{}, store.ErrNotFound
		}
		return store.APIKey{}, err
	}

	return store.APIKey{
		ID:            record.ID,
		UserID:        record.UserID,
		KeyHash:       record.KeyHash,
		KeyCiphertext: record.KeyCiphertext,
		KeyNonce:      record.KeyNonce,
		KeyPrefix:     record.KeyPrefix,
		Name:          record.Name,
		Scopes:        []string(record.Scopes),
		CreatedAt:     record.CreatedAt,
		RevokedAt:     record.RevokedAt,
		LastUsedAt:    record.LastUsedAt,
	}, nil
}

func (s *Store) UpdateAPIKeyLastUsed(ctx context.Context, keyID string) error {
	result := s.db.WithContext(ctx).
		Model(&apiKeyModel{}).
		Where("id = ?", keyID).
		Update("last_used_at", gorm.Expr("now()"))
	return result.Error
}

func (s *Store) UpsertPluginInstall(ctx context.Context, install store.PluginInstall) error {
	record := pluginInstallModel{
		InstallID: install.InstallID,
		UserID:    install.UserID,
		Name:      install.Name,
		KeyID:     install.KeyID,
		Active:    true,
	}

	return s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns: []clause.Column{{Name: "install_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"user_id":      install.UserID,
				"name":         install.Name,
				"key_id":       install.KeyID,
				"last_seen_at": gorm.Expr("now()"),
			}),
		},
	).Create(&record).Error
}

func (s *Store) GetPluginInstall(ctx context.Context, installID string) (store.PluginInstall, error) {
	var record pluginInstallModel
	err := s.db.WithContext(ctx).
		Where("install_id = ?", installID).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.PluginInstall{}, store.ErrNotFound
		}
		return store.PluginInstall{}, err
	}

	return store.PluginInstall{
		InstallID:          record.InstallID,
		UserID:             record.UserID,
		Name:               record.Name,
		KeyID:              record.KeyID,
		Active:             record.Active,
		Paired:             record.Paired,
		PairingRequestedAt: record.PairingRequestedAt,
		PairedAt:           record.PairedAt,
		CreatedAt:          record.CreatedAt,
		LastSeenAt:         record.LastSeenAt,
	}, nil
}

func (s *Store) ListPluginInstalls(ctx context.Context, userID string) ([]store.PluginInstall, error) {
	var records []pluginInstallModel
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("last_seen_at DESC NULLS LAST, created_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	installs := make([]store.PluginInstall, 0, len(records))
	for _, record := range records {
		installs = append(installs, store.PluginInstall{
			InstallID:          record.InstallID,
			UserID:             record.UserID,
			Name:               record.Name,
			KeyID:              record.KeyID,
			Active:             record.Active,
			Paired:             record.Paired,
			PairingRequestedAt: record.PairingRequestedAt,
			PairedAt:           record.PairedAt,
			CreatedAt:          record.CreatedAt,
			LastSeenAt:         record.LastSeenAt,
		})
	}

	return installs, nil
}

func (s *Store) UpdatePluginInstallActive(ctx context.Context, userID, installID string, active bool) error {
	result := s.db.WithContext(ctx).
		Model(&pluginInstallModel{}).
		Where("install_id = ? AND user_id = ?", installID, userID).
		Update("active", active)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) RequestPluginInstallPairing(ctx context.Context, userID, installID string) error {
	result := s.db.WithContext(ctx).
		Model(&pluginInstallModel{}).
		Where("install_id = ? AND user_id = ? AND paired = false AND pairing_requested_at IS NULL", installID, userID).
		Update("pairing_requested_at", gorm.Expr("now()"))
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Store) PairPluginInstall(ctx context.Context, userID, installID string) error {
	result := s.db.WithContext(ctx).
		Model(&pluginInstallModel{}).
		Where("install_id = ? AND user_id = ?", installID, userID).
		Updates(map[string]interface{}{
			"paired":               true,
			"paired_at":            gorm.Expr("COALESCE(paired_at, now())"),
			"pairing_requested_at": gorm.Expr("NULL"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) UpsertDevice(ctx context.Context, device store.Device) error {
	record := deviceModel{
		UserID:    device.UserID,
		Platform:  device.Platform,
		PushToken: device.PushToken,
	}

	return s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns: []clause.Column{{Name: "push_token"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"user_id":      device.UserID,
				"platform":     device.Platform,
				"last_seen_at": gorm.Expr("now()"),
			}),
		},
	).Create(&record).Error
}

func (s *Store) CreateQuestion(ctx context.Context, question store.Question) error {
	status := question.Status
	if status == "" {
		status = "pending"
	}

	var installID *string
	if question.InstallID != "" {
		installID = &question.InstallID
	}

	record := questionModel{
		ID:        question.ID,
		UserID:    question.UserID,
		InstallID: installID,
		SessionID: question.SessionID,
		Payload:   question.Payload,
		Status:    status,
	}

	return s.db.WithContext(ctx).Clauses(
		clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true},
	).Create(&record).Error
}

func (s *Store) ListQuestions(ctx context.Context, userID string) ([]store.Question, error) {
	var records []questionModel
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	questions := make([]store.Question, 0, len(records))
	for _, record := range records {
		installID := ""
		if record.InstallID != nil {
			installID = *record.InstallID
		}
		questions = append(questions, store.Question{
			ID:        record.ID,
			UserID:    record.UserID,
			InstallID: installID,
			SessionID: record.SessionID,
			Payload:   record.Payload,
			Status:    record.Status,
			CreatedAt: record.CreatedAt,
		})
	}

	return questions, nil
}

func (s *Store) GetQuestion(ctx context.Context, userID, questionID string) (store.Question, error) {
	var record questionModel
	err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", questionID, userID).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Question{}, store.ErrNotFound
		}
		return store.Question{}, err
	}

	installID := ""
	if record.InstallID != nil {
		installID = *record.InstallID
	}

	return store.Question{
		ID:        record.ID,
		UserID:    record.UserID,
		InstallID: installID,
		SessionID: record.SessionID,
		Payload:   record.Payload,
		Status:    record.Status,
		CreatedAt: record.CreatedAt,
	}, nil
}

func (s *Store) GetQuestionStatus(ctx context.Context, userID, questionID string) (string, error) {
	var record questionModel
	err := s.db.WithContext(ctx).
		Select("status").
		Where("id = ? AND user_id = ?", questionID, userID).
		Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", store.ErrNotFound
		}
		return "", err
	}

	return record.Status, nil
}

func (s *Store) GetAnswer(ctx context.Context, questionID string) (store.Answer, error) {
	var record answerModel
	err := s.db.WithContext(ctx).
		Where("question_id = ?", questionID).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Answer{}, store.ErrNotFound
		}
		return store.Answer{}, err
	}

	return store.Answer{
		ID:         record.ID,
		QuestionID: record.QuestionID,
		UserID:     record.UserID,
		Body:       record.Body,
		CreatedAt:  record.CreatedAt,
	}, nil
}

func (s *Store) AnswerQuestion(ctx context.Context, questionID, userID string, answers [][]string) (store.Answer, error) {
	var result store.Answer
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var question questionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", questionID).
			Take(&question).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return store.ErrNotFound
			}
			return err
		}

		if question.Status != "pending" {
			return store.ErrAlreadyResolved
		}

		answersJSON, err := json.Marshal(answers)
		if err != nil {
			return err
		}

		answer := answerModel{
			QuestionID: questionID,
			UserID:     userID,
			Body:       string(answersJSON),
		}
		if err := tx.Clauses(clause.Returning{}).Create(&answer).Error; err != nil {
			return err
		}

		if err := tx.Model(&questionModel{}).
			Where("id = ?", questionID).
			Updates(map[string]interface{}{
				"status":      "answered",
				"answered_at": gorm.Expr("now()"),
			}).Error; err != nil {
			return err
		}

		result = store.Answer{
			ID:         answer.ID,
			QuestionID: answer.QuestionID,
			UserID:     answer.UserID,
			Body:       answer.Body,
			CreatedAt:  answer.CreatedAt,
		}

		return nil
	})

	return result, err
}

func (s *Store) RejectQuestion(ctx context.Context, questionID string, userID *string) error {
	_ = userID
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var question questionModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", questionID).
			Take(&question).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return store.ErrNotFound
			}
			return err
		}

		if question.Status != "pending" {
			return store.ErrAlreadyResolved
		}

		return tx.Model(&questionModel{}).
			Where("id = ?", questionID).
			Updates(map[string]interface{}{
				"status":      "rejected",
				"rejected_at": gorm.Expr("now()"),
			}).Error
	})
}

func (s *Store) GetUIPreferences(ctx context.Context, userID string) (store.UIPreferences, error) {
	var record uiPreferenceModel
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.UIPreferences{}, store.ErrNotFound
		}
		return store.UIPreferences{}, err
	}

	data := store.UIPreferencesData{}
	if len(record.Data) > 0 {
		if err := json.Unmarshal(record.Data, &data); err != nil {
			return store.UIPreferences{}, err
		}
	}

	return store.UIPreferences{
		ID:        record.ID,
		UserID:    record.UserID,
		Data:      data,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}, nil
}

func (s *Store) UpsertUIPreferences(ctx context.Context, userID string, data store.UIPreferencesData) (store.UIPreferences, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return store.UIPreferences{}, err
	}

	record := uiPreferenceModel{
		UserID: userID,
		Data:   encoded,
	}

	if err := s.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"data":       record.Data,
				"updated_at": gorm.Expr("now()"),
			}),
		},
		clause.Returning{},
	).Create(&record).Error; err != nil {
		return store.UIPreferences{}, err
	}

	return store.UIPreferences{
		ID:        record.ID,
		UserID:    record.UserID,
		Data:      data,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}, nil
}

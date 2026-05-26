package postgres

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
	"sleepwalker.fm/internal/domain"
)

type TokenRepo struct {
	DB *DB
}

func NewTokenRepo(db *DB) *TokenRepo {
	return &TokenRepo{DB: db}
}

func (r *TokenRepo) UpsertTokens(ctx context.Context, tokens domain.SpotifyTokens) error {
	model := SpotifyToken{
		UserID:       tokens.UserID,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		Scope:        tokens.Scope,
		TokenType:    tokens.TokenType,
		ExpiresAt:    tokens.ExpiresAt,
		UpdatedAt:    time.Now(),
	}

	return r.DB.Gorm.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"access_token", "refresh_token", "scope", "token_type", "expires_at", "updated_at"}),
	}).Create(&model).Error
}

func (r *TokenRepo) GetByUserID(ctx context.Context, userID string) (domain.SpotifyTokens, error) {
	var model SpotifyToken
	err := r.DB.Gorm.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&model).Error
	if err != nil {
		return domain.SpotifyTokens{}, err
	}
	return domain.SpotifyTokens{
		UserID:       model.UserID,
		AccessToken:  model.AccessToken,
		RefreshToken: model.RefreshToken,
		Scope:        model.Scope,
		TokenType:    model.TokenType,
		ExpiresAt:    model.ExpiresAt,
		UpdatedAt:    model.UpdatedAt,
	}, nil
}

func (r *TokenRepo) TouchUpdatedAt(ctx context.Context, userID string, updatedAt time.Time) error {
	return r.DB.Gorm.WithContext(ctx).
		Model(&SpotifyToken{}).
		Where("user_id = ?", userID).
		Update("updated_at", updatedAt).Error
}

package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type OAuthStateRepo struct {
	DB *DB
}

func NewOAuthStateRepo(db *DB) *OAuthStateRepo {
	return &OAuthStateRepo{DB: db}
}

func (r *OAuthStateRepo) Create(ctx context.Context, state, verifier string, expiresAt time.Time) error {
	model := OAuthState{
		State:        state,
		CodeVerifier: verifier,
		ExpiresAt:    expiresAt,
	}
	return r.DB.Gorm.WithContext(ctx).Create(&model).Error
}

func (r *OAuthStateRepo) GetAndDelete(ctx context.Context, state string) (string, error) {
	var model OAuthState
	query := r.DB.Gorm.WithContext(ctx).
		Where("state = ? AND expires_at > NOW()", state).
		First(&model)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return "", gorm.ErrRecordNotFound
		}
		return "", query.Error
	}
	if err := r.DB.Gorm.WithContext(ctx).Delete(&model).Error; err != nil {
		return "", err
	}
	return model.CodeVerifier, nil
}

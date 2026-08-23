package persistence

import (
	"context"
	"fmt"
	"time"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// LoadRateLimitState はレート制限の待機状態を読み出す。
//
// 収集は1分ごとに別プロセスとして起動するため、待機の状態をプロセス内に
// 持てない。api_accounts の列に置いて、次の起動が読み直す。
func (s *Store) LoadRateLimitState(ctx context.Context, accountID domain.AccountID) (usecase.RateLimitState, error) {
	var account apiAccountModel
	err := s.db.WithContext(ctx).
		Select("rate_limited_until", "rate_limited_since", "rate_limit_failures").
		Where("id = ?", int64(accountID)).
		Take(&account).Error
	if err != nil {
		return usecase.RateLimitState{}, fmt.Errorf("レート制限の待機状態の取得に失敗しました: %w", err)
	}

	return usecase.RateLimitState{
		Until:    timeOrZero(account.RateLimitedUntil),
		Since:    timeOrZero(account.RateLimitedSince),
		Failures: account.RateLimitFailures,
	}, nil
}

// SaveRateLimitState はレート制限の待機状態を保存する。
// ゼロ値を渡すと待機を解除する（両方の日時が NULL、回数が 0 になる）。
//
// UpdateColumns を使うのは、GORM による updated_at の自動更新を避けるため。
// 列定義の ON UPDATE CURRENT_TIMESTAMP に任せる方針は SaveDevice と同じ。
func (s *Store) SaveRateLimitState(ctx context.Context, accountID domain.AccountID, state usecase.RateLimitState) error {
	err := s.db.WithContext(ctx).
		Model(&apiAccountModel{}).
		Where("id = ?", int64(accountID)).
		UpdateColumns(map[string]any{
			"rate_limited_until":  nilIfZero(state.Until),
			"rate_limited_since":  nilIfZero(state.Since),
			"rate_limit_failures": state.Failures,
		}).Error
	if err != nil {
		return fmt.Errorf("レート制限の待機状態の更新に失敗しました: %w", err)
	}
	return nil
}

// timeOrZero は NULL 由来の nil をゼロ値の time.Time に直す。
func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// nilIfZero はゼロ値の time.Time を NULL として書けるよう nil に直す。
func nilIfZero(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/usecase"
)

// このファイルは、レート制限の待機状態を読み書きする SQL を検証する。
// 方針は store_test.go と同じで、MySQL には接続しない。

func TestLoadRateLimitState_発行するSQL(t *testing.T) {
	store, capture := newDryRunStore(t)

	_, err := store.LoadRateLimitState(context.Background(), 7)
	require.NoError(t, err)

	stmt := capture.only(t)
	assert.Equal(t,
		"SELECT `rate_limited_until`,`rate_limited_since`,`rate_limit_failures` "+
			"FROM `api_accounts` WHERE id = ? LIMIT ?",
		stmt.SQL)
	require.Len(t, stmt.Vars, 2)
	assert.Equal(t, int64(7), stmt.Vars[0])
}

func TestSaveRateLimitState_発行するSQL(t *testing.T) {
	store, capture := newDryRunStore(t)

	until := time.Date(2026, 8, 23, 9, 15, 0, 0, time.UTC)
	since := time.Date(2026, 8, 23, 4, 27, 0, 0, time.UTC)
	err := store.SaveRateLimitState(context.Background(), 7, usecase.RateLimitState{
		Until:    until,
		Since:    since,
		Failures: 5,
	})
	require.NoError(t, err)

	stmt := capture.only(t)
	assert.Equal(t,
		"UPDATE `api_accounts` SET "+
			"`rate_limit_failures`=?,`rate_limited_since`=?,`rate_limited_until`=? "+
			"WHERE id = ?",
		stmt.SQL)
	assert.NotContains(t, stmt.SQL, "updated_at",
		"updated_at は列定義の ON UPDATE CURRENT_TIMESTAMP に任せる")

	require.Len(t, stmt.Vars, 4)
	assert.Equal(t, 5, stmt.Vars[0])
	assert.Equal(t, since, stmt.Vars[1])
	assert.Equal(t, until, stmt.Vars[2])
	assert.Equal(t, int64(7), stmt.Vars[3])
}

func TestSaveRateLimitState_ゼロ値ならNULLで解除する(t *testing.T) {
	// 待機の解除は「列を NULL に戻す」こと。ゼロ値の time.Time をそのまま
	// 書くと 0000-00-00 になり、MySQL の設定によっては挿入自体が失敗する。
	store, capture := newDryRunStore(t)

	require.NoError(t, store.SaveRateLimitState(context.Background(), 7, usecase.RateLimitState{}))

	stmt := capture.only(t)
	require.Len(t, stmt.Vars, 4)
	assert.Equal(t, 0, stmt.Vars[0])
	assert.Nil(t, stmt.Vars[1])
	assert.Nil(t, stmt.Vars[2])
}

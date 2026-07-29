package presenter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/adapter/presenter"
	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// newTestLogger は buf へ JSON 形式で書き出す logger を返す。
// adapter 層のテストから infra 層を import しないよう slog を直接使う。
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// logEntries は JSON Lines 形式のログをパースして返す。
func logEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	entries := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &e), "行: %s", line)
		entries = append(entries, e)
	}
	return entries
}

func TestPresent_保存したデバイスをINFOで記録する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "メインアカウント",
		Devices: []usecase.DeviceResult{{
			Device:  domain.Device{ID: "d1", Name: "温湿度計", Type: "Meter"},
			Outcome: usecase.OutcomeStored,
		}},
	}}})

	entries := logEntries(t, &buf)
	require.NotEmpty(t, entries)

	found := false
	for _, e := range entries {
		if e["device"] == "温湿度計" {
			found = true
			assert.Equal(t, "INFO", e["level"])
			assert.Equal(t, "メインアカウント", e["account"])
			assert.Equal(t, "stored", e["outcome"])
			assert.Equal(t, "Meter", e["type"])
		}
	}
	assert.True(t, found, "デバイスのログが出力されていない")
}

func TestPresent_デバイスの失敗をWARNで記録する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "acc1",
		Devices: []usecase.DeviceResult{{
			Device:  domain.Device{ID: "d1", Name: "玄関Bot"},
			Outcome: usecase.OutcomeFailed,
			Err:     errors.New("デバイスオフライン"),
		}},
	}}})

	entries := logEntries(t, &buf)

	found := false
	for _, e := range entries {
		if e["device"] == "玄関Bot" {
			found = true
			assert.Equal(t, "WARN", e["level"])
			assert.Contains(t, e["error"], "デバイスオフライン")
		}
	}
	assert.True(t, found, "失敗デバイスのログが出力されていない")
}

func TestPresent_アカウントの致命的エラーをERRORで記録する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "壊れたアカウント",
		Err:         errors.New("認証に失敗しました"),
	}}})

	entries := logEntries(t, &buf)

	found := false
	for _, e := range entries {
		if e["level"] == "ERROR" {
			found = true
			assert.Equal(t, "壊れたアカウント", e["account"])
			assert.Contains(t, e["error"], "認証に失敗しました")
		}
	}
	assert.True(t, found, "致命的エラーのログが出力されていない")
}

func TestPresent_アカウントごとに集計を出力する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "acc1",
		Devices: []usecase.DeviceResult{
			{Device: domain.Device{Name: "a"}, Outcome: usecase.OutcomeStored},
			{Device: domain.Device{Name: "b"}, Outcome: usecase.OutcomeStored},
			{Device: domain.Device{Name: "c"}, Outcome: usecase.OutcomeSkippedCloudDisabled},
			{Device: domain.Device{Name: "d"}, Outcome: usecase.OutcomeRegisteredOnly},
			{Device: domain.Device{Name: "e"}, Outcome: usecase.OutcomeFailed, Err: errors.New("x")},
		},
	}}})

	entries := logEntries(t, &buf)

	found := false
	for _, e := range entries {
		if e["msg"] == "アカウントの収集が完了しました" {
			found = true
			assert.Equal(t, float64(2), e["stored"])
			assert.Equal(t, float64(1), e["skipped"])
			assert.Equal(t, float64(1), e["registered_only"])
			assert.Equal(t, float64(1), e["failed"])
		}
	}
	assert.True(t, found, "集計ログが出力されていない")
}

func TestPresent_空のレポートでもパニックしない(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	assert.NotPanics(t, func() {
		p.Present(usecase.CollectReport{})
	})
}

func TestPresent_OutcomeFailedだがエラーなしの場合もWARNで記録する(t *testing.T) {
	var buf bytes.Buffer
	p := presenter.NewSlog(newTestLogger(&buf))

	p.Present(usecase.CollectReport{Accounts: []usecase.AccountResult{{
		AccountName: "acc1",
		Devices: []usecase.DeviceResult{{
			Device:  domain.Device{ID: "d1", Name: "オフラインBot"},
			Outcome: usecase.OutcomeFailed,
			Err:     nil, // Err は nil だが Outcome は Failed
		}},
	}}})

	entries := logEntries(t, &buf)

	found := false
	for _, e := range entries {
		if e["device"] == "オフラインBot" {
			found = true
			assert.Equal(t, "WARN", e["level"])
			assert.Equal(t, "failed", e["outcome"])
			// Err が nil のため error フィールドは存在しない
			_, hasError := e["error"]
			assert.False(t, hasError, "error フィールドは存在しないはず")
		}
	}
	assert.True(t, found, "OutcomeFailedのログが出力されていない")
}

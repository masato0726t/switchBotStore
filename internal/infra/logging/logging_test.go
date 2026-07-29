package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/infra/logging"
)

var testNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

func TestNew_JSON形式で出力する(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelInfo)

	logger.Info("保存しました", "device", "温湿度計", "count", 3)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "保存しました", entry["msg"])
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "温湿度計", entry["device"])
	assert.Equal(t, float64(3), entry["count"])
}

func TestSetup_日付名のログファイルを作る(t *testing.T) {
	dir := t.TempDir()

	logger, closeFn, err := logging.Setup(dir, slog.LevelInfo, testNow)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeFn() })

	logger.Info("テストメッセージ")
	require.NoError(t, closeFn())

	path := filepath.Join(dir, "2026-07-29.log")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "テストメッセージ")
}

func TestSetup_存在しないディレクトリを作る(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")

	_, closeFn, err := logging.Setup(dir, slog.LevelInfo, testNow)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeFn() })

	assert.DirExists(t, dir)
}

func TestSetup_既存ファイルに追記する(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-07-29.log")
	require.NoError(t, os.WriteFile(path, []byte("既存の行\n"), 0o644))

	logger, closeFn, err := logging.Setup(dir, slog.LevelInfo, testNow)
	require.NoError(t, err)
	logger.Info("追記した行")
	require.NoError(t, closeFn())

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "既存の行")
	assert.Contains(t, string(content), "追記した行")
}

func TestSetup_logDirが空なら標準エラー出力のみを使う(t *testing.T) {
	logger, closeFn, err := logging.Setup("", slog.LevelInfo, testNow)
	require.NoError(t, err)
	require.NotNil(t, logger)
	assert.NoError(t, closeFn())
}

func TestSetup_closeを二重に呼んでもパニックしない(t *testing.T) {
	logger, closeFn, err := logging.Setup(t.TempDir(), slog.LevelInfo, testNow)
	require.NoError(t, err)
	logger.Info("何か")

	require.NoError(t, closeFn())
	assert.NotPanics(t, func() { _ = closeFn() })
}

func TestNew_指定レベル未満は出力しない(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelWarn)

	logger.Info("これは出力されない")
	logger.Warn("これは出力される")
	logger.Error("これも出力される")

	output := buf.String()
	assert.NotContains(t, output, "これは出力されない",
		"WARN 指定時は Info を抑止する")
	assert.Contains(t, output, "これは出力される")
	assert.Contains(t, output, "これも出力される")
}

func TestNew_DEBUG指定ならInfoも出力する(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug)

	logger.Debug("デバッグ")
	logger.Info("情報")

	output := buf.String()
	assert.Contains(t, output, "デバッグ")
	assert.Contains(t, output, "情報")
}

func TestSetup_レベルがファイル出力にも適用される(t *testing.T) {
	dir := t.TempDir()

	logger, closeFn, err := logging.Setup(dir, slog.LevelError, testNow)
	require.NoError(t, err)

	logger.Warn("抑止されるはず")
	logger.Error("記録されるはず")
	require.NoError(t, closeFn())

	content, err := os.ReadFile(filepath.Join(dir, "2026-07-29.log"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "抑止されるはず")
	assert.Contains(t, string(content), "記録されるはず")
}

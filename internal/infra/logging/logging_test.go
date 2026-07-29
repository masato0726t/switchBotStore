package logging_test

import (
	"bytes"
	"encoding/json"
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
	logger := logging.New(&buf)

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

	logger, closeFn, err := logging.Setup(dir, testNow)
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

	_, closeFn, err := logging.Setup(dir, testNow)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeFn() })

	assert.DirExists(t, dir)
}

func TestSetup_既存ファイルに追記する(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-07-29.log")
	require.NoError(t, os.WriteFile(path, []byte("既存の行\n"), 0o644))

	logger, closeFn, err := logging.Setup(dir, testNow)
	require.NoError(t, err)
	logger.Info("追記した行")
	require.NoError(t, closeFn())

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "既存の行")
	assert.Contains(t, string(content), "追記した行")
}

func TestSetup_logDirが空なら標準エラー出力のみを使う(t *testing.T) {
	logger, closeFn, err := logging.Setup("", testNow)
	require.NoError(t, err)
	require.NotNil(t, logger)
	assert.NoError(t, closeFn())
}

func TestSetup_closeを二重に呼んでもパニックしない(t *testing.T) {
	logger, closeFn, err := logging.Setup(t.TempDir(), testNow)
	require.NoError(t, err)
	logger.Info("何か")

	require.NoError(t, closeFn())
	assert.NotPanics(t, func() { _ = closeFn() })
}

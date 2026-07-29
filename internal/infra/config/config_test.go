package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/infra/config"
)

// writeConfig は一時ディレクトリに config.json を書き、そのパスを返す。
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

const validConfig = `{
  "database": {"host":"localhost","port":3306,"user":"root","password":"pw","name":"switchbot_db"},
  "accounts": [{"name":"メイン","token":"tok1","secret":"sec1"}],
  "log_dir": "logs"
}`

func TestLoad_正常な設定を読み込む(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, validConfig))

	require.NoError(t, err)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 3306, cfg.Database.Port)
	assert.Equal(t, "switchbot_db", cfg.Database.Name)
	assert.Equal(t, "logs", cfg.LogDir)
	require.Len(t, cfg.Accounts, 1)
	assert.Equal(t, "tok1", cfg.Accounts[0].Token)
	assert.Equal(t, "sec1", cfg.Accounts[0].Secret)
}

func TestLoad_port未指定なら3306を補う(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, `{
	  "database": {"host":"localhost","user":"root","password":"pw","name":"db"},
	  "accounts": [{"name":"a","token":"t","secret":"s"}]
	}`))

	require.NoError(t, err)
	assert.Equal(t, config.DefaultPort, cfg.Database.Port)
}

func TestLoad_ファイルが存在しないとエラー(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}

func TestLoad_不正なJSONでエラー(t *testing.T) {
	_, err := config.Load(writeConfig(t, `{ これはJSONではない`))
	require.Error(t, err)
}

func TestLoad_検証エラーになるケース(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "accounts が空配列",
			content: `{"database":{"host":"h"},"accounts":[]}`,
		},
		{
			name:    "accounts が未指定",
			content: `{"database":{"host":"h"}}`,
		},
		{
			name:    "token が空",
			content: `{"database":{"host":"h"},"accounts":[{"name":"a","token":"","secret":"s"}]}`,
		},
		{
			name:    "secret が空",
			content: `{"database":{"host":"h"},"accounts":[{"name":"a","token":"t","secret":""}]}`,
		},
		{
			name:    "database.host が空",
			content: `{"database":{"host":""},"accounts":[{"name":"a","token":"t","secret":"s"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, tt.content))
			require.Error(t, err)
		})
	}
}

func TestLoad_現行実装が検証しない項目は空でも通る(t *testing.T) {
	// database.user / password / name と accounts[].name は現行実装が
	// 検証していないため、validator 導入後も通らなければならない。
	cfg, err := config.Load(writeConfig(t, `{
	  "database": {"host":"localhost"},
	  "accounts": [{"token":"t","secret":"s"}]
	}`))

	require.NoError(t, err)
	assert.Empty(t, cfg.Database.User)
	assert.Empty(t, cfg.Database.Password)
	assert.Empty(t, cfg.Database.Name)
	assert.Empty(t, cfg.Accounts[0].Name)
}

func TestLoad_複数アカウントを読み込む(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, `{
	  "database": {"host":"h"},
	  "accounts": [
	    {"name":"メイン","token":"t1","secret":"s1"},
	    {"name":"サブ","token":"t2","secret":"s2"}
	  ]
	}`))

	require.NoError(t, err)
	require.Len(t, cfg.Accounts, 2)
	assert.Equal(t, "t2", cfg.Accounts[1].Token)
}

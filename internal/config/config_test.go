package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "port": 3306, "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [
			{"name": "acc1", "token": "tok1", "secret": "sec1"},
			{"name": "acc2", "token": "tok2", "secret": "sec2"}
		]
	}`)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 3306, cfg.Database.Port)
	require.Len(t, cfg.Accounts, 2)
	assert.Equal(t, "tok1", cfg.Accounts[0].Token)
}

func TestLoad_DefaultPort(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "a", "token": "t", "secret": "s"}]
	}`)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 3306, cfg.Database.Port)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "notexist.json"))
	require.Error(t, err)
}

func TestLoad_InvalidJSON(t *testing.T) {
	path := writeConfigFile(t, `{invalid json}`)
	_, err := Load(path)
	require.Error(t, err)
}

func TestLoad_NoAccounts(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": []
	}`)
	_, err := Load(path)
	require.Error(t, err)
}

func TestLoad_MissingToken(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "acc", "token": "", "secret": "sec"}]
	}`)
	_, err := Load(path)
	require.Error(t, err)
}

func TestLoad_MissingSecret(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "acc", "token": "tok", "secret": ""}]
	}`)
	_, err := Load(path)
	require.Error(t, err)
}

func TestLoad_MissingDBHost(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "acc", "token": "tok", "secret": "sec"}]
	}`)
	_, err := Load(path)
	require.Error(t, err)
}

func TestLoad_MultipleAccounts(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [
			{"name": "a1", "token": "t1", "secret": "s1"},
			{"name": "a2", "token": "t2", "secret": "s2"},
			{"name": "a3", "token": "t3", "secret": "s3"}
		]
	}`)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Accounts, 3)
}

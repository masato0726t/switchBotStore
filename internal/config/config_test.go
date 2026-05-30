package config

import (
	"os"
	"path/filepath"
	"testing"
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
		],
		"collect_interval_minutes": 10
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "localhost")
	}
	if cfg.Database.Port != 3306 {
		t.Errorf("Database.Port = %d, want 3306", cfg.Database.Port)
	}
	if len(cfg.Accounts) != 2 {
		t.Fatalf("len(Accounts) = %d, want 2", len(cfg.Accounts))
	}
	if cfg.Accounts[0].Token != "tok1" {
		t.Errorf("Accounts[0].Token = %q, want %q", cfg.Accounts[0].Token, "tok1")
	}
	if cfg.CollectIntervalMinutes != 10 {
		t.Errorf("CollectIntervalMinutes = %d, want 10", cfg.CollectIntervalMinutes)
	}
}

func TestLoad_DefaultInterval(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "a", "token": "t", "secret": "s"}]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if cfg.CollectIntervalMinutes != 5 {
		t.Errorf("デフォルト値 CollectIntervalMinutes = %d, want 5", cfg.CollectIntervalMinutes)
	}
}

func TestLoad_DefaultPort(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "a", "token": "t", "secret": "s"}]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if cfg.Database.Port != 3306 {
		t.Errorf("デフォルト値 Database.Port = %d, want 3306", cfg.Database.Port)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "notexist.json"))
	if err == nil {
		t.Error("ファイルが存在しない場合はエラーを返す想定")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	path := writeConfigFile(t, `{invalid json}`)
	_, err := Load(path)
	if err == nil {
		t.Error("不正なJSONの場合はエラーを返す想定")
	}
}

func TestLoad_NoAccounts(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": []
	}`)
	_, err := Load(path)
	if err == nil {
		t.Error("accounts が空の場合はエラーを返す想定")
	}
}

func TestLoad_MissingToken(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "acc", "token": "", "secret": "sec"}]
	}`)
	_, err := Load(path)
	if err == nil {
		t.Error("token が空の場合はエラーを返す想定")
	}
}

func TestLoad_MissingSecret(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "localhost", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "acc", "token": "tok", "secret": ""}]
	}`)
	_, err := Load(path)
	if err == nil {
		t.Error("secret が空の場合はエラーを返す想定")
	}
}

func TestLoad_MissingDBHost(t *testing.T) {
	path := writeConfigFile(t, `{
		"database": {"host": "", "user": "root", "password": "pw", "name": "mydb"},
		"accounts": [{"name": "acc", "token": "tok", "secret": "sec"}]
	}`)
	_, err := Load(path)
	if err == nil {
		t.Error("database.host が空の場合はエラーを返す想定")
	}
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
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if len(cfg.Accounts) != 3 {
		t.Errorf("len(Accounts) = %d, want 3", len(cfg.Accounts))
	}
}

package mysql_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"switchBotStore/internal/infra/mysql"
)

func TestConfig_DSN(t *testing.T) {
	cfg := mysql.Config{
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "pw",
		Name:     "switchbot_db",
	}

	// 現行実装 (internal/database/database.go:15-16) と同一の DSN を生成すること。
	assert.Equal(t,
		"root:pw@tcp(localhost:3306)/switchbot_db?parseTime=true&loc=Local&charset=utf8mb4",
		cfg.DSN())
}

func TestConnect_接続できないホストでエラーを返す(t *testing.T) {
	cfg := mysql.Config{Host: "127.0.0.1", Port: 1, User: "u", Password: "p", Name: "d"}

	_, _, err := mysql.Connect(t.Context(), cfg)
	assert.Error(t, err)
}

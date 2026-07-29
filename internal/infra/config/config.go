// Package config は config.json の読み込みと検証を行う。
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/go-playground/validator/v10"
)

// DefaultPort は database.port 未指定時に使う MySQL のポート。
const DefaultPort = 3306

// Config は config.json の内容。
type Config struct {
	Database Database   `json:"database"`
	Accounts []Account  `json:"accounts" validate:"required,min=1,dive"`
	LogDir   string     `json:"log_dir"`
	LogLevel slog.Level `json:"log_level"`
}

// Database は MySQL への接続情報。
//
// validate タグは現行実装の検証内容をそのまま写したもの。host 以外を
// required にすると既存の config.json が読めなくなる恐れがあるため、
// 本リファクタリングでは検証を強化しない。
type Database struct {
	Host     string `json:"host" validate:"required"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Account は SwitchBot API アカウント1件分の設定。
type Account struct {
	Name   string `json:"name"`
	Token  string `json:"token" validate:"required"`
	Secret string `json:"secret" validate:"required"`
}

// Load は path の JSON を読み込み、検証してから返す。
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("設定ファイルを開けません (%s): %w", path, err)
	}
	defer f.Close()

	cfg := &Config{}
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("設定ファイルのパースに失敗しました (%s): %w", path, err)
	}

	if err := validator.New().Struct(cfg); err != nil {
		return nil, fmt.Errorf("設定ファイルの内容が不正です (%s): %w", path, err)
	}

	if cfg.Database.Port == 0 {
		cfg.Database.Port = DefaultPort
	}
	return cfg, nil
}

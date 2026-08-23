// Package config は config.json の読み込みと検証を行う。
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
)

// DefaultPort は database.port 未指定時に使う MySQL のポート。
const DefaultPort = 3306

// DefaultDeviceListCacheTTL は device_list_cache_ttl 未指定時に使う値。
//
// デバイス構成はめったに変わらないため、1日1回取り直せば十分という判断。
// 1分間隔で実行した場合、/v1.1/devices の呼び出しは 1,440 回/日から
// 1 回/日まで減る。
const DefaultDeviceListCacheTTL = 24 * time.Hour

// Config は config.json の内容。
type Config struct {
	Database Database   `json:"database"`
	Accounts []Account  `json:"accounts" validate:"required,min=1,dive"`
	LogDir   string     `json:"log_dir"`
	LogLevel slog.Level `json:"log_level"`

	// DeviceListCacheTTL はデバイス一覧を API から取り直す間隔を
	// time.ParseDuration の書式（"24h" など）で指定する。
	// 空文字なら DefaultDeviceListCacheTTL、"0" ならキャッシュを使わない。
	DeviceListCacheTTL string `json:"device_list_cache_ttl"`

	// deviceListCacheTTL は DeviceListCacheTTL を解析した値。Load が設定する。
	deviceListCacheTTL time.Duration
}

// DeviceCacheTTL は解析済みの device_list_cache_ttl を返す。
func (c *Config) DeviceCacheTTL() time.Duration { return c.deviceListCacheTTL }

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

// parseCacheTTL は device_list_cache_ttl の文字列を解釈する。
// 空文字は既定値、負の値はエラーとする。
func parseCacheTTL(raw string) (time.Duration, error) {
	if raw == "" {
		return DefaultDeviceListCacheTTL, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("負の値は指定できません: %s", raw)
	}
	return d, nil
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

	ttl, err := parseCacheTTL(cfg.DeviceListCacheTTL)
	if err != nil {
		return nil, fmt.Errorf("device_list_cache_ttl の指定が不正です (%s): %w", path, err)
	}
	cfg.deviceListCacheTTL = ttl

	return cfg, nil
}

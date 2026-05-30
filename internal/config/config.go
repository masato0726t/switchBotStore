package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Database DBConfig  `json:"database"`
	Accounts []Account `json:"accounts"`
	LogDir   string    `json:"log_dir"`
}

type DBConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type Account struct {
	Name   string `json:"name"`
	Token  string `json:"token"`
	Secret string `json:"secret"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("設定ファイルを開けません (%s): %v", path, err)
	}
	defer f.Close()

	cfg := &Config{}
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("設定ファイルのパースに失敗: %v", err)
	}

	if len(cfg.Accounts) == 0 {
		return nil, fmt.Errorf("accounts が設定されていません")
	}
	for i, acc := range cfg.Accounts {
		if acc.Token == "" || acc.Secret == "" {
			return nil, fmt.Errorf("accounts[%d]: token と secret は必須です", i)
		}
	}
	if cfg.Database.Host == "" {
		return nil, fmt.Errorf("database.host が設定されていません")
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 3306
	}

	return cfg, nil
}

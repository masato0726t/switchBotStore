package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"switchBotStore/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

func Connect(cfg config.DBConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local&charset=utf8mb4",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("DB接続の初期化に失敗: %v", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("DBへの疎通確認に失敗: %v", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	log.Printf("データベース %s に接続しました", cfg.Name)
	return db, nil
}

// Migrate は必要なテーブルが存在しない場合に作成する
func Migrate(db *sql.DB) error {
	tables := []struct {
		name string
		ddl  string
	}{
		{
			name: "api_accounts",
			ddl: `CREATE TABLE IF NOT EXISTS api_accounts (
				id         INT AUTO_INCREMENT PRIMARY KEY,
				name       VARCHAR(255)  NOT NULL COMMENT 'アカウント識別名',
				token      VARCHAR(255)  NOT NULL COMMENT 'SwitchBot APIトークン',
				secret     VARCHAR(255)  NOT NULL COMMENT 'SwitchBot APIシークレット',
				created_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_token (token)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='SwitchBot APIアカウント'`,
		},
		{
			name: "devices",
			ddl: `CREATE TABLE IF NOT EXISTS devices (
				id                   INT AUTO_INCREMENT PRIMARY KEY,
				api_account_id       INT          NOT NULL COMMENT 'api_accounts.id',
				device_id            VARCHAR(255) NOT NULL COMMENT 'SwitchBot デバイスID',
				device_name          VARCHAR(255)          COMMENT 'デバイス名',
				device_type          VARCHAR(100)          COMMENT 'デバイス種別',
				hub_device_id        VARCHAR(255)          COMMENT '接続先ハブのデバイスID',
				enable_cloud_service TINYINT(1)   NOT NULL DEFAULT 0 COMMENT 'クラウドサービス有効フラグ',
				is_virtual_infrared  TINYINT(1)   NOT NULL DEFAULT 0 COMMENT '仮想赤外線リモコンフラグ',
				created_at           TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at           TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				FOREIGN KEY (api_account_id) REFERENCES api_accounts(id),
				UNIQUE KEY uq_account_device (api_account_id, device_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='SwitchBot デバイス情報'`,
		},
		{
			name: "device_status_logs",
			ddl: `CREATE TABLE IF NOT EXISTS device_status_logs (
				id          BIGINT    AUTO_INCREMENT PRIMARY KEY,
				device_id   INT       NOT NULL COMMENT 'devices.id',
				status_data JSON      NOT NULL COMMENT 'APIから取得したステータスデータ(JSON)',
				recorded_at DATETIME  NOT NULL COMMENT 'データ収集日時',
				created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (device_id) REFERENCES devices(id),
				INDEX idx_device_recorded (device_id, recorded_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='デバイスステータス収集ログ'`,
		},
		{
			name: "system_settings",
			ddl: `CREATE TABLE IF NOT EXISTS system_settings (
				id            INT AUTO_INCREMENT PRIMARY KEY,
				setting_key   VARCHAR(100) NOT NULL COMMENT '設定キー',
				setting_value TEXT                  COMMENT '設定値',
				created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				UNIQUE KEY uq_setting_key (setting_key)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='システム設定'`,
		},
	}

	for _, t := range tables {
		if _, err := db.Exec(t.ddl); err != nil {
			return fmt.Errorf("テーブル %s の作成に失敗: %v", t.name, err)
		}
		log.Printf("テーブル確認/作成: %s", t.name)
	}

	return nil
}

func IsFirstRun(db *sql.DB) (bool, error) {
	var val sql.NullString
	err := db.QueryRow(
		"SELECT setting_value FROM system_settings WHERE setting_key = 'initial_collect_done'",
	).Scan(&val)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !val.Valid || val.String != "1", nil
}

func MarkFirstRunDone(db *sql.DB) error {
	_, err := db.Exec(`
		INSERT INTO system_settings (setting_key, setting_value)
		VALUES ('initial_collect_done', '1')
		ON DUPLICATE KEY UPDATE setting_value = '1', updated_at = CURRENT_TIMESTAMP`)
	return err
}

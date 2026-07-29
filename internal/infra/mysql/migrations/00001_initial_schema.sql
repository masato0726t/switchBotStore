-- +goose Up
-- 現行 internal/database/database.go の DDL をそのまま移設したもの。
-- 全て IF NOT EXISTS のため、既にテーブルが存在する DB では no-op になり、
-- goose のバージョン1として記録されるだけになる。
CREATE TABLE IF NOT EXISTS api_accounts (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(255)  NOT NULL COMMENT 'アカウント識別名',
    token      VARCHAR(255)  NOT NULL COMMENT 'SwitchBot APIトークン',
    secret     VARCHAR(255)  NOT NULL COMMENT 'SwitchBot APIシークレット',
    created_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_token (token)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='SwitchBot APIアカウント';

CREATE TABLE IF NOT EXISTS devices (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='SwitchBot デバイス情報';

CREATE TABLE IF NOT EXISTS device_status_logs (
    id          BIGINT    AUTO_INCREMENT PRIMARY KEY,
    device_id   INT       NOT NULL COMMENT 'devices.id',
    status_data JSON      NOT NULL COMMENT 'APIから取得したステータスデータ(JSON)',
    recorded_at DATETIME  NOT NULL COMMENT 'データ収集日時',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (device_id) REFERENCES devices(id),
    INDEX idx_device_recorded (device_id, recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='デバイスステータス収集ログ';

CREATE TABLE IF NOT EXISTS system_settings (
    id            INT AUTO_INCREMENT PRIMARY KEY,
    setting_key   VARCHAR(100) NOT NULL COMMENT '設定キー',
    setting_value TEXT                  COMMENT '設定値',
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_setting_key (setting_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='システム設定';

-- +goose Down
-- 実データを失うため、意図的にロールバックを行わない。
SELECT 1;

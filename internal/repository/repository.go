package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"switchBotStore/internal/switchbot"
)

// Repository はDBへのデータ永続化を抽象化する
type Repository interface {
	UpsertAccount(name, token, secret string) (int64, error)
	UpsertDevice(accountID int64, dev switchbot.DeviceInfo, isVirtualInfrared bool) (int64, error)
	SaveStatusLog(deviceID int64, status json.RawMessage, recordedAt time.Time) error
}

// SQL は Repository の MySQL実装
type SQL struct {
	db *sql.DB
}

func NewSQL(db *sql.DB) *SQL {
	return &SQL{db: db}
}

func (r *SQL) UpsertAccount(name, token, secret string) (int64, error) {
	_, err := r.db.Exec(`
		INSERT INTO api_accounts (name, token, secret)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			name       = VALUES(name),
			secret     = VALUES(secret),
			updated_at = CURRENT_TIMESTAMP`,
		name, token, secret)
	if err != nil {
		return 0, err
	}

	var id int64
	err = r.db.QueryRow("SELECT id FROM api_accounts WHERE token = ?", token).Scan(&id)
	return id, err
}

func (r *SQL) UpsertDevice(accountID int64, dev switchbot.DeviceInfo, isVirtualInfrared bool) (int64, error) {
	_, err := r.db.Exec(`
		INSERT INTO devices
			(api_account_id, device_id, device_name, device_type, hub_device_id, enable_cloud_service, is_virtual_infrared)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			device_name          = VALUES(device_name),
			device_type          = VALUES(device_type),
			hub_device_id        = VALUES(hub_device_id),
			enable_cloud_service = VALUES(enable_cloud_service),
			updated_at           = CURRENT_TIMESTAMP`,
		accountID, dev.DeviceID, dev.DeviceName, dev.DeviceType,
		dev.HubDeviceID, boolToInt(dev.EnableCloudService), boolToInt(isVirtualInfrared))
	if err != nil {
		return 0, err
	}

	var id int64
	err = r.db.QueryRow(
		"SELECT id FROM devices WHERE api_account_id = ? AND device_id = ?",
		accountID, dev.DeviceID,
	).Scan(&id)
	return id, err
}

func (r *SQL) SaveStatusLog(deviceID int64, status json.RawMessage, recordedAt time.Time) error {
	_, err := r.db.Exec(
		"INSERT INTO device_status_logs (device_id, status_data, recorded_at) VALUES (?, ?, ?)",
		deviceID, string(status), recordedAt.Format("2006-01-02 15:04:05"),
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

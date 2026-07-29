// Package persistence は GORM による usecase.Repository の実装。
//
// GORM のタグはこのパッケージの外へ出さない。domain は GORM を知らない。
// スキーマの正は internal/infra/mysql/migrations/*.sql 側にあり、ここの
// モデルはそこへのマッピングにすぎない（VerifySchema がズレを検出する）。
package persistence

import "time"

// apiAccountModel は api_accounts テーブルの1行。
type apiAccountModel struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Name      string    `gorm:"column:name"`
	Token     string    `gorm:"column:token"`
	Secret    string    `gorm:"column:secret"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName は GORM が使うテーブル名を返す。
func (apiAccountModel) TableName() string { return "api_accounts" }

// deviceModel は devices テーブルの1行。
type deviceModel struct {
	ID                 int64     `gorm:"column:id;primaryKey;autoIncrement"`
	APIAccountID       int64     `gorm:"column:api_account_id"`
	DeviceID           string    `gorm:"column:device_id"`
	DeviceName         string    `gorm:"column:device_name"`
	DeviceType         string    `gorm:"column:device_type"`
	HubDeviceID        string    `gorm:"column:hub_device_id"`
	EnableCloudService bool      `gorm:"column:enable_cloud_service"`
	IsVirtualInfrared  bool      `gorm:"column:is_virtual_infrared"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

// TableName は GORM が使うテーブル名を返す。
func (deviceModel) TableName() string { return "devices" }

// deviceStatusLogModel は device_status_logs テーブルの1行。
type deviceStatusLogModel struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	DeviceID   int64     `gorm:"column:device_id"`
	StatusData string    `gorm:"column:status_data"`
	RecordedAt time.Time `gorm:"column:recorded_at"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName は GORM が使うテーブル名を返す。
func (deviceStatusLogModel) TableName() string { return "device_status_logs" }

// allModels は VerifySchema が検査する全モデル。
// 新しいモデルを追加したらここにも足すこと。
func allModels() []any {
	return []any{
		apiAccountModel{},
		deviceModel{},
		deviceStatusLogModel{},
	}
}

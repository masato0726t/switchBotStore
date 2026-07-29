package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// Store が出力ポートを満たすことをコンパイル時に検証する。
var _ usecase.Repository = (*Store)(nil)

// Store は usecase.Repository の GORM 実装。
type Store struct {
	db *gorm.DB
}

// New は Store を生成する。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// SaveAccount はアカウントを登録し、token が同一の行が既にあれば更新する。
//
// MySQL の ON DUPLICATE KEY UPDATE は更新時に LAST_INSERT_ID() を更新しない
// ため、INSERT が返す ID は信頼できない。そのため id を SELECT し直す
// 2 クエリ方式を採る（現行実装と同じ）。
func (s *Store) SaveAccount(ctx context.Context, a domain.Account) (domain.AccountID, error) {
	m := toAccountModel(a)

	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "token"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "secret", "updated_at"}),
		}).
		Create(&m).Error
	if err != nil {
		return 0, fmt.Errorf("アカウントの保存に失敗しました: %w", err)
	}

	// 受け皿はモデル構造体にする。単一カラムをプリミティブ型へ直接
	// Take する書き方は GORM の挙動が分かりにくいため使わない。
	var saved apiAccountModel
	err = s.db.WithContext(ctx).
		Select("id").
		Where("token = ?", a.Credential.Token).
		Take(&saved).Error
	if err != nil {
		return 0, fmt.Errorf("アカウント ID の取得に失敗しました: %w", err)
	}
	return domain.AccountID(saved.ID), nil
}

// SaveDevice はデバイスを登録し、(api_account_id, device_id) が同一の行が
// 既にあれば更新する。ID の取得方法は SaveAccount と同じ理由で 2 クエリ。
func (s *Store) SaveDevice(ctx context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error) {
	m := toDeviceModel(accountID, d)

	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "api_account_id"}, {Name: "device_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"device_name", "device_type", "hub_device_id", "enable_cloud_service", "updated_at",
			}),
		}).
		Create(&m).Error
	if err != nil {
		return 0, fmt.Errorf("デバイスの保存に失敗しました: %w", err)
	}

	var saved deviceModel
	err = s.db.WithContext(ctx).
		Select("id").
		Where("api_account_id = ? AND device_id = ?", int64(accountID), string(d.ID)).
		Take(&saved).Error
	if err != nil {
		return 0, fmt.Errorf("デバイス ID の取得に失敗しました: %w", err)
	}
	return domain.DeviceRecordID(saved.ID), nil
}

// AppendStatus はステータス収集ログを1件追加する。
func (s *Store) AppendStatus(ctx context.Context, id domain.DeviceRecordID, snapshot domain.StatusSnapshot) error {
	m := toStatusLogModel(id, snapshot)
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("ステータスログの保存に失敗しました: %w", err)
	}
	return nil
}

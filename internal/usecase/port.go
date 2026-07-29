// Package usecase はアプリケーション固有のユースケースを実装する。
//
// domain 以外の内部パッケージには依存しない。ログ出力も行わず、処理結果は
// CollectReport として呼び出し元に返す（ログ整形は adapter/presenter の責務）。
package usecase

import (
	"context"
	"time"

	"switchBotStore/internal/domain"
)

// DeviceGateway は SwitchBot API へのアクセスを抽象化する出力ポート。
//
// 認証情報を引数で受け取るため、実装は全アカウントで1インスタンスを共有できる。
type DeviceGateway interface {
	// ListDevices はアカウントに登録された全デバイスを返す。
	// 物理デバイスと赤外線リモコンは DeviceKind で区別された単一のリストになる。
	ListDevices(ctx context.Context, cred domain.Credential) ([]domain.Device, error)

	// FetchStatus は指定デバイスの現在のステータスを返す。
	FetchStatus(ctx context.Context, cred domain.Credential, id domain.DeviceID) (domain.StatusPayload, error)
}

// Repository は収集結果の永続化を抽象化する出力ポート。
type Repository interface {
	// SaveAccount はアカウントを登録し、既に存在する場合は更新して ID を返す。
	SaveAccount(ctx context.Context, a domain.Account) (domain.AccountID, error)

	// SaveDevice はデバイスを登録し、既に存在する場合は更新して ID を返す。
	SaveDevice(ctx context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error)

	// AppendStatus はステータス収集ログを1件追加する。
	AppendStatus(ctx context.Context, id domain.DeviceRecordID, s domain.StatusSnapshot) error
}

// Clock は現在時刻の取得を抽象化する（テストで固定するため）。
type Clock interface {
	Now() time.Time
}

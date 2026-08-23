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

// RateLimitReporter は上流が応答に載せてくる残り呼び出し回数を報告できる
// DeviceGateway が、追加で満たすインターフェース。
//
// SwitchBot API は成功した応答にも X-RateLimit-Remaining を付けてくる。
// 残量を問い合わせる専用の API は無いが、この値を毎回のログに残せば
// API を1回も余分に呼ばずに「残量の推移・実際の上限・いつ戻るか」を
// 観測できる。DeviceGateway 本体に含めないのは、この報告ができない
// 実装（テスト用の偽物など）も引き続き使えるようにするため。
type RateLimitReporter interface {
	// TakeRemaining は前回の呼び出し以降に受け取った応答のうち、最後の
	// X-RateLimit-Remaining をそのまま返し、内部の記録を空に戻す。
	//
	// 読んだら消すのは、アカウントごとに枠が別なため。API を1回も呼ばな
	// かったアカウントに、直前のアカウントの残量が付いてしまうのを防ぐ。
	// 応答が無かった場合とヘッダーが無かった場合は空文字列を返す。
	TakeRemaining() string
}

// DeviceRecord は永続化済みのデバイスと、その devices.id の対。
//
// ID がゼロ値のときは「まだ devices へ登録していない」ことを表す。
type DeviceRecord struct {
	ID     domain.DeviceRecordID
	Device domain.Device
}

// Repository は収集結果の永続化を抽象化する出力ポート。
type Repository interface {
	// SaveAccount はアカウントを登録し、既に存在する場合は更新して ID を返す。
	SaveAccount(ctx context.Context, a domain.Account) (domain.AccountID, error)

	// LoadDeviceCache は accountID に登録済みのデバイスと、その一覧を API から
	// 最後に取得した日時を返す。一度も取得していなければ syncedAt はゼロ値。
	LoadDeviceCache(ctx context.Context, accountID domain.AccountID) (devices []DeviceRecord, syncedAt time.Time, err error)

	// MarkDevicesSynced はデバイス一覧を API から取得した日時を記録する。
	MarkDevicesSynced(ctx context.Context, accountID domain.AccountID, at time.Time) error

	// LoadRateLimitState はレート制限の待機状態を読み出す。
	// 待機していなければゼロ値を返す。
	LoadRateLimitState(ctx context.Context, accountID domain.AccountID) (RateLimitState, error)

	// SaveRateLimitState はレート制限の待機状態を保存する。
	// ゼロ値を渡すと待機を解除する。
	SaveRateLimitState(ctx context.Context, accountID domain.AccountID, state RateLimitState) error

	// SaveDevice はデバイスを登録し、既に存在する場合は更新して ID を返す。
	SaveDevice(ctx context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error)

	// AppendStatus はステータス収集ログを1件追加する。
	AppendStatus(ctx context.Context, id domain.DeviceRecordID, s domain.StatusSnapshot) error
}

// Clock は現在時刻の取得を抽象化する（テストで固定するため）。
type Clock interface {
	Now() time.Time
}

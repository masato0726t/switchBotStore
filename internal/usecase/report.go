package usecase

import (
	"errors"

	"switchBotStore/internal/domain"
)

// Outcome は1デバイスに対する収集処理の結末。
type Outcome int

const (
	// OutcomeStored はステータスを取得して保存できたことを表す。
	OutcomeStored Outcome = iota
	// OutcomeSkippedCloudDisabled はクラウドサービスが無効なためステータス取得を省いたことを表す。
	OutcomeSkippedCloudDisabled
	// OutcomeRegisteredOnly は赤外線リモコンのためデバイス登録のみ行ったことを表す。
	OutcomeRegisteredOnly
	// OutcomeFailed はデバイス単位の処理が失敗したことを表す。
	OutcomeFailed
)

// String はログ出力用の識別子を返す。
func (o Outcome) String() string {
	switch o {
	case OutcomeStored:
		return "stored"
	case OutcomeSkippedCloudDisabled:
		return "skipped_cloud_disabled"
	case OutcomeRegisteredOnly:
		return "registered_only"
	case OutcomeFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// DeviceResult は1デバイスに対する処理結果。
type DeviceResult struct {
	Device  domain.Device
	Outcome Outcome
	Err     error
}

// AccountResult は1アカウントに対する処理結果。
//
// Err はアカウント単位の致命的エラー（認証失敗、デバイス一覧の取得失敗など）。
// 個々のデバイスの失敗は Devices[i].Err に入り、Err には現れない。
type AccountResult struct {
	AccountName string
	Devices     []DeviceResult
	Err         error
}

// CollectReport は収集処理全体の結果。
type CollectReport struct {
	Accounts []AccountResult
}

// FatalError はアカウント単位の致命的エラーを集約して返す。1件もなければ nil。
//
// 呼び出し元はこの戻り値をそのままプロセスの終了コード判定に使う
// （非 nil なら exit 1）。デバイス1台の失敗は含まれないため、
// オフラインのデバイスがあっても正常終了する。
func (r CollectReport) FatalError() error {
	errs := make([]error, 0, len(r.Accounts))
	for _, a := range r.Accounts {
		if a.Err != nil {
			errs = append(errs, a.Err)
		}
	}
	return errors.Join(errs...)
}

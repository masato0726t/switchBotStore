package usecase

import (
	"context"
	"fmt"
	"time"

	"switchBotStore/internal/domain"
)

// CollectStatus は全アカウントのデバイスステータスを収集して永続化するユースケース。
type CollectStatus struct {
	gateway DeviceGateway
	repo    Repository
	clock   Clock
}

// NewCollectStatus は CollectStatus を生成する。
func NewCollectStatus(gateway DeviceGateway, repo Repository, clock Clock) *CollectStatus {
	return &CollectStatus{gateway: gateway, repo: repo, clock: clock}
}

// Execute は accounts の全デバイスからステータスを収集する。
//
// エラーは戻り値ではなく CollectReport に格納される。デバイス1台の失敗では
// 処理を止めず、アカウント単位の失敗のみ AccountResult.Err に記録して
// 次のアカウントへ進む。
func (uc *CollectStatus) Execute(ctx context.Context, accounts []domain.Account) CollectReport {
	// 同一バッチのログを同じ時刻でグルーピングできるよう、先頭で1回だけ取得する。
	recordedAt := uc.clock.Now()

	report := CollectReport{Accounts: make([]AccountResult, 0, len(accounts))}
	for _, acc := range accounts {
		report.Accounts = append(report.Accounts, uc.collectAccount(ctx, acc, recordedAt))
	}
	return report
}

func (uc *CollectStatus) collectAccount(ctx context.Context, acc domain.Account, recordedAt time.Time) AccountResult {
	result := AccountResult{AccountName: acc.Name}

	accountID, err := uc.repo.SaveAccount(ctx, acc)
	if err != nil {
		result.Err = fmt.Errorf("アカウントの登録に失敗しました: %w", err)
		return result
	}

	devices, err := uc.gateway.ListDevices(ctx, acc.Credential)
	if err != nil {
		result.Err = fmt.Errorf("デバイス一覧の取得に失敗しました: %w", err)
		return result
	}

	result.Devices = make([]DeviceResult, 0, len(devices))
	for _, dev := range devices {
		result.Devices = append(result.Devices,
			uc.collectDevice(ctx, acc.Credential, accountID, dev, recordedAt))
	}
	return result
}

func (uc *CollectStatus) collectDevice(
	ctx context.Context,
	cred domain.Credential,
	accountID domain.AccountID,
	dev domain.Device,
	recordedAt time.Time,
) DeviceResult {
	recordID, err := uc.repo.SaveDevice(ctx, accountID, dev)
	if err != nil {
		return failed(dev, fmt.Errorf("デバイスの登録に失敗しました: %w", err))
	}

	// ステータスを取得できないデバイスは、登録だけ済ませて次へ進む。
	if !dev.StatusReadable() {
		return DeviceResult{Device: dev, Outcome: skipOutcome(dev)}
	}

	payload, err := uc.gateway.FetchStatus(ctx, cred, dev.ID)
	if err != nil {
		return failed(dev, fmt.Errorf("ステータスの取得に失敗しました: %w", err))
	}

	snapshot := domain.StatusSnapshot{Payload: payload, RecordedAt: recordedAt}
	if err := uc.repo.AppendStatus(ctx, recordID, snapshot); err != nil {
		return failed(dev, fmt.Errorf("ステータスログの保存に失敗しました: %w", err))
	}

	return DeviceResult{Device: dev, Outcome: OutcomeStored}
}

func failed(dev domain.Device, err error) DeviceResult {
	return DeviceResult{Device: dev, Outcome: OutcomeFailed, Err: err}
}

// skipOutcome はステータスを取得しなかった理由を Outcome として返す。
func skipOutcome(dev domain.Device) Outcome {
	if dev.Kind == domain.DeviceKindInfraredRemote {
		return OutcomeRegisteredOnly
	}
	return OutcomeSkippedCloudDisabled
}

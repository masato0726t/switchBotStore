package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"switchBotStore/internal/domain"
)

// errDeviceRegistration は devices への登録に失敗したことを表す番兵エラー。
// 同期時刻を進めてよいかの判定に使う。
var errDeviceRegistration = errors.New("デバイスの登録に失敗しました")

// CollectStatus は全アカウントのデバイスステータスを収集して永続化するユースケース。
type CollectStatus struct {
	gateway        DeviceGateway
	repo           Repository
	clock          Clock
	deviceCacheTTL time.Duration
}

// NewCollectStatus は CollectStatus を生成する。
//
// deviceCacheTTL はデバイス一覧を API から取り直す間隔。この時間内は
// 前回取得した一覧（devices テーブル）を再利用し、/v1.1/devices の
// 呼び出しを省く。0 を渡すとキャッシュを使わず毎回 API を叩く。
func NewCollectStatus(gateway DeviceGateway, repo Repository, clock Clock, deviceCacheTTL time.Duration) *CollectStatus {
	return &CollectStatus{gateway: gateway, repo: repo, clock: clock, deviceCacheTTL: deviceCacheTTL}
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

	records, fromAPI, err := uc.deviceRecords(ctx, acc, accountID, recordedAt)
	if err != nil {
		result.Err = err
		return result
	}

	result.Devices = make([]DeviceResult, 0, len(records))
	for _, rec := range records {
		result.Devices = append(result.Devices,
			uc.collectDevice(ctx, acc.Credential, accountID, rec, recordedAt))
	}

	// API から取り直した一覧を全件 devices へ反映できたときだけ、同期時刻を
	// 進める。1台でも登録に失敗していれば時刻を据え置き、次回もう一度 API を
	// 叩き直させる（欠けたままの一覧をキャッシュとして使い続けないため）。
	if fromAPI && allRegistered(result.Devices) {
		if err := uc.repo.MarkDevicesSynced(ctx, accountID, recordedAt); err != nil {
			// 単純な UPDATE の失敗は DB 側の異常を示すため、アカウント単位の
			// 致命的エラーとして扱う（黙って毎回 API を叩き続けるより、
			// 気づける形で失敗させる）。
			result.Err = fmt.Errorf("デバイス一覧の同期時刻の記録に失敗しました: %w", err)
		}
	}
	return result
}

// deviceRecords は処理対象のデバイス一覧を返す。
//
// fromAPI は API から取り直したかどうか。false のときは devices テーブルの
// 内容をそのまま再利用しており、SwitchBot API を1回も呼んでいない。
func (uc *CollectStatus) deviceRecords(
	ctx context.Context,
	acc domain.Account,
	accountID domain.AccountID,
	now time.Time,
) (records []DeviceRecord, fromAPI bool, err error) {
	if cached, ok, err := uc.cachedDevices(ctx, accountID, now); err != nil {
		return nil, false, err
	} else if ok {
		return cached, false, nil
	}

	devices, err := uc.gateway.ListDevices(ctx, acc.Credential)
	if err != nil {
		return nil, false, fmt.Errorf("デバイス一覧の取得に失敗しました: %w", err)
	}

	// ID は未確定。collectDevice が SaveDevice で採番する。
	records = make([]DeviceRecord, 0, len(devices))
	for _, dev := range devices {
		records = append(records, DeviceRecord{Device: dev})
	}
	return records, true, nil
}

// cachedDevices は再利用できるデバイス一覧があれば返す。
//
// TTL 内であっても一覧が空なら再利用しない。初回実行時や、デバイスを
// 1台も登録できていない状態で同期時刻だけが残っている場合に、
// 永久に API を叩かなくなるのを避けるため。
func (uc *CollectStatus) cachedDevices(
	ctx context.Context,
	accountID domain.AccountID,
	now time.Time,
) ([]DeviceRecord, bool, error) {
	if uc.deviceCacheTTL <= 0 {
		return nil, false, nil
	}

	cached, syncedAt, err := uc.repo.LoadDeviceCache(ctx, accountID)
	if err != nil {
		return nil, false, fmt.Errorf("デバイス一覧のキャッシュ読み込みに失敗しました: %w", err)
	}
	if len(cached) == 0 || syncedAt.IsZero() {
		return nil, false, nil
	}
	// 時計が巻き戻った場合 (now < syncedAt) も期限切れ扱いにして API を叩く。
	if elapsed := now.Sub(syncedAt); elapsed < 0 || elapsed >= uc.deviceCacheTTL {
		return nil, false, nil
	}
	return cached, true, nil
}

// allRegistered は全デバイスが devices テーブルへ登録できたかを返す。
func allRegistered(results []DeviceResult) bool {
	for _, r := range results {
		if r.Outcome == OutcomeFailed && errors.Is(r.Err, errDeviceRegistration) {
			return false
		}
	}
	return true
}

func (uc *CollectStatus) collectDevice(
	ctx context.Context,
	cred domain.Credential,
	accountID domain.AccountID,
	rec DeviceRecord,
	recordedAt time.Time,
) DeviceResult {
	dev := rec.Device

	// キャッシュ由来のデバイスは devices.id が確定済みなので登録し直さない。
	recordID := rec.ID
	if recordID == 0 {
		id, err := uc.repo.SaveDevice(ctx, accountID, dev)
		if err != nil {
			return failed(dev, fmt.Errorf("%w: %w", errDeviceRegistration, err))
		}
		recordID = id
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
		return failed(dev, fmt.Errorf("ステータスの保存に失敗しました: %w", err))
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

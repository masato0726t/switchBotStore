// Package presenter はユースケースの実行結果を人間と機械が読める形に整形する。
package presenter

import (
	"log/slog"

	"switchBotStore/internal/usecase"
)

// SlogPresenter は CollectReport を構造化ログとして出力する。
type SlogPresenter struct {
	logger *slog.Logger
}

// NewSlog は SlogPresenter を生成する。
func NewSlog(logger *slog.Logger) *SlogPresenter {
	return &SlogPresenter{logger: logger}
}

// Present はレポート全体をログに書き出す。
func (p *SlogPresenter) Present(report usecase.CollectReport) {
	for _, acc := range report.Accounts {
		p.presentAccount(acc)
	}
}

func (p *SlogPresenter) presentAccount(acc usecase.AccountResult) {
	if acc.Err != nil {
		p.logger.Error("アカウントの収集に失敗しました",
			"account", acc.AccountName,
			"error", acc.Err.Error())
		// アカウント単位の致命的エラーは、デバイスの処理を開始する前
		// （アカウント登録またはデバイス一覧取得）にのみ発生する。
		// したがって Err が非 nil のとき Devices は空であり、
		// ここで return してもデバイスのログは失われない。
		return
	}

	for _, dev := range acc.Devices {
		p.presentDevice(acc.AccountName, dev)
	}

	counts := tally(acc.Devices)
	p.logger.Info("アカウントの収集が完了しました",
		"account", acc.AccountName,
		"total", len(acc.Devices),
		"stored", counts[usecase.OutcomeStored],
		"skipped", counts[usecase.OutcomeSkippedCloudDisabled],
		"registered_only", counts[usecase.OutcomeRegisteredOnly],
		"failed", counts[usecase.OutcomeFailed])
}

func (p *SlogPresenter) presentDevice(accountName string, dev usecase.DeviceResult) {
	attrs := []any{
		"account", accountName,
		"device", dev.Device.Name,
		"device_id", string(dev.Device.ID),
		"type", dev.Device.Type,
		"outcome", dev.Outcome.String(),
	}

	// ログレベルはログ行の outcome フィールドと常に一致するべき。
	// outcome が "failed" なら WARN として、運用監視で漏れないようにする。
	// error 属性は Err が非 nil のときだけ付ける。
	if dev.Outcome == usecase.OutcomeFailed {
		if dev.Err != nil {
			p.logger.Warn("デバイスの処理に失敗しました", append(attrs, "error", dev.Err.Error())...)
		} else {
			p.logger.Warn("デバイスの処理に失敗しました", attrs...)
		}
		return
	}
	p.logger.Info("デバイスを処理しました", attrs...)
}

// tally は Outcome ごとの件数を数える。
func tally(devices []usecase.DeviceResult) map[usecase.Outcome]int {
	counts := make(map[usecase.Outcome]int, 4)
	for _, d := range devices {
		counts[d.Outcome]++
	}
	return counts
}

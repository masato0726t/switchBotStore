// Package presenter はユースケースの実行結果を人間と機械が読める形に整形する。
package presenter

import (
	"log/slog"
	"time"

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
	// 収集の途中で止まった場合でも、そこまでに処理できたデバイスの結果は残る。
	// アカウント単位のエラーより先に出して、取れたぶんを見えるようにする。
	for _, dev := range acc.Devices {
		p.presentDevice(acc.AccountName, dev)
	}

	if acc.RateLimit != nil {
		p.presentRateLimit(acc)
	}

	if acc.Err != nil {
		// レート制限の詳細は presentRateLimit が WARN で出しているため、
		// ここで ERROR を重ねない（重ねると AppError の警報が鳴り続ける）。
		if !acc.RateLimited() {
			p.logger.Error("アカウントの収集に失敗しました",
				append([]any{
					"account", acc.AccountName,
					"error", acc.Err.Error(),
				}, gatewayAttrs(acc.Err)...)...)
		}
		return
	}

	counts := tally(acc.Devices)
	attrs := []any{
		"account", acc.AccountName,
		"total", len(acc.Devices),
		"stored", counts[usecase.OutcomeStored],
		"skipped", counts[usecase.OutcomeSkippedCloudDisabled],
		"registered_only", counts[usecase.OutcomeRegisteredOnly],
		"failed", counts[usecase.OutcomeFailed],
	}
	// 上流が返した残り呼び出し回数を成功時にも残す。これがあると、API を
	// 余分に呼ばずに残量の推移・実際の上限・いつ戻るかを後から追える。
	// ヘッダーが返らなかった回は付けない（空の値でログを埋めないため）。
	if acc.RateLimitRemaining != "" {
		attrs = append(attrs, "rate_limit_remaining", acc.RateLimitRemaining)
	}
	p.logger.Info("アカウントの収集が完了しました", attrs...)
}

// presentRateLimit はレート制限の顛末を出力する。
//
// 制限に当たったこと・待機して見送ったことは WARN、復帰は INFO にする。
// ERROR にしないのは、こちらの不具合ではなく待てば直るものだからで、
// ERROR のままだと AppError の警報が制限の続くあいだ鳴り続けてしまう。
// 一方で WARN が急増したときは AppWarnSpike が安全網として拾う。
func (p *SlogPresenter) presentRateLimit(acc usecase.AccountResult) {
	rl := acc.RateLimit

	attrs := []any{
		"account", acc.AccountName,
		"rate_limit_phase", rl.Phase.String(),
		"consecutive_failures", rl.State.Failures,
	}
	if rl.BlockedFor > 0 {
		attrs = append(attrs,
			"blocked_for", rl.BlockedFor.Round(time.Second).String(),
			"blocked_for_sec", int64(rl.BlockedFor.Seconds()))
	}
	if !rl.State.Since.IsZero() {
		attrs = append(attrs, "blocked_since", rl.State.Since.Format(time.RFC3339))
	}
	if !rl.State.Until.IsZero() {
		attrs = append(attrs,
			"next_attempt_at", rl.State.Until.Format(time.RFC3339),
			"retry_in", rl.RetryIn.Round(time.Second).String(),
			"retry_in_sec", int64(rl.RetryIn.Seconds()))
	}

	switch rl.Phase {
	case usecase.RateLimitPhaseHit:
		// 上流が実際に返した内容（HTTP ステータス・本文・残り回数）を
		// そのままフィールドとして出す。ここが無いと、ログだけを見ても
		// 何が起きているのか判断できない。
		p.logger.Warn("SwitchBot API のレート制限に当たりました。今回の収集を見送ります",
			append(attrs, gatewayAttrs(acc.Err)...)...)
	case usecase.RateLimitPhaseWaiting:
		p.logger.Warn("レート制限の待機中のため、API を呼ばずに今回の収集を見送りました", attrs...)
	case usecase.RateLimitPhaseRecovered:
		p.logger.Info("レート制限から復帰しました。収集を再開します", attrs...)
	}
}

// gatewayAttrs は err に上流 API の応答が含まれていれば、その内訳を返す。
func gatewayAttrs(err error) []any {
	if ge := usecase.GatewayErrorOf(err); ge != nil {
		return ge.LogAttrs()
	}
	return nil
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
			attrs = append(attrs, "error", dev.Err.Error())
			attrs = append(attrs, gatewayAttrs(dev.Err)...)
		}
		p.logger.Warn("デバイスの処理に失敗しました", attrs...)
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

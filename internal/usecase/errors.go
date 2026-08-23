package usecase

import (
	"errors"
	"fmt"
)

// ErrRateLimited は上流 API が呼び出し回数の制限で応答を拒んだことを表す番兵エラー。
//
// 呼び出し側はこのエラーを「こちらの不具合ではなく、待てば直るもの」として扱う。
// CollectReport.FatalError はこのエラーを致命的エラーから除くため、
// レート制限のあいだプロセスは終了コード 0 で終わる。
var ErrRateLimited = errors.New("上流 API のレート制限に当たりました")

// GatewayError は DeviceGateway が上流 API から成功以外の応答を受けたときの詳細。
//
// 「API が実際に何を返したか」をログだけで追えるようにするための型。
// HTTP ステータス・応答本文・レート制限ヘッダーをそのまま保持する。
// 応答の解釈は adapter 側で行い、ここでは値を運ぶだけにしている。
type GatewayError struct {
	// Path は呼び出したパス（例 "/v1.1/devices"）。
	Path string
	// HTTPStatus は HTTP のステータスコード（例 429）。
	HTTPStatus int
	// APIStatusCode は応答 JSON の statusCode。SwitchBot API は成功時に 100 を返す。
	// 応答が封筒の形をしていない（レート制限時など）ときは 0 になる。
	APIStatusCode int
	// APIMessage は応答 JSON の message。
	APIMessage string
	// Body は応答本文。長い応答は adapter 側で切り詰めてから入れる。
	Body string
	// Remaining, Limit, Reset, RetryAfter はレート制限に関する応答ヘッダーの生の値。
	// 返ってこないヘッダーは空文字列になる。
	Remaining  string
	Limit      string
	Reset      string
	RetryAfter string
	// RateLimited は呼び出し回数の制限による拒否かどうか。
	RateLimited bool
}

// Error はエラーメッセージを返す。
//
// HTTP ステータスと応答本文を必ず含める。以前は応答 JSON の statusCode だけを
// 出していたため、封筒の無い応答で "statusCode=0" とだけ表示され、
// 実際に何が返ってきたのかログから分からなかった。
func (e *GatewayError) Error() string {
	return fmt.Sprintf("上流 API がエラーを返しました (path=%s http_status=%d api_status_code=%d): %s",
		e.Path, e.HTTPStatus, e.APIStatusCode, e.summary())
}

// summary は本文かメッセージのうち、内容のあるほうを返す。
func (e *GatewayError) summary() string {
	if e.APIMessage != "" {
		return e.APIMessage
	}
	if e.Body != "" {
		return e.Body
	}
	return "(応答本文なし)"
}

// Unwrap はレート制限による拒否のとき ErrRateLimited を返す。
// これにより errors.Is(err, ErrRateLimited) で判定できる。
func (e *GatewayError) Unwrap() error {
	if e.RateLimited {
		return ErrRateLimited
	}
	return nil
}

// LogAttrs は構造化ログ用のキーと値の並びを返す。
//
// ログ基盤 (VictoriaLogs) では JSON のトップレベルにあるキーだけが
// unpack_json で展開されて検索・通知に使えるため、応答の内訳は
// エラーメッセージに埋め込むのではなく個別のフィールドとして出す。
// 値が空のヘッダーは省いてログ行を短く保つ。
func (e *GatewayError) LogAttrs() []any {
	attrs := []any{
		"path", e.Path,
		"http_status", e.HTTPStatus,
		"api_status_code", e.APIStatusCode,
		"api_message", e.APIMessage,
		"response_body", e.Body,
	}
	for _, h := range []struct {
		key   string
		value string
	}{
		{"rate_limit_remaining", e.Remaining},
		{"rate_limit_limit", e.Limit},
		{"rate_limit_reset", e.Reset},
		{"retry_after", e.RetryAfter},
	} {
		if h.value != "" {
			attrs = append(attrs, h.key, h.value)
		}
	}
	return attrs
}

// GatewayErrorOf は err の連鎖から *GatewayError を取り出す。見つからなければ nil。
func GatewayErrorOf(err error) *GatewayError {
	var ge *GatewayError
	if errors.As(err, &ge) {
		return ge
	}
	return nil
}

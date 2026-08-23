package switchbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// DefaultAPIBase は SwitchBot API のベース URL。
const DefaultAPIBase = "https://api.switch-bot.com"

// defaultTimeout は1リクエストあたりのタイムアウト。
const defaultTimeout = 30 * time.Second

// maxLoggedBodyBytes は失敗時にログへ載せる応答本文の最大バイト数。
//
// 失敗時の本文は短い JSON なので通常は切り詰めが起きない。
// 上流が HTML のエラーページなどを返したときにログ行が膨らむのを防ぐ。
const maxLoggedBodyBytes = 512

// Gateway が出力ポートを満たすことをコンパイル時に検証する。
var _ usecase.DeviceGateway = (*Gateway)(nil)

// Gateway は SwitchBot API v1.1 に対する usecase.DeviceGateway の実装。
//
// 認証情報をメソッド引数で受け取るため、全アカウントで1インスタンスを共有できる。
type Gateway struct {
	httpClient *http.Client
	apiBase    string
	signer     *signer
}

// NewGateway は既定の HTTP クライアントで Gateway を生成する。
func NewGateway() *Gateway {
	return &Gateway{
		httpClient: &http.Client{Timeout: defaultTimeout},
		apiBase:    DefaultAPIBase,
		signer:     newSigner(),
	}
}

// ListDevices はアカウントに登録された全デバイスを返す。
func (g *Gateway) ListDevices(ctx context.Context, cred domain.Credential) ([]domain.Device, error) {
	body, err := g.get(ctx, cred, "/v1.1/devices")
	if err != nil {
		return nil, err
	}

	var list deviceListBody
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("デバイス一覧のパースに失敗しました: %w", err)
	}
	return toDomainDevices(list), nil
}

// FetchStatus は指定デバイスの現在のステータスを生の JSON で返す。
func (g *Gateway) FetchStatus(ctx context.Context, cred domain.Credential, id domain.DeviceID) (domain.StatusPayload, error) {
	body, err := g.get(ctx, cred, "/v1.1/devices/"+string(id)+"/status")
	if err != nil {
		return nil, err
	}
	return domain.StatusPayload(body), nil
}

// get は認証ヘッダーを付けて GET し、レスポンス封筒の body 部分を返す。
//
// 成功以外はすべて *usecase.GatewayError を返す。HTTP ステータス・応答本文・
// レート制限ヘッダーをそのまま持たせてあるので、呼び出し側は「API が実際に
// 何を返したか」をログへ出せる。
func (g *Gateway) get(ctx context.Context, cred domain.Credential, path string) (json.RawMessage, error) {
	headers, err := g.signer.sign(cred.Token, cred.Secret)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase+path, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの生成に失敗しました: %w", err)
	}
	req.Header.Set("Authorization", headers.Authorization)
	req.Header.Set("sign", headers.Sign)
	req.Header.Set("nonce", headers.Nonce)
	req.Header.Set("t", headers.Timestamp)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP リクエストに失敗しました: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("レスポンスの読み込みに失敗しました: %w", err)
	}

	gwErr := newGatewayError(path, resp, raw)

	// 応答が共通の封筒の形をしていないことがある。レート制限のときは
	// {"message":"Too Many Requests"} だけが返り、statusCode を含まない。
	// パースできないときも本文を持ったエラーとして返す。
	var apiResp apiResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, gwErr
	}
	gwErr.APIStatusCode = apiResp.StatusCode
	if apiResp.Message != "" {
		gwErr.APIMessage = apiResp.Message
	}

	if resp.StatusCode != http.StatusOK || apiResp.StatusCode != statusCodeSuccess {
		return nil, gwErr
	}
	return apiResp.Body, nil
}

// newGatewayError は応答から詳細つきのエラーを組み立てる。
//
// レート制限の判定は HTTP 429 だけで行う。応答 JSON の statusCode=190 は
// レート制限にも使われていた時期があるが、現在はデバイスの内部エラーや
// オフラインでも返るため、これをレート制限とみなすとデバイスの故障を
// 「待てば直るもの」として黙って見送ってしまう。
func newGatewayError(path string, resp *http.Response, raw []byte) *usecase.GatewayError {
	return &usecase.GatewayError{
		Path:        path,
		HTTPStatus:  resp.StatusCode,
		Body:        truncate(string(raw), maxLoggedBodyBytes),
		Remaining:   resp.Header.Get("X-RateLimit-Remaining"),
		Limit:       resp.Header.Get("X-RateLimit-Limit"),
		Reset:       resp.Header.Get("X-RateLimit-Reset"),
		RetryAfter:  resp.Header.Get("Retry-After"),
		RateLimited: resp.StatusCode == http.StatusTooManyRequests,
	}
}

// truncate は s を最大 max バイトに切り詰める。切り詰めたことが分かる印を付ける。
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(以下省略)"
}

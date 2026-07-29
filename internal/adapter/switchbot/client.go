package switchbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// DefaultAPIBase は SwitchBot API のベース URL。
const DefaultAPIBase = "https://api.switch-bot.com"

// defaultTimeout は1リクエストあたりのタイムアウト。
const defaultTimeout = 30 * time.Second

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

	var apiResp apiResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("レスポンスのパースに失敗しました (body=%s): %w", string(raw), err)
	}
	if apiResp.StatusCode != statusCodeSuccess {
		return nil, fmt.Errorf("SwitchBot API がエラーを返しました (statusCode=%d): %s",
			apiResp.StatusCode, apiResp.Message)
	}
	return apiResp.Body, nil
}

package switchbot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const defaultAPIBase = "https://api.switch-bot.com"

// Client はSwitchBot APIクライアントのインターフェース
type Client interface {
	AccountName() string
	AccountToken() string
	AccountSecret() string
	GetDevices() (*DeviceList, error)
	GetDeviceStatus(deviceID string) (json.RawMessage, error)
}

// DeviceList はデバイス一覧APIのレスポンス body
type DeviceList struct {
	DeviceList         []DeviceInfo `json:"deviceList"`
	InfraredRemoteList []DeviceInfo `json:"infraredRemoteList"`
}

// DeviceInfo はデバイス個別の情報
type DeviceInfo struct {
	DeviceID           string `json:"deviceId"`
	DeviceName         string `json:"deviceName"`
	DeviceType         string `json:"deviceType"`
	HubDeviceID        string `json:"hubDeviceId"`
	EnableCloudService bool   `json:"enableCloudService"`
}

type apiResponse struct {
	StatusCode int             `json:"statusCode"`
	Message    string          `json:"message"`
	Body       json.RawMessage `json:"body"`
}

// SwitchBotClient は Client インターフェースの HTTP実装
type SwitchBotClient struct {
	name    string
	token   string
	secret  string
	httpCli *http.Client
	apiBase string
	nowFunc func() time.Time
}

func NewClient(token, secret, name string) *SwitchBotClient {
	return &SwitchBotClient{
		name:    name,
		token:   token,
		secret:  secret,
		httpCli: &http.Client{Timeout: 30 * time.Second},
		apiBase: defaultAPIBase,
		nowFunc: time.Now,
	}
}

func (c *SwitchBotClient) AccountName() string   { return c.name }
func (c *SwitchBotClient) AccountToken() string  { return c.token }
func (c *SwitchBotClient) AccountSecret() string { return c.secret }

// buildHeaders は nonce をランダム生成して認証ヘッダーを返す
func (c *SwitchBotClient) buildHeaders() (map[string]string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("nonce生成失敗: %v", err)
	}
	nonce := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return c.buildHeadersWithNonce(nonce)
}

// buildHeadersWithNonce は nonce を指定して認証ヘッダーを生成する（テスト用）
// 署名: HMAC-SHA256(token + timestamp_ms + nonce, secret) → Base64
func (c *SwitchBotClient) buildHeadersWithNonce(nonce string) (map[string]string, error) {
	t := strconv.FormatInt(c.nowFunc().UnixMilli(), 10)

	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write([]byte(c.token + t + nonce))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return map[string]string{
		"Authorization": c.token,
		"sign":          sign,
		"nonce":         nonce,
		"t":             t,
		"Content-Type":  "application/json",
	}, nil
}

func (c *SwitchBotClient) get(path string) (json.RawMessage, error) {
	headers, err := c.buildHeaders()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", c.apiBase+path, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTPリクエスト失敗: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("レスポンス読み込み失敗: %v", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("レスポンスのパース失敗: %v (body=%s)", err, string(raw))
	}

	if apiResp.StatusCode != 100 {
		return nil, fmt.Errorf("APIエラー (statusCode=%d): %s", apiResp.StatusCode, apiResp.Message)
	}

	return apiResp.Body, nil
}

// GetDevices はアカウントに登録されている全デバイスの一覧を返す
func (c *SwitchBotClient) GetDevices() (*DeviceList, error) {
	body, err := c.get("/v1.1/devices")
	if err != nil {
		return nil, err
	}

	var list DeviceList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("デバイス一覧のパース失敗: %v", err)
	}
	return &list, nil
}

// GetDeviceStatus は指定デバイスの現在のステータスを JSON で返す
func (c *SwitchBotClient) GetDeviceStatus(deviceID string) (json.RawMessage, error) {
	return c.get("/v1.1/devices/" + deviceID + "/status")
}

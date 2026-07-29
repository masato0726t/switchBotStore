package switchbot

import "encoding/json"

// statusCodeSuccess は SwitchBot API が成功時に返す statusCode。
const statusCodeSuccess = 100

// apiResponse は SwitchBot API 共通のレスポンス封筒。
type apiResponse struct {
	StatusCode int             `json:"statusCode"`
	Message    string          `json:"message"`
	Body       json.RawMessage `json:"body"`
}

// deviceListBody は GET /v1.1/devices のレスポンス body。
type deviceListBody struct {
	DeviceList         []deviceInfo `json:"deviceList"`
	InfraredRemoteList []deviceInfo `json:"infraredRemoteList"`
}

// deviceInfo は API が返すデバイス1件の情報。
//
// SwitchBot API は deviceList の要素に deviceType を、infraredRemoteList の
// 要素に remoteType を返す（名前が違うだけで、どちらもデバイスの種別）。
// 両方を受けて mapper が種別へ畳み込む。
type deviceInfo struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	DeviceType string `json:"deviceType"`

	// RemoteType は赤外線リモコンの種別（"Air Conditioner" / "TV" / "Light" など）。
	// 物理デバイスでは空になる。
	RemoteType string `json:"remoteType"`

	HubDeviceID        string `json:"hubDeviceId"`
	EnableCloudService bool   `json:"enableCloudService"`
}

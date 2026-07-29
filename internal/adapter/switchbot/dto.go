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
// 注意: SwitchBot API は infraredRemoteList の要素に deviceType ではなく
// remoteType を返すため、赤外線リモコンの DeviceType は空文字になる。
// これは現行実装から引き継いだ挙動であり、本リファクタリングでは変更しない。
type deviceInfo struct {
	DeviceID           string `json:"deviceId"`
	DeviceName         string `json:"deviceName"`
	DeviceType         string `json:"deviceType"`
	HubDeviceID        string `json:"hubDeviceId"`
	EnableCloudService bool   `json:"enableCloudService"`
}

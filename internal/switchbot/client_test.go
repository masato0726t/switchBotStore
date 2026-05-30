package switchbot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func newTestClient(t *testing.T, token, secret, name string) *SwitchBotClient {
	t.Helper()
	return NewClient(token, secret, name)
}

func TestAccountName(t *testing.T) {
	c := newTestClient(t, "tok", "sec", "myAccount")
	if c.AccountName() != "myAccount" {
		t.Errorf("AccountName() = %q, want %q", c.AccountName(), "myAccount")
	}
}

func TestAccountToken(t *testing.T) {
	c := newTestClient(t, "mytoken", "sec", "acc")
	if c.AccountToken() != "mytoken" {
		t.Errorf("AccountToken() = %q, want %q", c.AccountToken(), "mytoken")
	}
}

func TestBuildHeadersWithNonce_RequiredFields(t *testing.T) {
	c := newTestClient(t, "mytoken", "mysecret", "test")
	c.nowFunc = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

	headers, err := c.buildHeadersWithNonce("test-nonce-123")
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}

	for _, key := range []string{"Authorization", "sign", "nonce", "t", "Content-Type"} {
		if headers[key] == "" {
			t.Errorf("ヘッダー %q が空です", key)
		}
	}
}

func TestBuildHeadersWithNonce_Authorization(t *testing.T) {
	c := newTestClient(t, "mytoken", "mysecret", "test")
	c.nowFunc = func() time.Time { return time.Unix(0, 0) }

	headers, err := c.buildHeadersWithNonce("nonce")
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if headers["Authorization"] != "mytoken" {
		t.Errorf("Authorization = %q, want %q", headers["Authorization"], "mytoken")
	}
}

func TestBuildHeadersWithNonce_Timestamp(t *testing.T) {
	c := newTestClient(t, "tok", "sec", "test")
	fixedTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	c.nowFunc = func() time.Time { return fixedTime }

	headers, err := c.buildHeadersWithNonce("nonce")
	if err != nil {
		t.Fatal(err)
	}

	expectedT := strconv.FormatInt(fixedTime.UnixMilli(), 10)
	if headers["t"] != expectedT {
		t.Errorf("t = %q, want %q", headers["t"], expectedT)
	}
}

func TestBuildHeadersWithNonce_SignatureIsValid(t *testing.T) {
	token, secret, nonce := "mytoken", "mysecret", "fixed-nonce"
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	c := newTestClient(t, token, secret, "test")
	c.nowFunc = func() time.Time { return fixedTime }

	headers, err := c.buildHeadersWithNonce(nonce)
	if err != nil {
		t.Fatal(err)
	}

	ts := strconv.FormatInt(fixedTime.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token + ts + nonce))
	expectedSign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if headers["sign"] != expectedSign {
		t.Errorf("sign = %q, want %q", headers["sign"], expectedSign)
	}
}

func TestGetDevices_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.1/devices" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"statusCode": 100,
			"message": "success",
			"body": {
				"deviceList": [
					{"deviceId":"d1","deviceName":"温湿度計","deviceType":"Meter","enableCloudService":true},
					{"deviceId":"d2","deviceName":"スマートプラグ","deviceType":"Plug Mini (US)","enableCloudService":true}
				],
				"infraredRemoteList": [
					{"deviceId":"ir1","deviceName":"エアコン","deviceType":"Air Conditioner"}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok", "sec", "test")
	c.apiBase = srv.URL

	list, err := c.GetDevices()
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if len(list.DeviceList) != 2 {
		t.Errorf("len(DeviceList) = %d, want 2", len(list.DeviceList))
	}
	if list.DeviceList[0].DeviceID != "d1" {
		t.Errorf("DeviceList[0].DeviceID = %q, want %q", list.DeviceList[0].DeviceID, "d1")
	}
	if len(list.InfraredRemoteList) != 1 {
		t.Errorf("len(InfraredRemoteList) = %d, want 1", len(list.InfraredRemoteList))
	}
}

func TestGetDevices_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"statusCode": 401, "message": "Unauthorized"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok", "sec", "test")
	c.apiBase = srv.URL

	_, err := c.GetDevices()
	if err == nil {
		t.Error("APIエラーの場合はエラーを返す想定")
	}
}

func TestGetDevices_NetworkError(t *testing.T) {
	c := newTestClient(t, "tok", "sec", "test")
	c.apiBase = "http://127.0.0.1:1"

	_, err := c.GetDevices()
	if err == nil {
		t.Error("ネットワークエラーの場合はエラーを返す想定")
	}
}

func TestGetDeviceStatus_Success(t *testing.T) {
	statusBody := `{"deviceId":"d1","deviceType":"Meter","temperature":25.5,"humidity":60}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.1/devices/d1/status" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"statusCode":100,"message":"success","body":` + statusBody + `}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok", "sec", "test")
	c.apiBase = srv.URL

	raw, err := c.GetDeviceStatus("d1")
	if err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("JSONパース失敗: %v", err)
	}
	if body["deviceId"] != "d1" {
		t.Errorf("deviceId = %v, want %q", body["deviceId"], "d1")
	}
}

func TestGetDeviceStatus_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"statusCode":190,"message":"Device Not Found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok", "sec", "test")
	c.apiBase = srv.URL

	_, err := c.GetDeviceStatus("unknown")
	if err == nil {
		t.Error("APIエラーの場合はエラーを返す想定")
	}
}

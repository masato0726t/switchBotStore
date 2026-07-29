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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, token, secret, name string) *SwitchBotClient {
	t.Helper()
	return NewClient(token, secret, name)
}

func TestAccountName(t *testing.T) {
	c := newTestClient(t, "tok", "sec", "myAccount")
	assert.Equal(t, "myAccount", c.AccountName())
}

func TestAccountToken(t *testing.T) {
	c := newTestClient(t, "mytoken", "sec", "acc")
	assert.Equal(t, "mytoken", c.AccountToken())
}

func TestBuildHeadersWithNonce_RequiredFields(t *testing.T) {
	c := newTestClient(t, "mytoken", "mysecret", "test")
	c.nowFunc = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

	headers, err := c.buildHeadersWithNonce("test-nonce-123")
	require.NoError(t, err)

	for _, key := range []string{"Authorization", "sign", "nonce", "t", "Content-Type"} {
		assert.NotEmpty(t, headers[key], "ヘッダー %q が空です", key)
	}
}

func TestBuildHeadersWithNonce_Authorization(t *testing.T) {
	c := newTestClient(t, "mytoken", "mysecret", "test")
	c.nowFunc = func() time.Time { return time.Unix(0, 0) }

	headers, err := c.buildHeadersWithNonce("nonce")
	require.NoError(t, err)
	assert.Equal(t, "mytoken", headers["Authorization"])
}

func TestBuildHeadersWithNonce_Timestamp(t *testing.T) {
	c := newTestClient(t, "tok", "sec", "test")
	fixedTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	c.nowFunc = func() time.Time { return fixedTime }

	headers, err := c.buildHeadersWithNonce("nonce")
	require.NoError(t, err)

	expectedT := strconv.FormatInt(fixedTime.UnixMilli(), 10)
	assert.Equal(t, expectedT, headers["t"])
}

func TestBuildHeadersWithNonce_SignatureIsValid(t *testing.T) {
	token, secret, nonce := "mytoken", "mysecret", "fixed-nonce"
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	c := newTestClient(t, token, secret, "test")
	c.nowFunc = func() time.Time { return fixedTime }

	headers, err := c.buildHeadersWithNonce(nonce)
	require.NoError(t, err)

	ts := strconv.FormatInt(fixedTime.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token + ts + nonce))
	expectedSign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	assert.Equal(t, expectedSign, headers["sign"])
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
	require.NoError(t, err)
	require.Len(t, list.DeviceList, 2)
	assert.Equal(t, "d1", list.DeviceList[0].DeviceID)
	assert.Len(t, list.InfraredRemoteList, 1)
}

func TestGetDevices_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"statusCode": 401, "message": "Unauthorized"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok", "sec", "test")
	c.apiBase = srv.URL

	_, err := c.GetDevices()
	require.Error(t, err)
}

func TestGetDevices_NetworkError(t *testing.T) {
	c := newTestClient(t, "tok", "sec", "test")
	c.apiBase = "http://127.0.0.1:1"

	_, err := c.GetDevices()
	require.Error(t, err)
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
	require.NoError(t, err)

	var body map[string]interface{}
	err = json.Unmarshal(raw, &body)
	require.NoError(t, err)
	assert.Equal(t, "d1", body["deviceId"])
}

func TestGetDeviceStatus_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"statusCode":190,"message":"Device Not Found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, "tok", "sec", "test")
	c.apiBase = srv.URL

	_, err := c.GetDeviceStatus("unknown")
	require.Error(t, err)
}

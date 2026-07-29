package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/domain"
)

var testCred = domain.Credential{Token: "tok", Secret: "sec"}

// newTestGateway は httptest サーバーを向く Gateway を返す。
func newTestGateway(t *testing.T, handler http.HandlerFunc) *Gateway {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	g := NewGateway()
	g.apiBase = srv.URL
	return g
}

func TestListDevices_物理と赤外線をまとめて返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1.1/devices", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("sign"))
		assert.NotEmpty(t, r.Header.Get("nonce"))
		assert.NotEmpty(t, r.Header.Get("t"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"statusCode": 100,
			"message": "success",
			"body": {
				"deviceList": [
					{"deviceId":"d1","deviceName":"温湿度計","deviceType":"Meter","enableCloudService":true},
					{"deviceId":"d2","deviceName":"スマートプラグ","deviceType":"Plug Mini (US)","enableCloudService":true}
				],
				"infraredRemoteList": [
					{"deviceId":"ir1","deviceName":"エアコン"}
				]
			}
		}`))
	})

	devices, err := g.ListDevices(context.Background(), testCred)

	require.NoError(t, err)
	require.Len(t, devices, 3)
	assert.Equal(t, domain.DeviceID("d1"), devices[0].ID)
	assert.Equal(t, domain.DeviceKindPhysical, devices[0].Kind)
	assert.Equal(t, domain.DeviceKindInfraredRemote, devices[2].Kind)
}

func TestListDevices_APIエラーを返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"statusCode": 401, "message": "Unauthorized"}`))
	})

	_, err := g.ListDevices(context.Background(), testCred)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestListDevices_ネットワークエラーを返す(t *testing.T) {
	g := NewGateway()
	g.apiBase = "http://127.0.0.1:1"

	_, err := g.ListDevices(context.Background(), testCred)
	require.Error(t, err)
}

func TestListDevices_パースできないレスポンスでエラーを返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`これはJSONではない`))
	})

	_, err := g.ListDevices(context.Background(), testCred)
	require.Error(t, err)
}

func TestFetchStatus_生のJSONを返す(t *testing.T) {
	const statusBody = `{"deviceId":"d1","deviceType":"Meter","temperature":25.5,"humidity":60}`

	g := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1.1/devices/d1/status", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"statusCode":100,"message":"success","body":` + statusBody + `}`))
	})

	payload, err := g.FetchStatus(context.Background(), testCred, "d1")

	require.NoError(t, err)
	assert.JSONEq(t, statusBody, string(payload))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.Equal(t, "d1", decoded["deviceId"])
}

func TestFetchStatus_APIエラーを返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"statusCode":190,"message":"Device Not Found"}`))
	})

	_, err := g.FetchStatus(context.Background(), testCred, "unknown")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Device Not Found")
}

func TestFetchStatus_キャンセル済みcontextでエラーを返す(t *testing.T) {
	g := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"statusCode":100,"body":{}}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := g.FetchStatus(ctx, testCred, "d1")
	require.Error(t, err)
}

package switchbot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/domain"
)

func TestToDomainDevices_物理と赤外線を単一リストに畳み込む(t *testing.T) {
	body := deviceListBody{
		DeviceList: []deviceInfo{
			{DeviceID: "d1", DeviceName: "温湿度計", DeviceType: "Meter", HubDeviceID: "hub1", EnableCloudService: true},
		},
		// API は赤外線リモコンの種別を deviceType ではなく remoteType で返す。
		InfraredRemoteList: []deviceInfo{
			{DeviceID: "ir1", DeviceName: "エアコン", RemoteType: "Air Conditioner"},
		},
	}

	got := toDomainDevices(body)

	require.Len(t, got, 2)

	assert.Equal(t, domain.Device{
		ID:                  "d1",
		Name:                "温湿度計",
		Type:                "Meter",
		HubID:               "hub1",
		Kind:                domain.DeviceKindPhysical,
		CloudServiceEnabled: true,
	}, got[0])

	assert.Equal(t, domain.DeviceKindInfraredRemote, got[1].Kind)
	assert.Equal(t, domain.DeviceID("ir1"), got[1].ID)
	assert.False(t, got[1].StatusReadable(), "赤外線リモコンはステータスを取得できない")
}

func TestToDomainDevices_赤外線リモコンの種別をremoteTypeから取る(t *testing.T) {
	// deviceType だけを見ていると種別が空のまま保存され、種別で絞り込む側
	// （エアコン自動制御など）が対象を見つけられなくなる。
	body := deviceListBody{
		InfraredRemoteList: []deviceInfo{
			{DeviceID: "ir1", DeviceName: "エアコン", RemoteType: "Air Conditioner"},
			{DeviceID: "ir2", DeviceName: "ライト", RemoteType: "Light"},
		},
	}

	got := toDomainDevices(body)

	require.Len(t, got, 2)
	assert.Equal(t, "Air Conditioner", got[0].Type)
	assert.Equal(t, "Light", got[1].Type)
}

func TestResolveType(t *testing.T) {
	tests := []struct {
		name string
		info deviceInfo
		want string
	}{
		{"物理デバイスは deviceType", deviceInfo{DeviceType: "Meter"}, "Meter"},
		{"赤外線リモコンは remoteType", deviceInfo{RemoteType: "Air Conditioner"}, "Air Conditioner"},
		{"両方あれば deviceType を優先", deviceInfo{DeviceType: "Meter", RemoteType: "TV"}, "Meter"},
		{"どちらも無ければ空", deviceInfo{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveType(tt.info))
		})
	}
}

func TestToDomainDevices_空のレスポンスで空スライスを返す(t *testing.T) {
	got := toDomainDevices(deviceListBody{})
	assert.Empty(t, got)
}

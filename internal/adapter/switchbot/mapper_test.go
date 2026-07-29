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
		InfraredRemoteList: []deviceInfo{
			{DeviceID: "ir1", DeviceName: "エアコン", DeviceType: "Air Conditioner"},
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

func TestToDomainDevices_空のレスポンスで空スライスを返す(t *testing.T) {
	got := toDomainDevices(deviceListBody{})
	assert.Empty(t, got)
}

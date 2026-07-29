package persistence

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"switchBotStore/internal/domain"
)

func TestToAccountModel(t *testing.T) {
	got := toAccountModel(domain.Account{
		Name:       "メインアカウント",
		Credential: domain.Credential{Token: "tok", Secret: "sec"},
	})

	assert.Equal(t, "メインアカウント", got.Name)
	assert.Equal(t, "tok", got.Token)
	assert.Equal(t, "sec", got.Secret)
}

func TestToDeviceModel_物理デバイス(t *testing.T) {
	got := toDeviceModel(domain.AccountID(7), domain.Device{
		ID:                  "d1",
		Name:                "温湿度計",
		Type:                "Meter",
		HubID:               "hub1",
		Kind:                domain.DeviceKindPhysical,
		CloudServiceEnabled: true,
	})

	assert.Equal(t, int64(7), got.APIAccountID)
	assert.Equal(t, "d1", got.DeviceID)
	assert.Equal(t, "温湿度計", got.DeviceName)
	assert.Equal(t, "Meter", got.DeviceType)
	assert.Equal(t, "hub1", got.HubDeviceID)
	assert.True(t, got.EnableCloudService)
	assert.False(t, got.IsVirtualInfrared)
}

func TestToDeviceModel_赤外線リモコンはフラグが立つ(t *testing.T) {
	got := toDeviceModel(domain.AccountID(1), domain.Device{
		ID:   "ir1",
		Kind: domain.DeviceKindInfraredRemote,
	})

	assert.True(t, got.IsVirtualInfrared)
}

func TestToStatusLogModel(t *testing.T) {
	at := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	got := toStatusLogModel(domain.DeviceRecordID(42), domain.StatusSnapshot{
		Payload:    []byte(`{"temperature":25.5}`),
		RecordedAt: at,
	})

	assert.Equal(t, int64(42), got.DeviceID)
	assert.JSONEq(t, `{"temperature":25.5}`, got.StatusData)
	assert.Equal(t, at, got.RecordedAt)
}

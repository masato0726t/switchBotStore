package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"switchBotStore/internal/domain"
)

func TestDevice_StatusReadable(t *testing.T) {
	tests := []struct {
		name string
		dev  domain.Device
		want bool
	}{
		{
			name: "物理デバイスでクラウド有効なら取得できる",
			dev:  domain.Device{Kind: domain.DeviceKindPhysical, CloudServiceEnabled: true},
			want: true,
		},
		{
			name: "物理デバイスでもクラウド無効なら取得できない",
			dev:  domain.Device{Kind: domain.DeviceKindPhysical, CloudServiceEnabled: false},
			want: false,
		},
		{
			name: "赤外線リモコンはクラウド有効でも取得できない",
			dev:  domain.Device{Kind: domain.DeviceKindInfraredRemote, CloudServiceEnabled: true},
			want: false,
		},
		{
			name: "赤外線リモコンでクラウド無効なら取得できない",
			dev:  domain.Device{Kind: domain.DeviceKindInfraredRemote, CloudServiceEnabled: false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.dev.StatusReadable())
		})
	}
}

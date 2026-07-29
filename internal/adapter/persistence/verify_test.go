package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectedSchema_モデルからテーブル名とカラム名を導出する(t *testing.T) {
	tests := []struct {
		model       any
		wantTable   string
		wantColumns []string
	}{
		{
			model:     apiAccountModel{},
			wantTable: "api_accounts",
			wantColumns: []string{
				"id", "name", "token", "secret", "created_at", "updated_at",
			},
		},
		{
			model:     deviceModel{},
			wantTable: "devices",
			wantColumns: []string{
				"id", "api_account_id", "device_id", "device_name", "device_type",
				"hub_device_id", "enable_cloud_service", "is_virtual_infrared",
				"created_at", "updated_at",
			},
		},
		{
			model:     deviceStatusLogModel{},
			wantTable: "device_status_logs",
			wantColumns: []string{
				"id", "device_id", "status_data", "recorded_at", "created_at",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.wantTable, func(t *testing.T) {
			table, columns, err := expectedSchema(tt.model)

			require.NoError(t, err)
			assert.Equal(t, tt.wantTable, table)
			assert.ElementsMatch(t, tt.wantColumns, columns)
		})
	}
}

func TestAllModels_全モデルを列挙している(t *testing.T) {
	assert.Len(t, allModels(), 3)
}

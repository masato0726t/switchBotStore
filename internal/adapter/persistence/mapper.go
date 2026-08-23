package persistence

import (
	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

func toAccountModel(a domain.Account) apiAccountModel {
	return apiAccountModel{
		Name:   a.Name,
		Token:  a.Credential.Token,
		Secret: a.Credential.Secret,
	}
}

func toDeviceModel(accountID domain.AccountID, d domain.Device) deviceModel {
	return deviceModel{
		APIAccountID:       int64(accountID),
		DeviceID:           string(d.ID),
		DeviceName:         d.Name,
		DeviceType:         d.Type,
		HubDeviceID:        string(d.HubID),
		EnableCloudService: d.CloudServiceEnabled,
		IsVirtualInfrared:  d.Kind == domain.DeviceKindInfraredRemote,
	}
}

// toDeviceRecord は devices の1行を、ID 付きのドメインモデルへ戻す。
// SwitchBot API を叩かずに一覧を再構成するために使う。
func toDeviceRecord(m deviceModel) usecase.DeviceRecord {
	kind := domain.DeviceKindPhysical
	if m.IsVirtualInfrared {
		kind = domain.DeviceKindInfraredRemote
	}
	return usecase.DeviceRecord{
		ID: domain.DeviceRecordID(m.ID),
		Device: domain.Device{
			ID:                  domain.DeviceID(m.DeviceID),
			Name:                m.DeviceName,
			Type:                m.DeviceType,
			HubID:               domain.DeviceID(m.HubDeviceID),
			Kind:                kind,
			CloudServiceEnabled: m.EnableCloudService,
		},
	}
}

func toStatusLogModel(id domain.DeviceRecordID, s domain.StatusSnapshot) deviceStatusLogModel {
	return deviceStatusLogModel{
		DeviceID:   int64(id),
		StatusData: string(s.Payload),
		RecordedAt: s.RecordedAt,
	}
}

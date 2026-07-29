package persistence

import "switchBotStore/internal/domain"

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

func toStatusLogModel(id domain.DeviceRecordID, s domain.StatusSnapshot) deviceStatusLogModel {
	return deviceStatusLogModel{
		DeviceID:   int64(id),
		StatusData: string(s.Payload),
		RecordedAt: s.RecordedAt,
	}
}

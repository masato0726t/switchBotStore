package collector

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"switchBotStore/internal/switchbot"
)

// ---------- mock: switchbot.Client ----------

type mockClient struct {
	name       string
	token      string
	secret     string
	devicesRes *switchbot.DeviceList
	devicesErr error
	statusRes  map[string]json.RawMessage
	statusErr  map[string]error
}

func (m *mockClient) AccountName() string   { return m.name }
func (m *mockClient) AccountToken() string  { return m.token }
func (m *mockClient) AccountSecret() string { return m.secret }

func (m *mockClient) GetDevices() (*switchbot.DeviceList, error) {
	return m.devicesRes, m.devicesErr
}

func (m *mockClient) GetDeviceStatus(deviceID string) (json.RawMessage, error) {
	if err, ok := m.statusErr[deviceID]; ok {
		return nil, err
	}
	return m.statusRes[deviceID], nil
}

// ---------- mock: repository.Repository ----------

type logEntry struct {
	deviceID   int64
	status     json.RawMessage
	recordedAt time.Time
}

type mockRepo struct {
	accounts   map[string]int64
	devices    map[string]int64
	logs       []logEntry
	accountErr error
	deviceErr  error
	logErr     error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		accounts: make(map[string]int64),
		devices:  make(map[string]int64),
	}
}

func (r *mockRepo) UpsertAccount(name, token, secret string) (int64, error) {
	if r.accountErr != nil {
		return 0, r.accountErr
	}
	if id, ok := r.accounts[token]; ok {
		return id, nil
	}
	id := int64(len(r.accounts) + 1)
	r.accounts[token] = id
	return id, nil
}

func (r *mockRepo) UpsertDevice(accountID int64, dev switchbot.DeviceInfo, isVirtualInfrared bool) (int64, error) {
	if r.deviceErr != nil {
		return 0, r.deviceErr
	}
	key := fmt.Sprintf("%d/%s", accountID, dev.DeviceID)
	if id, ok := r.devices[key]; ok {
		return id, nil
	}
	id := int64(len(r.devices) + 1)
	r.devices[key] = id
	return id, nil
}

func (r *mockRepo) SaveStatusLog(deviceID int64, status json.RawMessage, recordedAt time.Time) error {
	if r.logErr != nil {
		return r.logErr
	}
	r.logs = append(r.logs, logEntry{deviceID: deviceID, status: status, recordedAt: recordedAt})
	return nil
}

// ---------- ヘルパー ----------

func makeStatus(deviceID string) json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"deviceId": deviceID})
	return raw
}

func makeClient(name, token string, devices []switchbot.DeviceInfo) *mockClient {
	statusRes := make(map[string]json.RawMessage)
	for _, d := range devices {
		statusRes[d.DeviceID] = makeStatus(d.DeviceID)
	}
	return &mockClient{
		name:      name,
		token:     token,
		devicesRes: &switchbot.DeviceList{DeviceList: devices},
		statusRes: statusRes,
		statusErr: make(map[string]error),
	}
}

// ---------- テスト ----------

func TestCollect_NormalFlow(t *testing.T) {
	repo := newMockRepo()
	client := makeClient("acc1", "tok1", []switchbot.DeviceInfo{
		{DeviceID: "d1", DeviceName: "温湿度計", DeviceType: "Meter", EnableCloudService: true},
	})

	c := New(repo, []switchbot.Client{client})
	if err := c.Collect(); err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if len(repo.logs) != 1 {
		t.Errorf("保存されたログ数 = %d, want 1", len(repo.logs))
	}
}

func TestCollect_SkipsDisabledCloud(t *testing.T) {
	repo := newMockRepo()
	client := &mockClient{
		name:  "acc1",
		token: "tok1",
		devicesRes: &switchbot.DeviceList{
			DeviceList: []switchbot.DeviceInfo{
				{DeviceID: "d1", DeviceName: "Bot", EnableCloudService: false},
				{DeviceID: "d2", DeviceName: "Meter", EnableCloudService: true},
			},
		},
		statusRes: map[string]json.RawMessage{"d2": makeStatus("d2")},
		statusErr: make(map[string]error),
	}

	c := New(repo, []switchbot.Client{client})
	if err := c.Collect(); err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if len(repo.logs) != 1 {
		t.Errorf("保存されたログ数 = %d, want 1 (クラウド無効デバイスはスキップ)", len(repo.logs))
	}
	if len(repo.devices) != 2 {
		t.Errorf("登録デバイス数 = %d, want 2", len(repo.devices))
	}
}

func TestCollect_InfraredOnlyRegistered(t *testing.T) {
	repo := newMockRepo()
	client := &mockClient{
		name:  "acc1",
		token: "tok1",
		devicesRes: &switchbot.DeviceList{
			InfraredRemoteList: []switchbot.DeviceInfo{
				{DeviceID: "ir1", DeviceName: "エアコン", DeviceType: "Air Conditioner"},
			},
		},
		statusRes: make(map[string]json.RawMessage),
		statusErr: make(map[string]error),
	}

	c := New(repo, []switchbot.Client{client})
	if err := c.Collect(); err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if len(repo.devices) != 1 {
		t.Errorf("登録デバイス数 = %d, want 1", len(repo.devices))
	}
	if len(repo.logs) != 0 {
		t.Errorf("ログ数 = %d, want 0 (赤外線リモコンはログ保存しない)", len(repo.logs))
	}
}

func TestCollect_MultipleAccounts(t *testing.T) {
	repo := newMockRepo()
	clients := []switchbot.Client{
		makeClient("acc1", "tok1", []switchbot.DeviceInfo{{DeviceID: "d1", EnableCloudService: true}}),
		makeClient("acc2", "tok2", []switchbot.DeviceInfo{{DeviceID: "d2", EnableCloudService: true}}),
	}

	c := New(repo, clients)
	if err := c.Collect(); err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if len(repo.accounts) != 2 {
		t.Errorf("登録アカウント数 = %d, want 2", len(repo.accounts))
	}
	if len(repo.logs) != 2 {
		t.Errorf("保存ログ数 = %d, want 2", len(repo.logs))
	}
}

func TestCollect_GetDevicesError(t *testing.T) {
	repo := newMockRepo()
	client := &mockClient{
		name:       "acc1",
		token:      "tok1",
		devicesErr: errors.New("API error"),
		statusRes:  make(map[string]json.RawMessage),
		statusErr:  make(map[string]error),
	}

	c := New(repo, []switchbot.Client{client})
	if err := c.Collect(); err == nil {
		t.Error("GetDevices エラー時はエラーを返す想定")
	}
}

func TestCollect_StatusErrorContinues(t *testing.T) {
	repo := newMockRepo()
	client := &mockClient{
		name:  "acc1",
		token: "tok1",
		devicesRes: &switchbot.DeviceList{
			DeviceList: []switchbot.DeviceInfo{
				{DeviceID: "d1", DeviceName: "失敗デバイス", EnableCloudService: true},
				{DeviceID: "d2", DeviceName: "成功デバイス", EnableCloudService: true},
			},
		},
		statusRes: map[string]json.RawMessage{"d2": makeStatus("d2")},
		statusErr: map[string]error{"d1": errors.New("デバイスオフライン")},
	}

	c := New(repo, []switchbot.Client{client})
	if err := c.Collect(); err != nil {
		t.Fatalf("部分エラーでは全体エラーにならない想定: %v", err)
	}
	if len(repo.logs) != 1 {
		t.Errorf("保存ログ数 = %d, want 1", len(repo.logs))
	}
}

func TestCollect_AccountUpsertError(t *testing.T) {
	repo := newMockRepo()
	repo.accountErr = errors.New("DB error")
	client := makeClient("acc1", "tok1", []switchbot.DeviceInfo{{DeviceID: "d1", EnableCloudService: true}})

	c := New(repo, []switchbot.Client{client})
	if err := c.Collect(); err == nil {
		t.Error("アカウント登録失敗時はエラーを返す想定")
	}
}

func TestCollect_SaveLogError(t *testing.T) {
	repo := newMockRepo()
	repo.logErr = errors.New("DB write error")
	client := &mockClient{
		name:  "acc1",
		token: "tok1",
		devicesRes: &switchbot.DeviceList{
			DeviceList: []switchbot.DeviceInfo{
				{DeviceID: "d1", EnableCloudService: true},
				{DeviceID: "d2", EnableCloudService: true},
			},
		},
		statusRes: map[string]json.RawMessage{
			"d1": makeStatus("d1"),
			"d2": makeStatus("d2"),
		},
		statusErr: make(map[string]error),
	}

	c := New(repo, []switchbot.Client{client})
	if err := c.Collect(); err != nil {
		t.Fatalf("SaveStatusLog エラーは全体エラーにならない想定: %v", err)
	}
	if len(repo.logs) != 0 {
		t.Errorf("保存ログ数 = %d, want 0 (全てエラー)", len(repo.logs))
	}
}

func TestInitialCollect_CallsCollect(t *testing.T) {
	repo := newMockRepo()
	client := makeClient("acc1", "tok1", []switchbot.DeviceInfo{
		{DeviceID: "d1", EnableCloudService: true},
	})

	c := New(repo, []switchbot.Client{client})
	if err := c.InitialCollect(); err != nil {
		t.Fatalf("エラーが発生しない想定: %v", err)
	}
	if len(repo.logs) != 1 {
		t.Errorf("InitialCollect でログが保存されていない (count=%d)", len(repo.logs))
	}
}

package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// ---------- フェイク ----------

// fakeGateway は usecase.DeviceGateway のテスト用実装。
type fakeGateway struct {
	devices    map[string][]domain.Device // token → デバイス一覧
	listErr    map[string]error           // token → ListDevices が返すエラー
	statuses   map[domain.DeviceID]domain.StatusPayload
	statusErrs map[domain.DeviceID]error

	listCalls int // ListDevices が呼ばれた回数（API 呼び出しの節約を検証する）
}

func (g *fakeGateway) ListDevices(_ context.Context, cred domain.Credential) ([]domain.Device, error) {
	g.listCalls++
	if err, ok := g.listErr[cred.Token]; ok {
		return nil, err
	}
	return g.devices[cred.Token], nil
}

func (g *fakeGateway) FetchStatus(_ context.Context, _ domain.Credential, id domain.DeviceID) (domain.StatusPayload, error) {
	if err, ok := g.statusErrs[id]; ok {
		return nil, err
	}
	return g.statuses[id], nil
}

// savedStatus は fakeRepo が記録した1件のステータス保存。
type savedStatus struct {
	deviceRecordID domain.DeviceRecordID
	snapshot       domain.StatusSnapshot
}

// fakeRepo は usecase.Repository のテスト用実装。
type fakeRepo struct {
	accountIDs map[string]domain.AccountID      // token → ID
	deviceIDs  map[string]domain.DeviceRecordID // "accountID/deviceID" → ID
	statuses   []savedStatus

	// cache は LoadDeviceCache が返す内容。accountID ごとに保持する。
	cache     map[domain.AccountID][]usecase.DeviceRecord
	syncedAt  map[domain.AccountID]time.Time
	syncMarks []syncMark

	// rateLimit は LoadRateLimitState が返す待機状態。
	// rateLimitSaves は SaveRateLimitState の呼び出し履歴。
	rateLimit      map[domain.AccountID]usecase.RateLimitState
	rateLimitSaves []rateLimitSave

	accountErr       error
	deviceErr        error
	statusErr        error
	cacheErr         error
	syncErr          error
	rateLimitLoadErr error
	rateLimitSaveErr error
}

// rateLimitSave は SaveRateLimitState の呼び出し1件分。
type rateLimitSave struct {
	accountID domain.AccountID
	state     usecase.RateLimitState
}

// syncMark は MarkDevicesSynced の呼び出し1件分。
type syncMark struct {
	accountID domain.AccountID
	at        time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		accountIDs: make(map[string]domain.AccountID),
		deviceIDs:  make(map[string]domain.DeviceRecordID),
		cache:      make(map[domain.AccountID][]usecase.DeviceRecord),
		syncedAt:   make(map[domain.AccountID]time.Time),
		rateLimit:  make(map[domain.AccountID]usecase.RateLimitState),
	}
}

func (r *fakeRepo) LoadRateLimitState(_ context.Context, accountID domain.AccountID) (usecase.RateLimitState, error) {
	if r.rateLimitLoadErr != nil {
		return usecase.RateLimitState{}, r.rateLimitLoadErr
	}
	return r.rateLimit[accountID], nil
}

func (r *fakeRepo) SaveRateLimitState(_ context.Context, accountID domain.AccountID, state usecase.RateLimitState) error {
	if r.rateLimitSaveErr != nil {
		return r.rateLimitSaveErr
	}
	r.rateLimitSaves = append(r.rateLimitSaves, rateLimitSave{accountID: accountID, state: state})
	r.rateLimit[accountID] = state
	return nil
}

func (r *fakeRepo) LoadDeviceCache(_ context.Context, accountID domain.AccountID) ([]usecase.DeviceRecord, time.Time, error) {
	if r.cacheErr != nil {
		return nil, time.Time{}, r.cacheErr
	}
	return r.cache[accountID], r.syncedAt[accountID], nil
}

func (r *fakeRepo) MarkDevicesSynced(_ context.Context, accountID domain.AccountID, at time.Time) error {
	if r.syncErr != nil {
		return r.syncErr
	}
	r.syncMarks = append(r.syncMarks, syncMark{accountID: accountID, at: at})
	r.syncedAt[accountID] = at
	return nil
}

func (r *fakeRepo) SaveAccount(_ context.Context, a domain.Account) (domain.AccountID, error) {
	if r.accountErr != nil {
		return 0, r.accountErr
	}
	if id, ok := r.accountIDs[a.Credential.Token]; ok {
		return id, nil
	}
	id := domain.AccountID(len(r.accountIDs) + 1)
	r.accountIDs[a.Credential.Token] = id
	return id, nil
}

func (r *fakeRepo) SaveDevice(_ context.Context, accountID domain.AccountID, d domain.Device) (domain.DeviceRecordID, error) {
	if r.deviceErr != nil {
		return 0, r.deviceErr
	}
	key := fmt.Sprintf("%d/%s", accountID, d.ID)
	if id, ok := r.deviceIDs[key]; ok {
		return id, nil
	}
	id := domain.DeviceRecordID(len(r.deviceIDs) + 1)
	r.deviceIDs[key] = id
	return id, nil
}

func (r *fakeRepo) AppendStatus(_ context.Context, id domain.DeviceRecordID, s domain.StatusSnapshot) error {
	if r.statusErr != nil {
		return r.statusErr
	}
	r.statuses = append(r.statuses, savedStatus{deviceRecordID: id, snapshot: s})
	return nil
}

// fixedClock は常に同じ時刻を返す usecase.Clock。
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// ---------- ヘルパー ----------

var testNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

func physicalDevice(id, name string) domain.Device {
	return domain.Device{
		ID:                  domain.DeviceID(id),
		Name:                name,
		Type:                "Meter",
		Kind:                domain.DeviceKindPhysical,
		CloudServiceEnabled: true,
	}
}

func account(name, token string) domain.Account {
	return domain.Account{Name: name, Credential: domain.Credential{Token: token, Secret: "sec"}}
}

// outcomes は結果から Outcome の並びだけを取り出す。
func outcomes(r usecase.AccountResult) []usecase.Outcome {
	got := make([]usecase.Outcome, 0, len(r.Devices))
	for _, d := range r.Devices {
		got = append(got, d.Outcome)
	}
	return got
}

// ---------- テスト ----------

func TestExecute_物理デバイスのステータスを保存する(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {dev}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{"temperature":25.5}`)},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	require.Len(t, report.Accounts, 1)
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeStored}, outcomes(report.Accounts[0]))

	require.Len(t, repo.statuses, 1)
	assert.Equal(t, testNow, repo.statuses[0].snapshot.RecordedAt)
	assert.JSONEq(t, `{"temperature":25.5}`, string(repo.statuses[0].snapshot.Payload))
}

func TestExecute_クラウド無効のデバイスは登録のみでスキップする(t *testing.T) {
	disabled := physicalDevice("d1", "Bot")
	disabled.CloudServiceEnabled = false

	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {disabled, physicalDevice("d2", "Meter")}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d2": []byte(`{}`)},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t,
		[]usecase.Outcome{usecase.OutcomeSkippedCloudDisabled, usecase.OutcomeStored},
		outcomes(report.Accounts[0]))
	assert.Len(t, repo.deviceIDs, 2, "スキップしたデバイスも登録される")
	assert.Len(t, repo.statuses, 1)
}

func TestExecute_赤外線リモコンは登録のみ行う(t *testing.T) {
	ir := domain.Device{
		ID:                  "ir1",
		Name:                "エアコン",
		Kind:                domain.DeviceKindInfraredRemote,
		CloudServiceEnabled: true,
	}
	gw := &fakeGateway{devices: map[string][]domain.Device{"tok1": {ir}}}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeRegisteredOnly}, outcomes(report.Accounts[0]))
	assert.Len(t, repo.deviceIDs, 1)
	assert.Empty(t, repo.statuses, "赤外線リモコンはステータスを保存しない")
}

func TestExecute_複数アカウントを処理する(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{
			"tok1": {physicalDevice("d1", "A")},
			"tok2": {physicalDevice("d2", "B")},
		},
		statuses: map[domain.DeviceID]domain.StatusPayload{
			"d1": []byte(`{}`),
			"d2": []byte(`{}`),
		},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{
		account("acc1", "tok1"),
		account("acc2", "tok2"),
	})

	require.NoError(t, report.FatalError())
	require.Len(t, report.Accounts, 2)
	assert.Len(t, repo.accountIDs, 2)
	assert.Len(t, repo.statuses, 2)
}

func TestExecute_デバイス一覧の取得失敗はアカウント単位の致命的エラーになる(t *testing.T) {
	wantErr := errors.New("API エラー")
	gw := &fakeGateway{listErr: map[string]error{"tok1": wantErr}}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Error(t, report.FatalError())
	assert.ErrorIs(t, report.Accounts[0].Err, wantErr, "元のエラーが %w で包まれている")
	assert.Empty(t, report.Accounts[0].Devices)
}

func TestExecute_アカウント登録の失敗はアカウント単位の致命的エラーになる(t *testing.T) {
	wantErr := errors.New("DB エラー")
	gw := &fakeGateway{devices: map[string][]domain.Device{"tok1": {physicalDevice("d1", "A")}}}
	repo := newFakeRepo()
	repo.accountErr = wantErr

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Error(t, report.FatalError())
	assert.ErrorIs(t, report.Accounts[0].Err, wantErr)
}

func TestExecute_ステータス取得の失敗は他のデバイスを止めない(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{
			"tok1": {physicalDevice("d1", "失敗"), physicalDevice("d2", "成功")},
		},
		statuses:   map[domain.DeviceID]domain.StatusPayload{"d2": []byte(`{}`)},
		statusErrs: map[domain.DeviceID]error{"d1": errors.New("デバイスオフライン")},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError(), "デバイス単位の失敗は致命的エラーにしない")
	assert.Equal(t,
		[]usecase.Outcome{usecase.OutcomeFailed, usecase.OutcomeStored},
		outcomes(report.Accounts[0]))
	assert.Error(t, report.Accounts[0].Devices[0].Err)
	assert.Len(t, repo.statuses, 1)
}

func TestExecute_ステータス保存の失敗は他のデバイスを止めない(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{
			"tok1": {physicalDevice("d1", "A"), physicalDevice("d2", "B")},
		},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`), "d2": []byte(`{}`)},
	}
	repo := newFakeRepo()
	repo.statusErr = errors.New("DB 書き込みエラー")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t,
		[]usecase.Outcome{usecase.OutcomeFailed, usecase.OutcomeFailed},
		outcomes(report.Accounts[0]))
	assert.Empty(t, repo.statuses)
}

func TestExecute_デバイス登録の失敗はそのデバイスのみ失敗にする(t *testing.T) {
	gw := &fakeGateway{devices: map[string][]domain.Device{"tok1": {physicalDevice("d1", "A")}}}
	repo := newFakeRepo()
	repo.deviceErr = errors.New("DB エラー")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeFailed}, outcomes(report.Accounts[0]))
}

func TestExecute_全デバイスが同じ収集時刻を共有する(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{
			"tok1": {physicalDevice("d1", "A"), physicalDevice("d2", "B")},
		},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`), "d2": []byte(`{}`)},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Len(t, repo.statuses, 2)
	assert.Equal(t, repo.statuses[0].snapshot.RecordedAt, repo.statuses[1].snapshot.RecordedAt)
}

func TestFatalError_複数アカウントのエラーを全件保持する(t *testing.T) {
	err1 := errors.New("アカウント1の失敗")
	err2 := errors.New("アカウント2の失敗")

	report := usecase.CollectReport{Accounts: []usecase.AccountResult{
		{AccountName: "a1", Err: err1},
		{AccountName: "a2", Err: err2},
	}}

	joined := report.FatalError()
	require.Error(t, joined)
	assert.ErrorIs(t, joined, err1)
	assert.ErrorIs(t, joined, err2, "最後の1件だけでなく全件が保持される")
}

func TestFatalError_エラーがなければnilを返す(t *testing.T) {
	report := usecase.CollectReport{Accounts: []usecase.AccountResult{{AccountName: "a1"}}}
	assert.NoError(t, report.FatalError())
}

// ---------- デバイス一覧キャッシュ ----------

// cachedDevice は devices テーブルに登録済みのデバイスを表すヘルパー。
func cachedDevice(recordID int64, dev domain.Device) usecase.DeviceRecord {
	return usecase.DeviceRecord{ID: domain.DeviceRecordID(recordID), Device: dev}
}

func TestExecute_キャッシュが新しければデバイス一覧APIを呼ばない(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {dev}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`)},
	}
	repo := newFakeRepo()
	repo.accountIDs["tok1"] = 1
	repo.cache[1] = []usecase.DeviceRecord{cachedDevice(42, dev)}
	repo.syncedAt[1] = testNow.Add(-1 * time.Hour)

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 24*time.Hour)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, 0, gw.listCalls, "キャッシュが有効な間は /v1.1/devices を呼ばない")
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeStored}, outcomes(report.Accounts[0]))

	// devices への登録もやり直さず、キャッシュの devices.id をそのまま使う。
	assert.Empty(t, repo.deviceIDs, "登録済みのデバイスを保存し直さない")
	require.Len(t, repo.statuses, 1)
	assert.Equal(t, domain.DeviceRecordID(42), repo.statuses[0].deviceRecordID)
	assert.Empty(t, repo.syncMarks, "API を叩いていないので同期時刻は進めない")
}

func TestExecute_キャッシュが期限切れならAPIから取り直す(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {dev}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`)},
	}
	repo := newFakeRepo()
	repo.accountIDs["tok1"] = 1
	repo.cache[1] = []usecase.DeviceRecord{cachedDevice(42, dev)}
	repo.syncedAt[1] = testNow.Add(-25 * time.Hour)

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 24*time.Hour)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, 1, gw.listCalls)
	assert.Equal(t, []syncMark{{accountID: 1, at: testNow}}, repo.syncMarks,
		"取り直したら同期時刻を収集時刻まで進める")
}

func TestExecute_キャッシュが空なら期限内でもAPIから取り直す(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {dev}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`)},
	}
	repo := newFakeRepo()
	repo.accountIDs["tok1"] = 1
	repo.syncedAt[1] = testNow.Add(-1 * time.Minute) // 時刻だけ残り、デバイスは0件

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 24*time.Hour)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, 1, gw.listCalls, "一覧が空のまま API を呼ばなくなるのを防ぐ")
}

func TestExecute_時計が巻き戻ってもAPIから取り直す(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {dev}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`)},
	}
	repo := newFakeRepo()
	repo.accountIDs["tok1"] = 1
	repo.cache[1] = []usecase.DeviceRecord{cachedDevice(42, dev)}
	repo.syncedAt[1] = testNow.Add(1 * time.Hour) // 未来の時刻

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 24*time.Hour)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, 1, gw.listCalls)
}

func TestExecute_TTLが0ならキャッシュを読みに行かない(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {dev}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`)},
	}
	repo := newFakeRepo()
	repo.accountIDs["tok1"] = 1
	repo.cache[1] = []usecase.DeviceRecord{cachedDevice(42, dev)}
	repo.syncedAt[1] = testNow
	repo.cacheErr = errors.New("読まれたら失敗するはず")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, 1, gw.listCalls)
}

func TestExecute_デバイス登録に失敗したら同期時刻を進めない(t *testing.T) {
	gw := &fakeGateway{
		devices: map[string][]domain.Device{"tok1": {physicalDevice("d1", "温湿度計")}},
	}
	repo := newFakeRepo()
	repo.deviceErr = errors.New("登録失敗")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 24*time.Hour)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeFailed}, outcomes(report.Accounts[0]))
	assert.Empty(t, repo.syncMarks, "欠けた一覧をキャッシュとして使わせない")
}

func TestExecute_ステータス取得の失敗では同期時刻を進める(t *testing.T) {
	gw := &fakeGateway{
		devices:    map[string][]domain.Device{"tok1": {physicalDevice("d1", "温湿度計")}},
		statusErrs: map[domain.DeviceID]error{"d1": errors.New("取得失敗")},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 24*time.Hour)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Len(t, repo.syncMarks, 1, "一覧そのものは正しく取れているのでキャッシュしてよい")
}

func TestExecute_キャッシュ読み込みの失敗はアカウント単位の致命的エラーになる(t *testing.T) {
	gw := &fakeGateway{devices: map[string][]domain.Device{"tok1": {physicalDevice("d1", "温湿度計")}}}
	repo := newFakeRepo()
	repo.cacheErr = errors.New("DB 障害")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 24*time.Hour)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Error(t, report.FatalError())
	assert.Equal(t, 0, gw.listCalls)
}

func TestExecute_同期時刻の記録失敗はアカウント単位の致命的エラーになる(t *testing.T) {
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {physicalDevice("d1", "温湿度計")}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`)},
	}
	repo := newFakeRepo()
	repo.syncErr = errors.New("UPDATE 失敗")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 24*time.Hour)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Error(t, report.FatalError())
	// ステータス自体は保存できている（記録の失敗で収集結果を捨てない）。
	assert.Len(t, repo.statuses, 1)
}

// reportingGateway は残り呼び出し回数も報告できる fakeGateway。
// 本物と同じく、読んだら記録が空に戻る。
type reportingGateway struct {
	*fakeGateway
	remaining string
}

func (g *reportingGateway) TakeRemaining() string {
	v := g.remaining
	g.remaining = ""
	return v
}

func TestExecute_残り呼び出し回数をレポートに載せる(t *testing.T) {
	gw := &reportingGateway{
		fakeGateway: &fakeGateway{
			devices:  map[string][]domain.Device{"tok1": {physicalDevice("d1", "温湿度計")}},
			statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`)},
		},
		remaining: "9899",
	}

	uc := usecase.NewCollectStatus(gw, newFakeRepo(), fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, "9899", report.Accounts[0].RateLimitRemaining)
}

func TestExecute_残りを報告できないgatewayでは空のままにする(t *testing.T) {
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {physicalDevice("d1", "温湿度計")}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{}`)},
	}

	uc := usecase.NewCollectStatus(gw, newFakeRepo(), fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Empty(t, report.Accounts[0].RateLimitRemaining)
}

func TestExecute_残り回数を別のアカウントに持ち越さない(t *testing.T) {
	gw := &reportingGateway{
		fakeGateway: &fakeGateway{
			devices: map[string][]domain.Device{
				"tok1": {physicalDevice("d1", "温湿度計")},
				"tok2": {physicalDevice("d2", "CO2センサー")},
			},
			statuses: map[domain.DeviceID]domain.StatusPayload{
				"d1": []byte(`{}`),
				"d2": []byte(`{}`),
			},
		},
		remaining: "9899",
	}

	uc := usecase.NewCollectStatus(gw, newFakeRepo(), fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{
		account("acc1", "tok1"),
		account("acc2", "tok2"),
	})

	require.NoError(t, report.FatalError())
	require.Len(t, report.Accounts, 2)
	assert.Equal(t, "9899", report.Accounts[0].RateLimitRemaining)
	// アカウントごとに枠が別なので、直前の値を付け回さない。
	assert.Empty(t, report.Accounts[1].RateLimitRemaining)
}

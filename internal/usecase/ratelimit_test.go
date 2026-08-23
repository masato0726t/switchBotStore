package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"switchBotStore/internal/domain"
	"switchBotStore/internal/usecase"
)

// rateLimitErr は SwitchBot API が実際に返す 429 を模したエラーを作る。
// 応答本文とヘッダーは 2026-08-23 の実測値。
func rateLimitErr(path string) *usecase.GatewayError {
	return &usecase.GatewayError{
		Path:        path,
		HTTPStatus:  429,
		APIMessage:  "Too Many Requests",
		Body:        `{"message":"Too Many Requests"}`,
		Remaining:   "0",
		RateLimited: true,
	}
}

func TestGatewayError_レート制限は番兵エラーとして判定できる(t *testing.T) {
	err := rateLimitErr("/v1.1/devices")
	assert.True(t, errors.Is(err, usecase.ErrRateLimited))

	other := &usecase.GatewayError{Path: "/v1.1/devices", HTTPStatus: 500}
	assert.False(t, errors.Is(other, usecase.ErrRateLimited))
}

func TestGatewayError_メッセージにHTTPステータスと応答が入る(t *testing.T) {
	// 以前は応答 JSON の statusCode だけを出していたため、封筒の無い応答が
	// "statusCode=0" とだけ表示され、ログから何が起きたのか読めなかった。
	msg := rateLimitErr("/v1.1/devices").Error()
	assert.Contains(t, msg, "http_status=429")
	assert.Contains(t, msg, "/v1.1/devices")
	assert.Contains(t, msg, "Too Many Requests")
}

func TestGatewayError_LogAttrsに応答の内訳が入る(t *testing.T) {
	attrs := rateLimitErr("/v1.1/devices").LogAttrs()

	got := map[string]any{}
	for i := 0; i+1 < len(attrs); i += 2 {
		got[attrs[i].(string)] = attrs[i+1]
	}

	assert.Equal(t, 429, got["http_status"])
	assert.Equal(t, "/v1.1/devices", got["path"])
	assert.Equal(t, "Too Many Requests", got["api_message"])
	assert.Equal(t, `{"message":"Too Many Requests"}`, got["response_body"])
	assert.Equal(t, "0", got["rate_limit_remaining"])

	// 返ってこなかったヘッダーはログ行に出さない。
	assert.NotContains(t, got, "retry_after")
	assert.NotContains(t, got, "rate_limit_limit")
}

func TestExecute_レート制限は致命的エラーにしない(t *testing.T) {
	gw := &fakeGateway{
		listErr: map[string]error{"tok1": rateLimitErr("/v1.1/devices")},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	// 終了コードを 0 に保つ。毎分 exit 1 を返すと systemd がユニットを
	// failed にし、ServiceFailed の警報が制限の続くあいだ鳴り続けるため。
	require.NoError(t, report.FatalError())
	assert.True(t, report.RateLimited())

	require.Len(t, report.Accounts, 1)
	acc := report.Accounts[0]
	assert.True(t, acc.RateLimited())
	require.NotNil(t, acc.RateLimit)
	assert.Equal(t, usecase.RateLimitPhaseHit, acc.RateLimit.Phase)
	assert.Equal(t, 1, acc.RateLimit.State.Failures)
	assert.Equal(t, testNow.Add(1*time.Minute), acc.RateLimit.State.Until)
	assert.Equal(t, testNow, acc.RateLimit.State.Since)
}

func TestExecute_待機中はAPIを1回も呼ばない(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{devices: map[string][]domain.Device{"tok1": {dev}}}
	repo := newFakeRepo()
	repo.rateLimit[1] = usecase.RateLimitState{
		Until:    testNow.Add(5 * time.Minute),
		Since:    testNow.Add(-20 * time.Minute),
		Failures: 4,
	}

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, 0, gw.listCalls, "待機中は上流 API を呼ばない")
	assert.Empty(t, repo.statuses)

	acc := report.Accounts[0]
	require.NotNil(t, acc.RateLimit)
	assert.Equal(t, usecase.RateLimitPhaseWaiting, acc.RateLimit.Phase)
	assert.Equal(t, 20*time.Minute, acc.RateLimit.BlockedFor)
	assert.Equal(t, 5*time.Minute, acc.RateLimit.RetryIn)

	// 待機中は状態を書き換えない（待機時間が伸び続けるのを避ける）。
	assert.Empty(t, repo.rateLimitSaves)
}

func TestExecute_待機が明けて成功したら待機状態を解除する(t *testing.T) {
	dev := physicalDevice("d1", "温湿度計")
	gw := &fakeGateway{
		devices:  map[string][]domain.Device{"tok1": {dev}},
		statuses: map[domain.DeviceID]domain.StatusPayload{"d1": []byte(`{"temperature":25.5}`)},
	}
	repo := newFakeRepo()
	repo.rateLimit[1] = usecase.RateLimitState{
		Until:    testNow.Add(-1 * time.Minute),
		Since:    testNow.Add(-30 * time.Minute),
		Failures: 3,
	}

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	assert.Equal(t, 1, gw.listCalls)
	require.Len(t, repo.statuses, 1)

	acc := report.Accounts[0]
	require.NotNil(t, acc.RateLimit)
	assert.Equal(t, usecase.RateLimitPhaseRecovered, acc.RateLimit.Phase)
	assert.Equal(t, 30*time.Minute, acc.RateLimit.BlockedFor)

	require.Len(t, repo.rateLimitSaves, 1)
	assert.Equal(t, usecase.RateLimitState{}, repo.rateLimitSaves[0].state)
}

func TestExecute_待機時間は倍になり15分で頭打ちになる(t *testing.T) {
	tests := []struct {
		name     string
		failures int // 直前までの連続失敗回数
		want     time.Duration
	}{
		{name: "1回目は1分", failures: 0, want: 1 * time.Minute},
		{name: "2回目は2分", failures: 1, want: 2 * time.Minute},
		{name: "3回目は4分", failures: 2, want: 4 * time.Minute},
		{name: "4回目は8分", failures: 3, want: 8 * time.Minute},
		// 16分にはせず15分で止める。同じ CT の auto-air-conditioner が
		// センサー値を有効とみなす20分より短くしておくため。
		{name: "5回目は15分で頭打ち", failures: 4, want: 15 * time.Minute},
		{name: "以降も15分のまま", failures: 20, want: 15 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeGateway{listErr: map[string]error{"tok1": rateLimitErr("/v1.1/devices")}}
			repo := newFakeRepo()
			if tt.failures > 0 {
				repo.rateLimit[1] = usecase.RateLimitState{
					Until:    testNow.Add(-1 * time.Second), // 期限切れ＝待機していない
					Since:    testNow.Add(-1 * time.Hour),
					Failures: tt.failures,
				}
			}

			uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
			report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

			acc := report.Accounts[0]
			require.NotNil(t, acc.RateLimit)
			assert.Equal(t, tt.failures+1, acc.RateLimit.State.Failures)
			assert.Equal(t, tt.want, acc.RateLimit.State.Until.Sub(testNow))
		})
	}
}

func TestExecute_ステータス取得で制限に当たったら残りのデバイスを呼ばない(t *testing.T) {
	d1 := physicalDevice("d1", "温湿度計")
	d2 := physicalDevice("d2", "CO2センサー")
	d3 := physicalDevice("d3", "外温度計")
	gw := &fakeGateway{
		devices: map[string][]domain.Device{"tok1": {d1, d2, d3}},
		statuses: map[domain.DeviceID]domain.StatusPayload{
			"d1": []byte(`{"temperature":25.5}`),
			"d3": []byte(`{"temperature":31.0}`),
		},
		statusErrs: map[domain.DeviceID]error{
			"d2": rateLimitErr("/v1.1/devices/d2/status"),
		},
	}
	repo := newFakeRepo()

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.NoError(t, report.FatalError())
	acc := report.Accounts[0]

	// d1 は保存済み、d2 で制限に当たって打ち切り。d3 は呼ばれない。
	assert.Equal(t, []usecase.Outcome{usecase.OutcomeStored, usecase.OutcomeFailed}, outcomes(acc))
	require.Len(t, repo.statuses, 1)

	require.NotNil(t, acc.RateLimit)
	assert.Equal(t, usecase.RateLimitPhaseHit, acc.RateLimit.Phase)
}

func TestExecute_待機状態の保存に失敗したら致命的エラーにする(t *testing.T) {
	// 待機状態を書けないまま「見送った」ことにすると、次回も同じだけ
	// API を叩いてしまいバックオフが効かない。気づける形で失敗させる。
	gw := &fakeGateway{listErr: map[string]error{"tok1": rateLimitErr("/v1.1/devices")}}
	repo := newFakeRepo()
	repo.rateLimitSaveErr = errors.New("DB 障害")

	uc := usecase.NewCollectStatus(gw, repo, fixedClock{t: testNow}, 0)
	report := uc.Execute(context.Background(), []domain.Account{account("acc1", "tok1")})

	require.Error(t, report.FatalError())
	assert.False(t, report.RateLimited())
}

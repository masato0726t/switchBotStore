package usecase

import "time"

// maxRateLimitBackoff は 429 が続いたときの待機時間の上限。
//
// 20分ではなく15分にしているのは、同じ CT で動く auto-air-conditioner が
// センサー値を有効とみなす上限が20分 (sensor_max_age_min) だからである。
// 待機の上限をそれより短くしておけば、レート制限が解けたことに最大15分
// 気づかなくても、次の収集で入る値はエアコン制御にとって十分新しい。
const maxRateLimitBackoff = 15 * time.Minute

// initialRateLimitBackoff は最初に 429 を受けたときの待機時間。
// 実行間隔 (毎分) と同じにしてあるので、1回目は事実上「次の回を待つ」だけになる。
const initialRateLimitBackoff = 1 * time.Minute

// RateLimitState はアカウント単位のレート制限の状態。DB に永続化する。
//
// 収集は1分ごとに別プロセスとして起動するため、待機の状態はプロセス内に
// 持てない。api_accounts の列として保存し、次の起動が読み直す。
type RateLimitState struct {
	// Until はこの時刻まで API を呼ばない。ゼロ値なら待機していない。
	Until time.Time
	// Since は連続した 429 のうち最初のものを受けた時刻。ゼロ値なら待機していない。
	Since time.Time
	// Failures は連続して 429 を受けた回数。
	Failures int
}

// Blocked は now の時点で API の呼び出しを見送るべきかを返す。
func (s RateLimitState) Blocked(now time.Time) bool {
	return !s.Until.IsZero() && now.Before(s.Until)
}

// Active は待機中または過去に 429 を受けて未回復かを返す。
func (s RateLimitState) Active() bool {
	return s.Failures > 0 || !s.Until.IsZero()
}

// BlockedFor は最初に 429 を受けてからの経過時間を返す。
func (s RateLimitState) BlockedFor(now time.Time) time.Duration {
	if s.Since.IsZero() {
		return 0
	}
	if d := now.Sub(s.Since); d > 0 {
		return d
	}
	return 0
}

// next は 429 をもう1回受けたときの新しい状態を返す。
//
// 待機時間は 1分から始めて倍にしていき、maxRateLimitBackoff で頭打ちにする。
func (s RateLimitState) next(now time.Time) RateLimitState {
	since := s.Since
	if since.IsZero() {
		since = now
	}
	failures := s.Failures + 1
	return RateLimitState{
		Until:    now.Add(backoffFor(failures)),
		Since:    since,
		Failures: failures,
	}
}

// backoffFor は連続失敗回数に対する待機時間を返す。
// 1分, 2分, 4分, 8分と倍にして maxRateLimitBackoff で止める。
func backoffFor(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	d := initialRateLimitBackoff
	for i := 1; i < failures; i++ {
		d *= 2
		if d >= maxRateLimitBackoff {
			return maxRateLimitBackoff
		}
	}
	if d > maxRateLimitBackoff {
		return maxRateLimitBackoff
	}
	return d
}

// RateLimitPhase は今回の実行がレート制限とどう関わったかを表す。
type RateLimitPhase int

const (
	// RateLimitPhaseNone はレート制限と無関係に終わったことを表す。
	RateLimitPhaseNone RateLimitPhase = iota
	// RateLimitPhaseHit は今回 API を呼んで拒否されたことを表す。
	RateLimitPhaseHit
	// RateLimitPhaseWaiting は待機中のため API を呼ばずに見送ったことを表す。
	RateLimitPhaseWaiting
	// RateLimitPhaseRecovered は待機が明けて収集を再開できたことを表す。
	RateLimitPhaseRecovered
)

// String はログ出力用の識別子を返す。
func (p RateLimitPhase) String() string {
	switch p {
	case RateLimitPhaseHit:
		return "hit"
	case RateLimitPhaseWaiting:
		return "waiting"
	case RateLimitPhaseRecovered:
		return "recovered"
	default:
		return "none"
	}
}

// RateLimitOutcome は今回の実行でのレート制限の顛末。ログ出力に使う。
type RateLimitOutcome struct {
	Phase RateLimitPhase
	// State は今回の実行を終えた時点の状態。
	State RateLimitState
	// BlockedFor は最初に 429 を受けてからの経過時間。
	BlockedFor time.Duration
	// RetryIn は次に API を呼ぶまでの残り時間。
	RetryIn time.Duration
}

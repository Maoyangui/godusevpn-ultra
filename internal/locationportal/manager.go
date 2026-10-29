package locationportal

import (
	"context"
	"errors"
	"math"
	"os"
	"sync"
	"time"
)

const Scope = "覆盖 Windows 定位服务供使用该服务的应用读取；不改变 IP、GNSS 硬件或使用自行定位的应用。"
const CompetitionBoundary = "门户接口没有与其他程序的原子比较替换；修改前会重新读取，发现不同状态会停止，不能排除同时修改或相同值往返。"

type Report struct {
	Status                string       `json:"status"`
	Connected             bool         `json:"connected"`
	Override              bool         `json:"override"`
	Managed               bool         `json:"managed"`
	CanApply              bool         `json:"canApply"`
	CanRestore            bool         `json:"canRestore"`
	ConfiguredPosition    *Position    `json:"configuredPosition,omitempty"`
	ConfigurationVerified bool         `json:"configurationVerified"`
	ConsumerVerified      bool         `json:"consumerVerified"`
	Consumer              *Observation `json:"consumer,omitempty"`
	RunningTrajectory     bool         `json:"runningTrajectory"`
	CompletedPoints       int          `json:"completedPoints"`
	TotalPoints           int          `json:"totalPoints"`
	ObservedAt            string       `json:"observedAt,omitempty"`
	Scope                 string       `json:"scope"`
	Detail                string       `json:"detail"`
	AtomicExternalCAS     bool         `json:"atomicExternalCAS"`
	Warnings              []string     `json:"warnings"`
}
type Observation struct {
	Position       Position `json:"position"`
	Timestamp      string   `json:"timestamp"`
	AccuracyMetres float64  `json:"accuracyMetres"`
	PositionSource string   `json:"positionSource"`
}
type ObserveRequest struct {
	RequestedAfter string `json:"requestedAfter"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

func DecodeObservation(raw []byte) (Observation, error) {
	var r struct {
		SchemaVersion int       `json:"schemaVersion"`
		Status        string    `json:"status"`
		Position      *Position `json:"position"`
		Timestamp     string    `json:"timestamp"`
		Accuracy      *float64  `json:"accuracyMetres"`
		Source        string    `json:"positionSource"`
	}
	if len(raw) > 4096 || strict(raw, &r) != nil || r.SchemaVersion != 1 || r.Status != "observed" || r.Position == nil || r.Accuracy == nil || r.Source != "default" {
		return Observation{}, ErrReadback
	}
	if _, e := time.Parse(time.RFC3339Nano, r.Timestamp); e != nil {
		return Observation{}, ErrReadback
	}
	return Observation{*r.Position, r.Timestamp, *r.Accuracy, r.Source}, nil
}

type Consumer func(context.Context, ObserveRequest) (Observation, error)
type ApplyRequest struct {
	Points          []Position `json:"points"`
	IntervalSeconds int        `json:"intervalSeconds"`
}

func ParseApply(raw string) (ApplyRequest, error) {
	var r ApplyRequest
	if strict([]byte(raw), &r) != nil || len(r.Points) < 1 || len(r.Points) > 120 {
		return r, ErrInput
	}
	if len(r.Points) == 1 {
		if r.IntervalSeconds != 0 {
			return r, ErrInput
		}
	} else if r.IntervalSeconds < 2 || r.IntervalSeconds > 60 || r.IntervalSeconds*(len(r.Points)-1) > 600 {
		return r, ErrInput
	}
	for _, p := range r.Points {
		if !validPosition(p) {
			return r, ErrInput
		}
	}
	return r, nil
}

type portalAPI interface {
	Check(context.Context) (bool, error)
	Override(context.Context) (bool, error)
	Injected(context.Context) (Position, error)
	setOverride(context.Context, bool) error
	setPosition(context.Context, Position) error
	Close() error
}
type Manager struct {
	mu                     sync.Mutex
	observeMu              sync.Mutex
	observeCancel          context.CancelFunc
	client                 portalAPI
	config                 Config
	identity               func() (userIdentity, error)
	open                   openStore
	consume                Consumer
	now                    func() time.Time
	cancel                 context.CancelFunc
	done                   chan struct{}
	completed, total       int
	trajectoryError        error
	trajectoryID           uint64
	waitPoint              func(context.Context, time.Duration) error
	beforeTrajectoryFinish func(uint64)
}

// New constructs only. It performs no positioning, portal requests or file I/O.
func New(consumer Consumer) *Manager {
	return &Manager{identity: nativeIdentity, open: openNativeStore, consume: consumer, now: time.Now, waitPoint: waitTrajectoryPoint}
}
func baseReport() Report {
	return Report{Status: "not_connected", Scope: Scope, Detail: "连接经过证书核验的本机门户后才可操作。", Warnings: []string{CompetitionBoundary, "恢复只关闭本应用启用的覆盖；系统接口未提供清除已注入位置缓存的方法。"}}
}
func (m *Manager) Connect(ctx context.Context, c Config) (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := baseReport()
	if m.client != nil && m.config == c {
		return m.readLocked(ctx)
	}
	if m.client != nil || m.cancel != nil {
		return r, ErrBusy
	}
	if _, e := m.identity(); e != nil {
		return r, ErrUnsupported
	}
	client, e := NewClient(c)
	if e != nil {
		return r, e
	}
	if _, e = client.Check(ctx); e != nil {
		client.Close()
		return r, e
	}
	m.client = client
	m.config = c
	return m.readLocked(ctx)
}
func (m *Manager) Read(ctx context.Context) (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readLocked(ctx)
}
func (m *Manager) readLocked(ctx context.Context) (Report, error) {
	r := baseReport()
	if m.client == nil {
		return r, nil
	}
	id, e := m.identity()
	if e != nil {
		return r, ErrUnsupported
	}
	on, e := m.client.Check(ctx)
	if e != nil {
		return r, e
	}
	r.Connected = true
	r.Override = on
	r.CanApply = !on
	r.Status = "connected"
	r.Detail = "认证与证书已核对；尚未启用本应用的位置覆盖。"
	r.ObservedAt = m.now().UTC().Format(time.RFC3339Nano)
	s, e := m.open(id, storeReadOnly)
	if errors.Is(e, os.ErrNotExist) {
		if on {
			r.Status = "external_override"
			r.Detail = "已有其他覆盖，未接管；请由其原操作方恢复。"
		}
		return r, nil
	}
	if e != nil {
		return r, ErrStore
	}
	defer s.close()
	j, e := load(s, id.sid)
	if e != nil {
		return r, e
	}
	if j == nil || j.Stage == "restored" {
		if on {
			r.Status = "external_override"
			r.Detail = "存在无本应用有效记录的覆盖，禁止接管。"
		}
		return r, nil
	}
	if !m.sameEndpoint(j) {
		r.Status = "other_endpoint_backup"
		r.CanApply = false
		return r, ErrConflict
	}
	r.Managed = true
	r.CanApply = false
	r.Status = j.Stage
	r.CanRestore = true
	r.Detail = "存在本应用私有备份；恢复前会重新核对当前覆盖。"
	{
		p, e := m.client.Injected(ctx)
		if e != nil {
			return r, e
		}
		r.ConfiguredPosition = &p
		if !acceptsRecovery(j, p, on) {
			r.Status = "external_conflict"
			r.CanRestore = false
			return r, ErrConflict
		}
		r.ConfigurationVerified = on && j.Stage == "active" && j.Expected != nil && *j.Expected == p
		r.CanApply = on && j.Stage == "active"
		if !on && j.Stage == "active" {
			r.Status = "override_disabled"
			r.Detail = "覆盖已被关闭；本应用的注入缓存仍可按原备份恢复，不会自动重新启用。"
		}
	}
	r.RunningTrajectory = m.cancel != nil
	r.CompletedPoints = m.completed
	r.TotalPoints = m.total
	if m.trajectoryError != nil {
		r.Status = "trajectory_stopped"
		r.Detail = "轨迹已停止；未重试，已应用的覆盖仍需恢复。"
	}
	return r, nil
}
func (m *Manager) sameEndpoint(j *transaction) bool {
	return j.Port == m.config.Port && j.Certificate == m.config.CertificateSHA256
}
func accepts(j *transaction, p Position) bool {
	return j.Expected != nil && *j.Expected == p || j.Pending != nil && *j.Pending == p
}
func acceptsRecovery(j *transaction, p Position, on bool) bool {
	return accepts(j, p) || j.Before != nil && p == *j.Before && (!on || j.Stage == "restoring" || j.Stage == "applying" && j.Expected == nil)
}
func (m *Manager) applyPoint(ctx context.Context, p Position) error {
	if m.client == nil {
		return ErrConnection
	}
	id, e := m.identity()
	if e != nil {
		return ErrUnsupported
	}
	s, e := m.open(id, storeCreate)
	if e != nil {
		return e
	}
	defer s.close()
	j, e := load(s, id.sid)
	if e != nil {
		return e
	}
	on, e := m.client.Check(ctx)
	if e != nil {
		return e
	}
	var previous Position
	if j == nil || j.Stage == "restored" {
		if on {
			return ErrConflict
		}
		before, readError := m.client.Injected(ctx)
		if readError != nil {
			return readError
		}
		j = &transaction{Owner: id.sid, Port: m.config.Port, Certificate: m.config.CertificateSHA256, Stage: "applying", Before: &before}
		previous = before
	} else {
		if !m.sameEndpoint(j) || j.Stage != "active" || !on {
			return ErrConflict
		}
		p, e := m.client.Injected(ctx)
		if e != nil {
			return e
		}
		if !accepts(j, p) {
			return ErrConflict
		}
		previous = p
	}
	j.Stage = "applying"
	j.Pending = &p
	if save(s, j) != nil {
		return ErrStore
	}
	actualBefore, e := m.client.Override(ctx)
	if e != nil || actualBefore != on {
		return ErrConflict
	}
	cacheBefore, e := m.client.Injected(ctx)
	if e != nil {
		return e
	}
	if cacheBefore != previous {
		return ErrConflict
	}
	// The official endpoint requires override enabled before setting position.
	// Its old cached coordinate can be active briefly; the UI makes no consumer
	// claim until the new coordinate is independently read. Intent is durable.
	if !on {
		if e = m.client.setOverride(ctx, true); e != nil {
			return e
		}
	}
	if e = m.client.setPosition(ctx, p); e != nil {
		return e
	}
	got, e := m.client.Injected(ctx)
	if e != nil || got != p {
		return ErrReadback
	}
	actual, e := m.client.Override(ctx)
	if e != nil || !actual {
		return ErrReadback
	}
	got, e = m.client.Injected(ctx)
	if e != nil || got != p {
		return ErrReadback
	}
	j.Stage = "active"
	j.Expected = &p
	j.Pending = nil
	j.WrittenAt = m.now().UTC().Format(time.RFC3339Nano)
	if save(s, j) != nil {
		return ErrStore
	}
	return nil
}
func (m *Manager) Apply(ctx context.Context, r ApplyRequest) (Report, error) {
	// Revalidate typed callers too; the JSON parser is not a trust boundary.
	if len(r.Points) < 1 || len(r.Points) > 120 || len(r.Points) == 1 && r.IntervalSeconds != 0 || len(r.Points) > 1 && (r.IntervalSeconds < 2 || r.IntervalSeconds > 60 || r.IntervalSeconds*(len(r.Points)-1) > 600) {
		return baseReport(), ErrInput
	}
	for _, p := range r.Points {
		if !validPosition(p) {
			return baseReport(), ErrInput
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return baseReport(), ErrBusy
	}
	if e := m.applyPoint(ctx, r.Points[0]); e != nil {
		return baseReport(), e
	}
	m.completed, m.total = 1, len(r.Points)
	m.trajectoryError = nil
	if len(r.Points) > 1 {
		run, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.done = make(chan struct{})
		m.trajectoryID++
		go m.trajectory(run, append([]Position(nil), r.Points[1:]...), time.Duration(r.IntervalSeconds)*time.Second, m.done, m.trajectoryID)
	}
	return m.readLocked(ctx)
}
func waitTrajectoryPoint(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (m *Manager) finishTrajectory(owner uint64, done chan struct{}, err error) {
	if m.beforeTrajectoryFinish != nil {
		m.beforeTrajectoryFinish(owner)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.trajectoryID != owner || m.done != done {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.cancel = nil
	if err != nil {
		m.trajectoryError = err
	}
}
func (m *Manager) trajectory(ctx context.Context, points []Position, interval time.Duration, done chan struct{}, owner uint64) {
	defer close(done)
	for _, p := range points {
		if m.waitPoint(ctx, interval) != nil {
			return
		}
		m.mu.Lock()
		if ctx.Err() != nil || m.trajectoryID != owner || m.done != done {
			m.mu.Unlock()
			return
		}
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		e := m.applyPoint(call, p)
		cancel()
		if e != nil {
			m.mu.Unlock()
			m.finishTrajectory(owner, done, e)
			return
		}
		m.completed++
		m.mu.Unlock()
	}
	m.finishTrajectory(owner, done, nil)
}
func (m *Manager) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.trajectoryID++
	m.mu.Unlock()
}
func (m *Manager) Restore(ctx context.Context) (Report, error) {
	m.Stop()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		return baseReport(), ErrConnection
	}
	id, e := m.identity()
	if e != nil {
		return baseReport(), ErrUnsupported
	}
	s, e := m.open(id, storeUpdate)
	if e != nil {
		return baseReport(), ErrStore
	}
	defer s.close()
	j, e := load(s, id.sid)
	if e != nil || j == nil || j.Stage == "restored" {
		return baseReport(), ErrStore
	}
	if !m.sameEndpoint(j) {
		return baseReport(), ErrConflict
	}
	on, e := m.client.Check(ctx)
	if e != nil {
		return baseReport(), e
	}
	current, e := m.client.Injected(ctx)
	if e != nil {
		return baseReport(), e
	}
	// Disabled does not mean empty. Preserve and restore the original injected
	// cache too; an unreadable/unknown cache is never treated as absent.
	if !acceptsRecovery(j, current, on) {
		return baseReport(), ErrConflict
	}
	j.Expected = &current
	j.Stage = "restoring"
	j.Pending = nil
	if save(s, j) != nil {
		return baseReport(), ErrStore
	}
	freshOn, e := m.client.Override(ctx)
	if e != nil || freshOn != on {
		return baseReport(), ErrConflict
	}
	freshCache, e := m.client.Injected(ctx)
	if e != nil || freshCache != current {
		return baseReport(), ErrConflict
	}
	modeEnabled := on
	if current != *j.Before && !on {
		if e = m.client.setOverride(ctx, true); e != nil {
			return baseReport(), e
		}
		modeEnabled = true
	}
	if current != *j.Before {
		if e = m.client.setPosition(ctx, *j.Before); e != nil {
			return baseReport(), e
		}
	}
	readBefore, e := m.client.Injected(ctx)
	if e != nil || readBefore != *j.Before {
		return baseReport(), ErrReadback
	}
	if modeEnabled {
		if e = m.client.setOverride(ctx, false); e != nil {
			return baseReport(), e
		}
	}
	on, e = m.client.Override(ctx)
	if e != nil || on {
		return baseReport(), ErrReadback
	}
	j.Stage = "restored"
	j.Expected = nil
	j.Pending = nil
	if save(s, j) != nil {
		return baseReport(), ErrStore
	}
	r := baseReport()
	r.Connected = true
	r.Status = "restored"
	r.CanApply = true
	r.Detail = "原注入坐标已恢复，覆盖已关闭并分别读回；未使用或声称存在清空系统缓存接口。"
	return r, nil
}
func validObservation(o Observation, target Position, after, now time.Time) bool {
	t, e := time.Parse(time.RFC3339Nano, o.Timestamp)
	return e == nil && validPosition(o.Position) && o.PositionSource == "default" && !t.Before(after) && !t.After(now.Add(2*time.Second)) && now.Sub(t) <= 15*time.Second && !math.IsNaN(o.AccuracyMetres) && !math.IsInf(o.AccuracyMetres, 0) && o.AccuracyMetres > 0 && o.AccuracyMetres <= 1000 && distance(o.Position, target) <= 5
}
func distance(a, b Position) float64 {
	const r = 6371000
	lat1, lat2 := a.Latitude*math.Pi/180, b.Latitude*math.Pi/180
	dlat, dlon := (b.Latitude-a.Latitude)*math.Pi/180, (b.Longitude-a.Longitude)*math.Pi/180
	v := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * r * math.Atan2(math.Sqrt(v), math.Sqrt(math.Max(0, 1-v)))
}
func (m *Manager) Observe(ctx context.Context) (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, e := m.readLocked(ctx)
	if e != nil {
		return r, e
	}
	if !r.ConfigurationVerified || r.RunningTrajectory || m.consume == nil {
		return r, ErrReadback
	}
	id, e := m.identity()
	if e != nil {
		return r, ErrUnsupported
	}
	s, e := m.open(id, storeReadOnly)
	if e != nil {
		return r, ErrStore
	}
	defer s.close()
	j, e := load(s, id.sid)
	if e != nil || j == nil || j.Expected == nil {
		return r, ErrStore
	}
	after, e := time.Parse(time.RFC3339Nano, j.WrittenAt)
	if e != nil {
		return r, ErrStore
	}
	requestAfter := m.now().UTC()
	if after.After(requestAfter) {
		return r, ErrReadback
	}
	run, cancel := context.WithCancel(ctx)
	m.observeMu.Lock()
	m.observeCancel = cancel
	m.observeMu.Unlock()
	defer func() { cancel(); m.observeMu.Lock(); m.observeCancel = nil; m.observeMu.Unlock() }()
	o, e := m.consume(run, ObserveRequest{requestAfter.Format(time.RFC3339Nano), 12})
	if e != nil || !validObservation(o, *j.Expected, requestAfter, m.now().UTC()) {
		return r, ErrReadback
	}
	// Observation is not enough if configuration changed while the consumer ran.
	on, e := m.client.Override(ctx)
	if e != nil || !on {
		return r, ErrReadback
	}
	p, e := m.client.Injected(ctx)
	if e != nil || p != *j.Expected {
		return r, ErrConflict
	}
	r.Consumer = &o
	r.ConsumerVerified = true
	r.Status = "consumer_verified"
	r.Detail = "独立 Windows 定位消费者返回新鲜 Default 来源且与本次位置匹配；其他应用仍需各自验证。"
	return r, nil
}

// Disconnect does not silently abandon an owned override. The UI must restore
// first; shutdown only stops movement and releases in-memory credentials.
func (m *Manager) CancelObservation() {
	m.observeMu.Lock()
	defer m.observeMu.Unlock()
	if m.observeCancel != nil {
		m.observeCancel()
	}
}
func (m *Manager) Close() {
	m.CancelObservation()
	m.Stop()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
	m.config = Config{}
}

package health

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/store"
)

// ProviderTelemetryEvent captures one completed provider attempt for health
// aggregation. It mirrors the spec telemetry event; sensitive request/response
// bodies are never included.
type ProviderTelemetryEvent struct {
	Timestamp         time.Time
	Provider          string
	ProviderAccountID string
	Model             string
	Capability        string

	Status string // "success" | "failed"
	// HTTPStatus is the upstream HTTP status code (0 when not applicable).
	HTTPStatus int
	LatencyMs  int
	TTFTMs     int

	InputTokens  int
	OutputTokens int
	CostMicroUSD int64

	ErrorType    ProviderErrorType
	ErrorMessage string

	ChainID            string
	FallbackTriggered  bool
	FallbackToProvider string
	FallbackToModel    string
	FinalFailure       bool
}

// LatencyThresholds maps a capability to its p95 latency threshold in ms.
type LatencyThresholds map[string]int

// DefaultLatencyThresholds returns the spec's default per-capability p95
// thresholds used when none are configured.
func DefaultLatencyThresholds() LatencyThresholds {
	return LatencyThresholds{
		"chat_completions": 10_000,
		"embeddings":       5_000,
		"image_generation": 60_000,
		"audio":            60_000,
		"search":           15_000,
	}
}

// ThresholdFor resolves the p95 latency threshold for a capability, falling
// back to the chat default when unknown.
func (lt LatencyThresholds) ThresholdFor(capability string) int {
	if v, ok := lt[capability]; ok && v > 0 {
		return v
	}
	if v, ok := lt["chat_completions"]; ok && v > 0 {
		return v
	}
	return 10_000
}

// Config controls the health telemetry service.
type Config struct {
	// Enabled gates telemetry recording. When false, Record is a no-op.
	Enabled bool
	// QueueSize is the async event channel capacity. Full queues drop events.
	QueueSize int
	// CurrentFlushInterval is how often provider_health_current is recomputed.
	CurrentFlushInterval time.Duration
	// SnapshotInterval is how often 1-minute snapshot rows are written.
	SnapshotInterval time.Duration
	// RollingWindow is the lookback for current-state aggregation.
	RollingWindow time.Duration
	// MaxHistoryWindow caps how long minute buckets are retained in memory
	// for dashboard range queries. Also bounds chain bucket growth.
	MaxHistoryWindow time.Duration
	// MaxSamplesPerBucket caps latency/TTFT samples kept per minute bucket.
	MaxSamplesPerBucket int
	// LatencyThresholds per capability (ms).
	LatencyThresholds LatencyThresholds
}

// Service records provider telemetry events asynchronously and aggregates them
// into provider_health_current (fast dashboard load) and
// provider_health_snapshots (historical charts).
//
// All recording is best-effort: a full queue or a DB write failure logs a
// warning and continues. The gateway request path never blocks on telemetry.
type Service struct {
	cfg  Config
	log  *slog.Logger
	repo *store.ProviderHealthRepo

	ch      chan ProviderTelemetryEvent
	once    sync.Once
	done    chan struct{}
	started bool

	mu      sync.Mutex // guards states + chains maps
	states  map[string]*keyState
	chains  map[string]*chainState // key = chain_id
	dropped atomic.Uint64
}

// chainState rolls up telemetry across all providers in one chain so the
// dashboard can show fallback rate and final-failure count per chain.
type chainState struct {
	chainID     string
	buckets     map[int64]*chainBucket // key = unix minute
	lastUpdated time.Time
}

type chainBucket struct {
	minute     time.Time
	requests   int64
	successes  int64
	failures   int64
	fallbacks  int64
	finalFails int64
}

type keyState struct {
	provider            string
	account             string
	model               string
	capability          string
	buckets             map[int64]*minuteBucket // key = unix minute
	consecutiveFailures int
	lastSuccess         *time.Time
	lastFailure         *time.Time
}

type minuteBucket struct {
	minute              time.Time
	revision            uint64
	snapshottedRevision uint64
	requests            int64
	successes           int64
	failures            int64
	fallbacks           int64
	finalFails          int64
	inputTokens         int64
	outputTokens        int64
	costMicros          int64
	latencies           []int
	ttfts               []int
	errCounts           map[ProviderErrorType]int64

	// Frozen percentile markers replace the raw sample arrays once a bucket
	// ages past the rolling window and has been persisted, bounding memory
	// for the longer retained history window.
	frozen                    bool
	latP50, latP95, latP99    int
	ttftP50, ttftP95, ttftP99 int
}

// New builds a health telemetry Service. The caller must call Start to launch
// the background aggregator and Close on shutdown.
func New(cfg Config, log *slog.Logger, repo *store.ProviderHealthRepo) *Service {
	cfg = normalizeConfig(cfg)
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		cfg:    cfg,
		log:    log,
		repo:   repo,
		ch:     make(chan ProviderTelemetryEvent, cfg.QueueSize),
		done:   make(chan struct{}),
		states: make(map[string]*keyState),
		chains: make(map[string]*chainState),
	}
}

// normalizeConfig fills zero-value defaults and enforces invariants so the rest
// of the service can assume a fully populated config.
func normalizeConfig(cfg Config) Config {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 5000
	}
	if cfg.CurrentFlushInterval <= 0 {
		cfg.CurrentFlushInterval = 30 * time.Second
	}
	if cfg.SnapshotInterval <= 0 {
		cfg.SnapshotInterval = 60 * time.Second
	}
	if cfg.RollingWindow <= 0 {
		cfg.RollingWindow = 15 * time.Minute
	}
	if cfg.MaxHistoryWindow <= 0 {
		cfg.MaxHistoryWindow = 24 * time.Hour
	}
	if cfg.MaxHistoryWindow < cfg.RollingWindow {
		cfg.MaxHistoryWindow = cfg.RollingWindow
	}
	if cfg.MaxSamplesPerBucket <= 0 {
		cfg.MaxSamplesPerBucket = 500
	}
	if cfg.LatencyThresholds == nil {
		cfg.LatencyThresholds = DefaultLatencyThresholds()
	}
	return cfg
}

// Record enqueues a telemetry event. Non-blocking: when the queue is full the
// event is dropped with a debug log so the request path is never delayed.
func (s *Service) Record(ev ProviderTelemetryEvent) {
	if s == nil || !s.cfg.Enabled {
		return
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	select {
	case s.ch <- ev:
	default:
		s.dropped.Add(1)
		s.log.Debug("health telemetry queue full; dropping event",
			"provider", ev.Provider, "model", ev.Model)
	}
}

// DroppedEvents reports telemetry events lost because the non-blocking queue
// was full. Surfacing this value lets dashboards qualify health coverage.
func (s *Service) DroppedEvents() uint64 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}

// RollingWindow reports the configured lookback represented by
// provider_health_current. Dashboards must expose this actual collector window
// instead of implying that an arbitrary requested range was applied.
func (s *Service) RollingWindow() time.Duration {
	if s == nil {
		return 0
	}
	return s.cfg.RollingWindow
}

// EffectiveWindow clamps a requested lookback to the minute buckets actually
// retained in memory, so responses can report the applied span instead of
// implying unbounded history. Non-positive requests resolve to the full
// retained window.
func (s *Service) EffectiveWindow(requested time.Duration) time.Duration {
	if s == nil {
		return 0
	}
	if requested <= 0 || requested > s.cfg.MaxHistoryWindow {
		return s.cfg.MaxHistoryWindow
	}
	return requested
}

// ChainStat is the rolled-up health of one routing chain over the rolling
// window, used by the chain-impact view.
type ChainStat struct {
	ChainID          string
	Requests         int64
	Successes        int64
	Failures         int64
	Fallbacks        int64
	FinalFailures    int64
	FallbackRate     float64 // 0-1
	FinalFailureRate float64 // 0-1
	LastUpdated      time.Time
}

// ProviderStat is one provider dimension key's rolled-up traffic over a
// lookback window, used by the dashboard's range-scoped views.
type ProviderStat struct {
	Provider          string
	ProviderAccountID string
	Model             string
	Capability        string
	Requests          int64
	Successes         int64
	Failures          int64
	Fallbacks         int64
	FinalFailures     int64
	LatencyP95Ms      int
	TTFTP95Ms         int
}

// ProviderStatsSince returns per-key traffic aggregates over the given
// lookback (clamped to the retained history window). Latency/TTFT p95 pools
// raw samples for minutes inside the rolling window and takes the max of
// frozen per-minute percentiles for older minutes. Keys with no traffic in the
// window are omitted.
func (s *Service) ProviderStatsSince(window time.Duration) []ProviderStat {
	if s == nil {
		return nil
	}
	windowStart := time.Now().UTC().Add(-s.EffectiveWindow(window)).Truncate(time.Minute)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProviderStat, 0, len(s.states))
	for _, ks := range s.states {
		if st, ok := providerStatFor(ks, windowStart); ok {
			out = append(out, st)
		}
	}
	return out
}

// providerStatFor aggregates one key's buckets at or after windowStart,
// reporting false when the key has no traffic in the window.
func providerStatFor(ks *keyState, windowStart time.Time) (ProviderStat, bool) {
	var st ProviderStat
	st.Provider = ks.provider
	st.ProviderAccountID = ks.account
	st.Model = ks.model
	st.Capability = ks.capability
	var latencies, ttfts []int
	var maxFrozenLat, maxFrozenTTFT int
	for _, b := range ks.buckets {
		if b.minute.Before(windowStart) {
			continue
		}
		st.Requests += b.requests
		st.Successes += b.successes
		st.Failures += b.failures
		st.Fallbacks += b.fallbacks
		st.FinalFailures += b.finalFails
		if b.frozen {
			maxFrozenLat = max(maxFrozenLat, b.latP95)
			maxFrozenTTFT = max(maxFrozenTTFT, b.ttftP95)
		} else {
			latencies = append(latencies, b.latencies...)
			ttfts = append(ttfts, b.ttfts...)
		}
	}
	st.LatencyP95Ms = max(percentileOf(latencies, 95), maxFrozenLat)
	st.TTFTP95Ms = max(percentileOf(ttfts, 95), maxFrozenTTFT)
	if st.Requests == 0 && st.Fallbacks == 0 {
		return st, false
	}
	return st, true
}

// ChainStats returns rolled-up per-chain stats over the rolling window.
func (s *Service) ChainStats() []ChainStat {
	if s == nil {
		return nil
	}
	return s.ChainStatsSince(s.cfg.RollingWindow)
}

// ChainStatsSince returns rolled-up per-chain stats over the given lookback
// (clamped to the retained history window). Chains with no telemetry in that
// window are omitted.
func (s *Service) ChainStatsSince(window time.Duration) []ChainStat {
	if s == nil {
		return nil
	}
	windowStart := time.Now().UTC().Add(-s.EffectiveWindow(window)).Truncate(time.Minute)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ChainStat, 0, len(s.chains))
	for id, cs := range s.chains {
		var agg ChainStat
		agg.ChainID = id
		for _, b := range cs.buckets {
			if b.minute.Before(windowStart) {
				continue
			}
			agg.Requests += b.requests
			agg.Successes += b.successes
			agg.Failures += b.failures
			agg.Fallbacks += b.fallbacks
			agg.FinalFailures += b.finalFails
		}
		if agg.Requests > 0 {
			agg.FallbackRate = float64(agg.Fallbacks) / float64(agg.Requests)
			agg.FinalFailureRate = float64(agg.FinalFailures) / float64(agg.Requests)
		}
		agg.LastUpdated = cs.lastUpdated
		if agg.Requests > 0 || agg.Fallbacks > 0 {
			out = append(out, agg)
		}
	}
	return out
}

// Start launches the drain + aggregator goroutine once.
func (s *Service) Start(ctx context.Context) {
	if s == nil || !s.cfg.Enabled {
		return
	}
	s.once.Do(func() {
		s.started = true
		go s.run(ctx)
	})
}

// Close stops the service and waits for the aggregator to exit. No-op when the
// service was never started or is disabled.
func (s *Service) Close(timeout time.Duration) {
	if s == nil {
		return
	}
	if !s.cfg.Enabled || !s.started {
		return
	}
	close(s.ch)
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-timer.C:
	}
}

func (s *Service) run(ctx context.Context) {
	defer close(s.done)
	currentTick := time.NewTicker(s.cfg.CurrentFlushInterval)
	snapTick := time.NewTicker(s.cfg.SnapshotInterval)
	defer currentTick.Stop()
	defer snapTick.Stop()

	for {
		select {
		case <-ctx.Done():
			s.flushCurrent(context.Background())
			s.flushSnapshots(context.Background(), true)
			return
		case ev, ok := <-s.ch:
			if !ok {
				s.flushCurrent(context.Background())
				s.flushSnapshots(context.Background(), true)
				return
			}
			s.ingest(ev)
		case <-currentTick.C:
			s.flushCurrent(context.Background())
		case <-snapTick.C:
			s.flushSnapshots(context.Background(), false)
		}
	}
}

func (s *Service) ingest(ev ProviderTelemetryEvent) {
	min := ev.Timestamp.UTC().Truncate(time.Minute)
	s.mu.Lock()
	defer s.mu.Unlock()

	if ev.FinalFailure {
		s.markFinalFailure(ev, min)
	} else if ev.Provider != "" {
		s.recordProviderAttempt(ev, min)
	}
	if ev.ChainID != "" {
		s.recordChainEvent(ev, min)
	}
}

// markFinalFailure folds a final-failure marker into the already-recorded
// provider attempt. The marker exists only to close the chain-level request:
// the concrete attempt was recorded immediately before it, and counting it
// again would double-count the failure and create a second account-less row.
// Caller holds s.mu.
func (s *Service) markFinalFailure(ev ProviderTelemetryEvent, min time.Time) {
	if ev.Provider == "" {
		return
	}
	key := store.HealthKey(ev.Provider, ev.ProviderAccountID, ev.Model, ev.Capability)
	ks, ok := s.states[key]
	if !ok {
		return
	}
	if bucket, ok := ks.buckets[min.Unix()]; ok {
		bucket.finalFails++
		bucket.revision++
	}
}

// recordProviderAttempt folds one provider attempt into its minute bucket,
// creating the key/bucket on first sight. Caller holds s.mu.
func (s *Service) recordProviderAttempt(ev ProviderTelemetryEvent, min time.Time) {
	key := store.HealthKey(ev.Provider, ev.ProviderAccountID, ev.Model, ev.Capability)
	ks := s.keyState(key, ev)
	pos := min.Unix()
	bucket, ok := ks.buckets[pos]
	if !ok {
		bucket = &minuteBucket{minute: min, errCounts: make(map[ProviderErrorType]int64)}
		ks.buckets[pos] = bucket
	}
	bucket.requests++
	bucket.inputTokens += int64(ev.InputTokens)
	bucket.outputTokens += int64(ev.OutputTokens)
	bucket.costMicros += ev.CostMicroUSD
	if ev.Status == "success" {
		bucket.successes++
		ks.consecutiveFailures = 0
		now := ev.Timestamp
		ks.lastSuccess = &now
	} else {
		bucket.failures++
		bucket.errCounts[ev.ErrorType]++
		ks.consecutiveFailures++
		now := ev.Timestamp
		ks.lastFailure = &now
	}
	if ev.FallbackTriggered && ev.Status == "failed" {
		bucket.fallbacks++
	}
	s.appendSamples(bucket, ev)
	bucket.revision++
}

// keyState returns the keyState for the event's health key, creating it when
// absent. Caller holds s.mu.
func (s *Service) keyState(key string, ev ProviderTelemetryEvent) *keyState {
	ks, ok := s.states[key]
	if ok {
		return ks
	}
	ks = &keyState{
		provider:   ev.Provider,
		account:    ev.ProviderAccountID,
		model:      ev.Model,
		capability: ev.Capability,
		buckets:    make(map[int64]*minuteBucket),
	}
	s.states[key] = ks
	return ks
}

// appendSamples records latency/TTFT samples for percentile pooling. Frozen
// buckets keep only their percentile markers; a late sample cannot extend the
// already-persisted snapshot anyway.
func (s *Service) appendSamples(bucket *minuteBucket, ev ProviderTelemetryEvent) {
	if bucket.frozen {
		return
	}
	if ev.LatencyMs > 0 && len(bucket.latencies) < s.cfg.MaxSamplesPerBucket {
		bucket.latencies = append(bucket.latencies, ev.LatencyMs)
	}
	if ev.TTFTMs > 0 && len(bucket.ttfts) < s.cfg.MaxSamplesPerBucket {
		bucket.ttfts = append(bucket.ttfts, ev.TTFTMs)
	}
}

// recordChainEvent rolls a terminal chain event into its minute bucket. Only
// terminal events (success or final failure) count as requests; non-terminal
// per-attempt failures are ignored here. Caller holds s.mu.
func (s *Service) recordChainEvent(ev ProviderTelemetryEvent, min time.Time) {
	cs, ok := s.chains[ev.ChainID]
	if !ok {
		cs = &chainState{chainID: ev.ChainID, buckets: make(map[int64]*chainBucket)}
		s.chains[ev.ChainID] = cs
	}
	pos := min.Unix()
	cb, ok := cs.buckets[pos]
	if !ok {
		cb = &chainBucket{minute: min}
		cs.buckets[pos] = cb
	}
	if ev.Status == "success" || ev.FinalFailure {
		cb.requests++
		if ev.Status == "success" {
			cb.successes++
		} else {
			cb.failures++
			cb.finalFails++
		}
		if ev.FallbackTriggered {
			cb.fallbacks++
		}
	}
	cs.lastUpdated = ev.Timestamp
}

// flushCurrent recomputes provider_health_current for every active key from its
// rolling window of minute buckets.
func (s *Service) flushCurrent(ctx context.Context) {
	if s.repo == nil {
		return
	}
	s.mu.Lock()
	now := time.Now().UTC()
	windowStart := now.Add(-s.cfg.RollingWindow).Truncate(time.Minute)
	keys := make([]*keyState, 0, len(s.states))
	for _, ks := range s.states {
		keys = append(keys, ks)
	}
	s.mu.Unlock()
	if err := s.repo.DeleteStaleTrafficCurrent(ctx, windowStart); err != nil {
		s.log.Warn("stale health current cleanup failed", "err", err)
	}

	for _, ks := range keys {
		agg := s.aggregate(ks, windowStart, now)
		if agg.requests == 0 {
			// Leave a concurrently refreshed probe-only row intact, but remove the
			// expired traffic aggregate so the API cannot label it as current.
			if err := s.repo.DeleteTrafficCurrent(ctx, ks.provider, ks.account, ks.model,
				ks.capability, now.Add(-time.Second)); err != nil {
				s.log.Warn("expired health current cleanup failed", "err", err,
					"provider", ks.provider, "model", ks.model)
			}
			continue
		}
		s.upsertCurrent(ctx, ks, agg, now)
	}
}

// flushSnapshots persists completed minute buckets while retaining them through
// the history window used for dashboard range queries (the current-state
// aggregation still reads only its shorter rolling window). A revision marker
// avoids rewriting unchanged buckets; late events make a retained bucket dirty
// and replace the same persisted snapshot on the next flush.
// snapshotWrite is one pending bucket persistence produced by
// collectSnapshotWrites and consumed by persistSnapshots.
type snapshotWrite struct {
	ks       *keyState
	minute   int64
	original *minuteBucket
	bucket   *minuteBucket
	revision uint64
}

func (s *Service) flushSnapshots(ctx context.Context, flushAll bool) {
	if s.repo == nil {
		return
	}
	s.mu.Lock()
	now := time.Now().UTC()
	historyStart := now.Add(-s.cfg.MaxHistoryWindow).Truncate(time.Minute)
	toWrite := s.collectSnapshotWrites(now, flushAll)
	s.mu.Unlock()

	s.persistSnapshots(ctx, toWrite, historyStart)
}

// collectSnapshotWrites selects dirty completed buckets to persist and
// freezes/prunes aged ones, returning the writes to perform. Caller holds s.mu.
func (s *Service) collectSnapshotWrites(now time.Time, flushAll bool) []snapshotWrite {
	currentMinute := now.Truncate(time.Minute).Unix()
	windowStart := now.Add(-s.cfg.RollingWindow).Truncate(time.Minute)
	historyStart := now.Add(-s.cfg.MaxHistoryWindow).Truncate(time.Minute)
	var toWrite []snapshotWrite
	for key, ks := range s.states {
		for m, b := range ks.buckets {
			completed := m < currentMinute || flushAll
			if completed && b.revision != b.snapshottedRevision {
				toWrite = append(toWrite, snapshotWrite{
					ks: ks, minute: m, original: b, bucket: cloneMinuteBucket(b), revision: b.revision,
				})
			}
			if b.minute.Before(windowStart) && b.revision == b.snapshottedRevision {
				// Persisted and past the rolling window: freeze raw samples
				// into percentile markers, then keep the counter-only bucket
				// until it ages out of the retained history window.
				s.freezeMinuteBucket(b)
				if b.minute.Before(historyStart) {
					delete(ks.buckets, m)
				}
			}
		}
		if len(ks.buckets) == 0 {
			// Everything this key saw is outside the history window and was
			// snapshotted before its buckets were dropped, so the in-memory
			// state can go; a future event recreates it fresh.
			delete(s.states, key)
		}
	}
	s.pruneChainBuckets(historyStart)
	return toWrite
}

// pruneChainBuckets drops counter-only chain buckets past the retained history
// window, then removes chains left without any retained bucket so chain
// memory stays bounded. Caller holds s.mu.
func (s *Service) pruneChainBuckets(historyStart time.Time) {
	for id, cs := range s.chains {
		for m, cb := range cs.buckets {
			if cb.minute.Before(historyStart) {
				delete(cs.buckets, m)
			}
		}
		if len(cs.buckets) == 0 {
			delete(s.chains, id)
		}
	}
}

// persistSnapshots writes cloned buckets and marks the exact revision persisted
// so that a newer revision is retried. Caller must NOT hold s.mu.
func (s *Service) persistSnapshots(ctx context.Context, toWrite []snapshotWrite, historyStart time.Time) {
	for _, p := range toWrite {
		if err := s.writeSnapshot(ctx, p.ks, p.bucket); err != nil {
			s.log.Warn("health snapshot write failed", "err", err,
				"provider", p.ks.provider, "model", p.ks.model)
			continue
		}
		s.mu.Lock()
		// A late event may have changed the bucket during the DB write. Mark only
		// the exact revision persisted so that a newer revision is retried.
		if cur, ok := p.ks.buckets[p.minute]; ok && cur == p.original && cur.revision == p.revision {
			cur.snapshottedRevision = p.revision
			if cur.minute.Before(historyStart) {
				delete(p.ks.buckets, p.minute)
			}
		}
		s.mu.Unlock()
	}
}

func cloneMinuteBucket(source *minuteBucket) *minuteBucket {
	clone := *source
	clone.latencies = append([]int(nil), source.latencies...)
	clone.ttfts = append([]int(nil), source.ttfts...)
	clone.errCounts = make(map[ProviderErrorType]int64, len(source.errCounts))
	for errorType, count := range source.errCounts {
		clone.errCounts[errorType] = count
	}
	return &clone
}

type windowAgg struct {
	requests, successes, failures int64
	fallbacks, finalFails         int64
	inputTokens, outputTokens     int64
	costMicros                    int64
	consecutiveFailures           int
	lastSuccess, lastFailure      *time.Time
	errCounts                     map[ProviderErrorType]int64
	latencies                     []int
	ttfts                         []int
}

func (s *Service) aggregate(ks *keyState, windowStart, _ time.Time) windowAgg {
	s.mu.Lock()
	defer s.mu.Unlock()
	var a windowAgg
	a.errCounts = make(map[ProviderErrorType]int64)
	for _, b := range ks.buckets {
		if b.minute.Before(windowStart) {
			continue
		}
		a.requests += b.requests
		a.successes += b.successes
		a.failures += b.failures
		a.fallbacks += b.fallbacks
		a.finalFails += b.finalFails
		a.inputTokens += b.inputTokens
		a.outputTokens += b.outputTokens
		a.costMicros += b.costMicros
		for t, c := range b.errCounts {
			a.errCounts[t] += c
		}
		a.latencies = append(a.latencies, b.latencies...)
		a.ttfts = append(a.ttfts, b.ttfts...)
	}
	a.consecutiveFailures = ks.consecutiveFailures
	a.lastSuccess = ks.lastSuccess
	a.lastFailure = ks.lastFailure
	return a
}

func (s *Service) upsertCurrent(ctx context.Context, ks *keyState, a windowAgg, now time.Time) {
	if s.repo == nil {
		return
	}
	successRate := 0.0
	if a.requests > 0 {
		successRate = float64(a.successes) / float64(a.requests)
	}
	errorRate := 0.0
	if a.requests > 0 {
		errorRate = float64(a.failures) / float64(a.requests)
	}
	dominant := dominantErrorType(a.errCounts)
	p95Lat := percentileOf(a.latencies, 95)
	p95TTFT := percentileOf(a.ttfts, 95)
	threshold := s.cfg.LatencyThresholds.ThresholdFor(ks.capability)

	score := ComputeScore(ScoreInput{
		SuccessRate:         successRate,
		P95LatencyMs:        p95Lat,
		LatencyThresholdMs:  threshold,
		DominantErrorType:   dominant,
		ConsecutiveFailures: a.consecutiveFailures,
	})
	status := StatusFromScore(score, a.requests > 0, false)
	mainIssue := MainIssue(dominant, p95Lat, threshold, a.fallbacks)
	var recommendation string
	if mainIssue != "" {
		recommendation = RecommendationForIssue(mainIssue)
	}

	cur := store.ProviderHealthCurrent{
		ID:                  uuid.NewString(),
		Provider:            ks.provider,
		ProviderAccountID:   ks.account,
		Model:               ks.model,
		Capability:          ks.capability,
		HealthStatus:        status,
		HealthScore:         score,
		SuccessRate:         successRate,
		ErrorRate:           errorRate,
		RequestCount:        a.requests,
		FallbackCount:       a.fallbacks,
		LatencyP95Ms:        intPtr(p95Lat),
		TTFTP95Ms:           intPtr(p95TTFT),
		ConsecutiveFailures: a.consecutiveFailures,
		LastSuccessAt:       a.lastSuccess,
		LastFailureAt:       a.lastFailure,
		LastUpdatedAt:       now,
	}
	if mainIssue != "" {
		cur.MainIssue = &mainIssue
	}
	if recommendation != "" {
		cur.Recommendation = &recommendation
	}
	if err := s.repo.UpsertCurrent(ctx, cur); err != nil {
		s.log.Warn("health current upsert failed", "err", err,
			"provider", ks.provider, "model", ks.model)
	}
}

func (s *Service) writeSnapshot(ctx context.Context, ks *keyState, b *minuteBucket) error {
	successRate := 0.0
	if b.requests > 0 {
		successRate = float64(b.successes) / float64(b.requests)
	}
	dominant := dominantErrorType(b.errCounts)
	p50Lat, p95Lat, p99Lat := bucketLatencyPercentiles(b)
	p50TTFT, p95TTFT, p99TTFT := bucketTTFTPercentiles(b)
	threshold := s.cfg.LatencyThresholds.ThresholdFor(ks.capability)

	score := ComputeScore(ScoreInput{
		SuccessRate:        successRate,
		P95LatencyMs:       p95Lat,
		LatencyThresholdMs: threshold,
		DominantErrorType:  dominant,
	})
	status := StatusFromScore(score, b.requests > 0, false)
	mainIssue := MainIssue(dominant, p95Lat, threshold, b.fallbacks)

	snap := store.ProviderHealthSnapshot{
		ID:                  uuid.NewString(),
		BucketStart:         b.minute,
		BucketSizeSeconds:   60,
		Provider:            ks.provider,
		ProviderAccountID:   ks.account,
		Model:               ks.model,
		Capability:          ks.capability,
		RequestCount:        b.requests,
		SuccessCount:        b.successes,
		FailureCount:        b.failures,
		FallbackCount:       b.fallbacks,
		FinalFailureCount:   b.finalFails,
		InputTokens:         b.inputTokens,
		OutputTokens:        b.outputTokens,
		EstimatedCostMicros: b.costMicros,
		LatencyP50Ms:        intPtr(p50Lat),
		LatencyP95Ms:        intPtr(p95Lat),
		LatencyP99Ms:        intPtr(p99Lat),
		TTFTP50Ms:           intPtr(p50TTFT),
		TTFTP95Ms:           intPtr(p95TTFT),
		TTFTP99Ms:           intPtr(p99TTFT),
		RateLimitedCount:    b.errCounts[ProviderErrorRateLimited],
		AuthErrorCount:      b.errCounts[ProviderErrorAuth],
		QuotaExceededCount:  b.errCounts[ProviderErrorQuotaExceeded],
		TimeoutCount:        b.errCounts[ProviderErrorTimeout],
		Provider5xxCount:    b.errCounts[ProviderErrorProvider5xx],
		BadRequestCount:     b.errCounts[ProviderErrorBadRequest],
		NetworkErrorCount:   b.errCounts[ProviderErrorNetwork],
		UnsupportedCount:    b.errCounts[ProviderErrorUnsupported],
		UnknownErrorCount:   b.errCounts[ProviderErrorUnknown],
		HealthScore:         score,
		HealthStatus:        status,
		CreatedAt:           time.Now(),
	}
	if mainIssue != "" {
		snap.MainIssue = &mainIssue
	}
	return s.repo.InsertSnapshot(ctx, snap)
}

// freezeMinuteBucket replaces raw latency/TTFT samples with their percentiles
// once a bucket has aged past the rolling window and been persisted. Called
// with s.mu held. Late events stop appending samples to frozen buckets.
func (s *Service) freezeMinuteBucket(b *minuteBucket) {
	if b.frozen {
		return
	}
	b.frozen = true
	b.latP50, b.latP95, b.latP99 = percentileOf(b.latencies, 50), percentileOf(b.latencies, 95), percentileOf(b.latencies, 99)
	b.ttftP50, b.ttftP95, b.ttftP99 = percentileOf(b.ttfts, 50), percentileOf(b.ttfts, 95), percentileOf(b.ttfts, 99)
	b.latencies = nil
	b.ttfts = nil
}

// bucketLatencyPercentiles resolves a bucket's latency percentiles, reading
// frozen markers once raw samples have been discarded.
func bucketLatencyPercentiles(b *minuteBucket) (p50, p95, p99 int) {
	if b.frozen {
		return b.latP50, b.latP95, b.latP99
	}
	return percentileOf(b.latencies, 50), percentileOf(b.latencies, 95), percentileOf(b.latencies, 99)
}

// bucketTTFTPercentiles resolves a bucket's TTFT percentiles, reading frozen
// markers once raw samples have been discarded.
func bucketTTFTPercentiles(b *minuteBucket) (p50, p95, p99 int) {
	if b.frozen {
		return b.ttftP50, b.ttftP95, b.ttftP99
	}
	return percentileOf(b.ttfts, 50), percentileOf(b.ttfts, 95), percentileOf(b.ttfts, 99)
}

func dominantErrorType(counts map[ProviderErrorType]int64) ProviderErrorType {
	var best ProviderErrorType
	var bestCount int64
	for t, c := range counts {
		if t == ProviderErrorNone {
			continue
		}
		if c > bestCount {
			best = t
			bestCount = c
		}
	}
	if bestCount == 0 {
		return ProviderErrorNone
	}
	return best
}

func percentileOf(samples []int, q float64) int {
	if len(samples) == 0 {
		return 0
	}
	return Percentile(SortInts(samples), q)
}

func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

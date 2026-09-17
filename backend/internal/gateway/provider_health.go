package gateway

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mydisha/keirouter/backend/internal/health"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// mountProviderHealth registers the actionable provider health dashboard API.
// These sit alongside the existing /health/accounts endpoints and reuse the
// same admin auth + loopback middleware.
func (s *Server) mountProviderHealth(r chi.Router) {
	r.Get("/health/overview", s.adminHealthOverview)
	r.Get("/health/providers/{provider}", s.adminHealthProviderDetail)
	r.Get("/health/models", s.adminHealthModels)
	r.Get("/health/chains", s.adminHealthChains)
	r.Get("/health/chains/{id}", s.adminHealthChainDetail)
	r.Get("/health/probes", s.adminHealthProbeHistory)
	r.Post("/health/probes/run", s.adminHealthRunProbe)
}

// parseRange resolves a ?range= duration (e.g. 1h, 24h) to a since time.
// Defaults to 1h. Caps at 30d to bound snapshot scans.
func parseRange(raw string) time.Time {
	d := parseRangeDuration(raw)
	if d <= 0 {
		d = time.Hour
	}
	if d > 30*24*time.Hour {
		d = 30 * 24 * time.Hour
	}
	return time.Now().Add(-d)
}

func parseRangeDuration(raw string) time.Duration {
	switch raw {
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h", "":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	return time.Hour
}

// healthProviderStatus rolls up live status signals for one provider from
// provider_health_current. Traffic metrics are kept separately because they
// follow the requested range, not the live rolling window.
type healthProviderStatus struct {
	provider       string
	status         string
	score          int
	accounts       map[string]struct{}
	models         map[string]struct{}
	lastProbe      *time.Time
	mainIssue      string
	recommendation string
}

// healthOverviewSummary is the summary-card block of /health/overview.
type healthOverviewSummary struct {
	Healthy               int64  `json:"healthy"`
	Degraded              int64  `json:"degraded"`
	Unhealthy             int64  `json:"unhealthy"`
	Unknown               int64  `json:"unknown"`
	Disabled              int64  `json:"disabled"`
	Fallbacks             int64  `json:"fallbacks"`
	AvgP95LatencyMs       int    `json:"avg_p95_latency_ms"`
	TelemetryDropped      uint64 `json:"telemetry_dropped"`
	TelemetryDroppedScope string `json:"telemetry_dropped_scope"`
}

// healthProviderEntry is one row in the overview's provider table.
type healthProviderEntry struct {
	Provider        string     `json:"provider"`
	Status          string     `json:"status"`
	Score           int        `json:"score"`
	Accounts        int        `json:"accounts"`
	ModelsMonitored int        `json:"models_monitored"`
	SuccessRate     float64    `json:"success_rate"`
	ErrorRate       float64    `json:"error_rate"`
	LatencyP95Ms    int        `json:"latency_p95_ms"`
	TTFTP95Ms       int        `json:"ttft_p95_ms"`
	FallbackCount   int64      `json:"fallback_count"`
	MainIssue       string     `json:"main_issue"`
	Recommendation  string     `json:"recommendation"`
	LastProbeAt     *time.Time `json:"last_probe_at,omitempty"`
}

// healthOverviewResponse is the typed /health/overview payload.
type healthOverviewResponse struct {
	Window    healthWindow          `json:"window"`
	Summary   healthOverviewSummary `json:"summary"`
	Providers []healthProviderEntry `json:"providers"`
}

// adminHealthOverview returns summary cards + a per-provider status table.
// Status/score/main issue are live signals from provider_health_current;
// traffic metrics cover the requested range from in-memory telemetry stats.
func (s *Server) adminHealthOverview(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "provider health not configured")
		return
	}
	statusFilter := r.URL.Query().Get("status")
	requestedRange := r.URL.Query().Get("range")

	rows, err := s.db.ProviderHealth().ListCurrent(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	metrics := s.metricsByProvider(rows, s.resolveHealthRange(requestedRange))

	// Aggregate account/model/capability keys into one truthful provider row.
	byProvider := map[string]*healthProviderStatus{}
	for _, current := range rows {
		agg, ok := byProvider[current.Provider]
		if !ok {
			agg = &healthProviderStatus{
				provider: current.Provider,
				accounts: map[string]struct{}{},
				models:   map[string]struct{}{},
				score:    current.HealthScore,
				status:   current.HealthStatus,
			}
			if current.MainIssue != nil {
				agg.mainIssue = *current.MainIssue
			}
			if current.Recommendation != nil {
				agg.recommendation = *current.Recommendation
			}
			byProvider[current.Provider] = agg
		} else {
			replaceIssue := rank(current.HealthStatus) > rank(agg.status) || current.HealthScore < agg.score
			if rank(current.HealthStatus) > rank(agg.status) {
				agg.status = current.HealthStatus
			}
			if current.HealthScore < agg.score {
				agg.score = current.HealthScore
			}
			if current.MainIssue != nil && *current.MainIssue != "" && (agg.mainIssue == "" || replaceIssue) {
				agg.mainIssue = *current.MainIssue
				if current.Recommendation != nil {
					agg.recommendation = *current.Recommendation
				}
			}
		}
		if current.ProviderAccountID != "" {
			agg.accounts[current.ProviderAccountID] = struct{}{}
		}
		if current.Model != "" {
			agg.models[current.Model] = struct{}{}
		}
		if current.LastProbeAt != nil && (agg.lastProbe == nil || current.LastProbeAt.After(*agg.lastProbe)) {
			agg.lastProbe = current.LastProbeAt
		}
	}

	providerIDs := make([]string, 0, len(byProvider))
	for provider := range byProvider {
		providerIDs = append(providerIDs, provider)
	}
	sort.Strings(providerIDs)

	summary := healthOverviewSummary{TelemetryDroppedScope: "process_lifetime"}
	if s.providerHealth != nil {
		summary.TelemetryDropped = s.providerHealth.DroppedEvents()
	}
	providers := make([]healthProviderEntry, 0, len(byProvider))
	var totalProviderP95 int
	var providerP95Count int
	for _, provider := range providerIDs {
		agg := byProvider[provider]
		switch agg.status {
		case health.StatusHealthy:
			summary.Healthy++
		case health.StatusDegraded:
			summary.Degraded++
		case health.StatusUnhealthy:
			summary.Unhealthy++
		case health.StatusUnknown:
			summary.Unknown++
		case health.StatusDisabled:
			summary.Disabled++
		}
		m := metrics[provider]
		summary.Fallbacks += m.fallbacks
		if m.latencyP95 > 0 {
			totalProviderP95 += m.latencyP95
			providerP95Count++
		}
		if statusFilter != "" && agg.status != statusFilter {
			continue
		}
		providers = append(providers, healthProviderEntry{
			Provider:        agg.provider,
			Status:          agg.status,
			Score:           agg.score,
			Accounts:        len(agg.accounts),
			ModelsMonitored: len(agg.models),
			SuccessRate:     m.successRate(),
			ErrorRate:       m.errorRate(),
			LatencyP95Ms:    m.latencyP95,
			TTFTP95Ms:       m.ttftP95,
			FallbackCount:   m.fallbacks,
			MainIssue:       agg.mainIssue,
			Recommendation:  agg.recommendation,
			LastProbeAt:     agg.lastProbe,
		})
	}

	if providerP95Count > 0 {
		summary.AvgP95LatencyMs = totalProviderP95 / providerP95Count
	}
	writeJSON(w, http.StatusOK, healthOverviewResponse{
		Window:    s.healthWindow(requestedRange),
		Summary:   summary,
		Providers: providers,
	})
}

// adminHealthProviderDetail returns detailed metrics + snapshots for one provider.
func (s *Server) adminHealthProviderDetail(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	since := parseRange(r.URL.Query().Get("range"))

	rows, err := s.db.ProviderHealth().ListCurrentByProvider(r.Context(), provider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "no health data for provider: "+provider)
		return
	}

	var score int = 100
	status := health.StatusHealthy
	var mainIssue, recommendation string
	errBreakdown := map[string]int64{}

	for _, c := range rows {
		if c.HealthScore < score {
			score = c.HealthScore
		}
		if rank(c.HealthStatus) > rank(status) {
			status = c.HealthStatus
		}
		if c.MainIssue != nil && *c.MainIssue != "" && mainIssue == "" {
			mainIssue = *c.MainIssue
		}
		if c.Recommendation != nil && *c.Recommendation != "" && recommendation == "" {
			recommendation = *c.Recommendation
		}
	}

	// Traffic metrics cover the requested range from in-memory telemetry
	// stats, falling back to the rolling current table when telemetry is off.
	metrics := s.metricsByProvider(rows, s.resolveHealthRange(r.URL.Query().Get("range")))[provider]

	snaps, _ := s.db.ProviderHealth().ListSnapshots(r.Context(), provider, "", "", "", since)
	for _, sn := range snaps {
		errBreakdown["rate_limited"] += sn.RateLimitedCount
		errBreakdown["auth_error"] += sn.AuthErrorCount
		errBreakdown["quota_exceeded"] += sn.QuotaExceededCount
		errBreakdown["timeout"] += sn.TimeoutCount
		errBreakdown["provider_5xx"] += sn.Provider5xxCount
		errBreakdown["bad_request"] += sn.BadRequestCount
		errBreakdown["network_error"] += sn.NetworkErrorCount
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider":       provider,
		"status":         status,
		"score":          score,
		"main_issue":     mainIssue,
		"recommendation": recommendation,
		"metrics": map[string]any{
			"requests":       metrics.requests,
			"success_rate":   metrics.successRate(),
			"error_rate":     metrics.errorRate(),
			"latency_p95_ms": metrics.latencyP95,
			"ttft_p95_ms":    metrics.ttftP95,
			"fallback_count": metrics.fallbacks,
		},
		"error_breakdown": errBreakdown,
		"models":          rows,
		"snapshots":       snaps,
	})
}

// healthModelEntry is one row in the models matrix.
type healthModelEntry struct {
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	Capability    string    `json:"capability"`
	Status        string    `json:"status"`
	Score         int       `json:"score"`
	SuccessRate   float64   `json:"success_rate"`
	ErrorRate     float64   `json:"error_rate"`
	FallbackCount int64     `json:"fallback_count"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
	LatencyP95Ms  int       `json:"latency_p95_ms,omitempty"`
	TTFTP95Ms     int       `json:"ttft_p95_ms,omitempty"`
	MainIssue     string    `json:"main_issue,omitempty"`
}

// healthModelsResponse is the typed /health/models payload.
type healthModelsResponse struct {
	Models []healthModelEntry `json:"models"`
}

// adminHealthModels returns the model-level health matrix. Status and score
// are live signals from provider_health_current; traffic metrics cover the
// requested range from in-memory telemetry stats.
func (s *Server) adminHealthModels(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	rows, err := s.db.ProviderHealth().ListCurrent(r.Context(), statusFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	metrics := s.metricsByModel(rows, s.resolveHealthRange(r.URL.Query().Get("range")))

	out := make([]healthModelEntry, 0, len(rows))
	for _, c := range rows {
		if c.Model == "" {
			continue
		}
		m := metrics[modelKey{c.Provider, c.Model}]
		entry := healthModelEntry{
			Provider:      c.Provider,
			Model:         c.Model,
			Capability:    c.Capability,
			Status:        c.HealthStatus,
			Score:         c.HealthScore,
			SuccessRate:   m.successRate(),
			ErrorRate:     m.errorRate(),
			FallbackCount: m.fallbacks,
			LastUpdatedAt: c.LastUpdatedAt,
			LatencyP95Ms:  m.latencyP95,
			TTFTP95Ms:     m.ttftP95,
		}
		if c.MainIssue != nil {
			entry.MainIssue = *c.MainIssue
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, healthModelsResponse{Models: out})
}

// ---- range resolution + window descriptor ----------------------------------

// resolveHealthRange maps a ?range= value to the in-memory lookback actually
// applied by the telemetry service, clamped to its retained history window.
// Returns zero when telemetry is disabled.
func (s *Server) resolveHealthRange(raw string) time.Duration {
	d := parseRangeDuration(raw)
	if d <= 0 {
		d = time.Hour
	}
	if s.providerHealth == nil {
		return 0
	}
	return s.providerHealth.EffectiveWindow(d)
}

// Window kinds describing how a response's aggregation span was sourced.
const (
	windowKindHistory     = "in_memory_history"
	windowKindUnavailable = "unavailable"
)

// healthWindow describes the aggregation span a response actually covers: the
// requested range clamped to the telemetry service's retained history.
type healthWindow struct {
	Kind            string     `json:"kind"`
	DurationSeconds int64      `json:"duration_seconds"`
	RequestedRange  string     `json:"requested_range"`
	GeneratedAt     time.Time  `json:"generated_at"`
	Since           *time.Time `json:"since,omitempty"`
}

func (s *Server) healthWindow(requestedRange string) healthWindow {
	generatedAt := time.Now().UTC()
	window := healthWindow{
		Kind:           windowKindUnavailable,
		RequestedRange: requestedRange,
		GeneratedAt:    generatedAt,
	}
	if s.providerHealth == nil {
		return window
	}
	applied := s.providerHealth.EffectiveWindow(parseRangeDuration(requestedRange))
	if applied <= 0 {
		return window
	}
	window.Kind = windowKindHistory
	window.DurationSeconds = int64(applied.Seconds())
	since := generatedAt.Add(-applied)
	window.Since = &since
	return window
}

// ---- shared traffic metrics -------------------------------------------------

// providerMetrics carries traffic metrics for one rollup (provider, model, or
// a single provider's detail view) regardless of whether they were sourced
// from in-memory range stats or the rolling current table.
type providerMetrics struct {
	requests, successes, failures, fallbacks int64
	latencyP95, ttftP95                      int
}

func (m providerMetrics) successRate() float64 {
	if m.requests <= 0 {
		return 0
	}
	return float64(m.successes) / float64(m.requests) * 100
}

func (m providerMetrics) errorRate() float64 {
	if m.requests <= 0 {
		return 0
	}
	return float64(m.failures) / float64(m.requests) * 100
}

// addRow folds one provider_health_current row into the metrics, weighting
// rates by the row's request count.
func (m *providerMetrics) addRow(c store.ProviderHealthCurrent) {
	m.requests += c.RequestCount
	m.successes += int64(float64(c.RequestCount) * c.SuccessRate)
	m.failures += int64(float64(c.RequestCount) * c.ErrorRate)
	m.fallbacks += c.FallbackCount
	if c.LatencyP95Ms != nil && *c.LatencyP95Ms > m.latencyP95 {
		m.latencyP95 = *c.LatencyP95Ms
	}
	if c.TTFTP95Ms != nil && *c.TTFTP95Ms > m.ttftP95 {
		m.ttftP95 = *c.TTFTP95Ms
	}
}

// addStat folds one in-memory range stat into the metrics.
func (m *providerMetrics) addStat(st health.ProviderStat) {
	m.requests += st.Requests
	m.successes += st.Successes
	m.failures += st.Failures
	m.fallbacks += st.Fallbacks
	if st.LatencyP95Ms > m.latencyP95 {
		m.latencyP95 = st.LatencyP95Ms
	}
	if st.TTFTP95Ms > m.ttftP95 {
		m.ttftP95 = st.TTFTP95Ms
	}
}

// modelKey identifies one provider+model pair in the models matrix.
type modelKey struct {
	provider string
	model    string
}

// metricsByProvider returns per-provider traffic metrics for the applied
// range. When telemetry is disabled it falls back to the rolling current
// table so the dashboard still shows approximate numbers.
func (s *Server) metricsByProvider(rows []store.ProviderHealthCurrent, applied time.Duration) map[string]providerMetrics {
	byProvider := map[string]providerMetrics{}
	if s.providerHealth != nil {
		for _, st := range s.providerHealth.ProviderStatsSince(applied) {
			m := byProvider[st.Provider]
			m.addStat(st)
			byProvider[st.Provider] = m
		}
		return byProvider
	}
	for _, c := range rows {
		m := byProvider[c.Provider]
		m.addRow(c)
		byProvider[c.Provider] = m
	}
	return byProvider
}

// metricsByModel returns per provider+model traffic metrics for the applied
// range, with the same current-table fallback as metricsByProvider.
func (s *Server) metricsByModel(rows []store.ProviderHealthCurrent, applied time.Duration) map[modelKey]providerMetrics {
	byModel := map[modelKey]providerMetrics{}
	if s.providerHealth != nil {
		for _, st := range s.providerHealth.ProviderStatsSince(applied) {
			key := modelKey{st.Provider, st.Model}
			m := byModel[key]
			m.addStat(st)
			byModel[key] = m
		}
		return byModel
	}
	for _, c := range rows {
		if c.Model == "" {
			continue
		}
		key := modelKey{c.Provider, c.Model}
		m := byModel[key]
		m.addRow(c)
		byModel[key] = m
	}
	return byModel
}

// chainStatsByChainID returns per-chain telemetry for the applied range,
// keyed by chain ID.
func (s *Server) chainStatsByChainID(applied time.Duration) map[string]health.ChainStat {
	byID := map[string]health.ChainStat{}
	if s.providerHealth == nil {
		return byID
	}
	for _, st := range s.providerHealth.ChainStatsSince(applied) {
		byID[st.ChainID] = st
	}
	return byID
}

// healthChainEntry is one row in the chains table.
type healthChainEntry struct {
	ChainID           string  `json:"chain_id"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	AffectedProvider  string  `json:"affected_provider"`
	AffectedModel     string  `json:"affected_model"`
	MainIssue         string  `json:"main_issue"`
	StepCount         int     `json:"step_count"`
	Requests          int64   `json:"requests"`
	FallbackRate      float64 `json:"fallback_rate"`
	FinalFailureCount int64   `json:"final_failure_count"`
	FallbackCount     int64   `json:"fallback_count"`
	Recommendation    string  `json:"recommendation"`
}

// healthChainsResponse is the typed /health/chains payload.
type healthChainsResponse struct {
	Chains []healthChainEntry `json:"chains"`
	Window healthWindow       `json:"window"`
}

// adminHealthChains returns chain health: fallback rate, final failures, and
// affected providers, derived from real-traffic telemetry joined with chain
// config + current provider health. Telemetry covers the requested range.
func (s *Server) adminHealthChains(w http.ResponseWriter, r *http.Request) {
	requestedRange := r.URL.Query().Get("range")
	chains, err := s.chains.ListByTenant(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	healthRows, _ := s.db.ProviderHealth().ListCurrent(r.Context(), "")
	chainStats := s.chainStatsByChainID(s.resolveHealthRange(requestedRange))

	out := make([]healthChainEntry, 0, len(chains))
	for _, c := range chains {
		worstStatus := health.StatusHealthy
		var affectedProvider, affectedModel, mainIssue string
		for _, step := range c.Steps {
			// Match any account/capability for this provider+model.
			for _, h := range healthRows {
				if h.Provider == step.Provider && h.Model == step.Model {
					if rank(h.HealthStatus) > rank(worstStatus) {
						worstStatus = h.HealthStatus
						affectedProvider = step.Provider
						affectedModel = step.Model
						if h.MainIssue != nil {
							mainIssue = *h.MainIssue
						}
					}
				}
			}
		}
		entry := healthChainEntry{
			ChainID:          c.ID,
			Name:             c.Name,
			Status:           worstStatus,
			AffectedProvider: affectedProvider,
			AffectedModel:    affectedModel,
			MainIssue:        mainIssue,
			StepCount:        len(c.Steps),
		}
		if st, ok := chainStats[c.ID]; ok {
			entry.Requests = st.Requests
			entry.FallbackRate = st.FallbackRate * 100
			entry.FinalFailureCount = st.FinalFailures
			entry.FallbackCount = st.Fallbacks
		}
		entry.Recommendation = chainRecommendation(worstStatus, mainIssue, affectedProvider, affectedModel)
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, healthChainsResponse{
		Chains: out,
		Window: s.healthWindow(requestedRange),
	})
}

// adminHealthChainDetail returns step-level health + usage for one chain.
func (s *Server) adminHealthChainDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.chains.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "chain not found")
		return
	}
	healthRows, _ := s.db.ProviderHealth().ListCurrent(r.Context(), "")
	chainStats := s.chainStatsByChainID(s.resolveHealthRange(r.URL.Query().Get("range")))

	steps := make([]map[string]any, 0, len(c.Steps))
	for _, step := range c.Steps {
		var status string = health.StatusUnknown
		var mainIssue string
		var score int = 100
		for _, h := range healthRows {
			if h.Provider == step.Provider && h.Model == step.Model {
				if rank(h.HealthStatus) > rank(status) || status == health.StatusUnknown {
					status = h.HealthStatus
					score = h.HealthScore
					if h.MainIssue != nil {
						mainIssue = *h.MainIssue
					}
				}
			}
		}
		steps = append(steps, map[string]any{
			"position":   step.Position,
			"provider":   step.Provider,
			"model":      step.Model,
			"status":     status,
			"score":      score,
			"main_issue": mainIssue,
		})
	}

	resp := map[string]any{
		"chain_id": c.ID,
		"name":     c.Name,
		"strategy": c.Strategy,
		"steps":    steps,
	}
	if st, ok := chainStats[c.ID]; ok {
		resp["requests"] = st.Requests
		resp["fallback_rate"] = st.FallbackRate * 100
		resp["final_failure_count"] = st.FinalFailures
		resp["fallback_count"] = st.Fallbacks
	}
	if c.FallbackProvider != "" {
		resp["fallback_provider"] = c.FallbackProvider
		resp["fallback_model"] = c.FallbackModel
	}
	resp["window"] = s.healthWindow(r.URL.Query().Get("range"))
	writeJSON(w, http.StatusOK, resp)
}

// chainRecommendation derives a next-step suggestion from a chain's worst
// provider status and the affected provider/model.
func chainRecommendation(status, mainIssue, provider, model string) string {
	switch status {
	case health.StatusUnhealthy, health.StatusDegraded:
		return "Move a healthy fallback provider above " + provider + "/" + model + " temporarily, or add capacity."
	case health.StatusUnknown:
		return "Run a manual probe on " + provider + "/" + model + " to populate health data."
	}
	_ = mainIssue
	return ""
}

// adminHealthProbeHistory returns paginated probe results.
func (s *Server) adminHealthProbeHistory(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	since := parseRange(r.URL.Query().Get("range"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if limit <= 0 {
		limit = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	items, total, err := s.db.ProviderHealth().ListProbeResults(r.Context(), provider, since, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		entry := map[string]any{
			"time":                p.CreatedAt,
			"provider":            p.Provider,
			"provider_account_id": p.ProviderAccountID,
			"model":               p.Model,
			"capability":          p.Capability,
			"status":              p.Status,
			"triggered_by":        p.TriggeredBy,
		}
		if p.HTTPStatus != nil {
			entry["http_status"] = *p.HTTPStatus
		}
		if p.LatencyMs != nil {
			entry["latency_ms"] = *p.LatencyMs
		}
		if p.TTFTMs != nil {
			entry["ttft_ms"] = *p.TTFTMs
		}
		if p.ErrorType != nil {
			entry["error_type"] = *p.ErrorType
		}
		if p.ErrorMessage != nil {
			entry["error_message"] = *p.ErrorMessage
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out,
		"pagination": map[string]int{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// adminHealthRunProbe triggers a manual synthetic probe.
func (s *Server) adminHealthRunProbe(w http.ResponseWriter, r *http.Request) {
	if s.probeRunner == nil {
		writeError(w, http.StatusServiceUnavailable, "probe runner not configured")
		return
	}
	var body struct {
		Provider          string `json:"provider"`
		ProviderAccountID string `json:"provider_account_id"`
		Model             string `json:"model"`
		Capability        string `json:"capability"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Provider == "" || body.Model == "" {
		writeError(w, http.StatusBadRequest, "provider and model are required")
		return
	}
	res, err := s.probeRunner.Run(r.Context(), health.ProbeRequest{
		Provider:          body.Provider,
		ProviderAccountID: body.ProviderAccountID,
		Model:             body.Model,
		Capability:        body.Capability,
		TriggeredBy:       "manual",
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := map[string]any{
		"status":   res.Status,
		"provider": res.Provider,
		"model":    res.Model,
		"message":  "Probe completed successfully.",
	}
	if res.HTTPStatus != nil {
		resp["http_status"] = *res.HTTPStatus
	}
	if res.LatencyMs != nil {
		resp["latency_ms"] = *res.LatencyMs
	}
	if res.TTFTMs != nil {
		resp["ttft_ms"] = *res.TTFTMs
	}
	if res.ErrorType != nil {
		resp["error_type"] = *res.ErrorType
	}
	if res.ErrorMessage != nil {
		resp["message"] = *res.ErrorMessage
	}
	writeJSON(w, http.StatusOK, resp)
}

// rank orders statuses so the worst one wins in a rollup.
func rank(status string) int {
	switch status {
	case health.StatusDisabled:
		return 5
	case health.StatusUnhealthy:
		return 4
	case health.StatusDegraded:
		return 3
	case health.StatusUnknown:
		return 2
	case health.StatusHealthy:
		return 1
	}
	return 0
}

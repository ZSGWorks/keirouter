package healthcheck

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/vault"
)

// ConnectorSource resolves provider connectors.
type ConnectorSource interface {
	Get(provider string) (core.Connector, error)
}

// CooldownSource reports dispatcher model cooldowns so probes skip parked
// targets instead of burning quota on them. Implemented by *store.RoutingRepo.
type CooldownSource interface {
	ActiveCooldowns(ctx context.Context, accountIDs []string, model string) (map[string]bool, error)
}

// Config controls the background health checker.
type Config struct {
	Enabled              bool
	Interval             time.Duration
	Timeout              time.Duration
	MaxParallel          int
	FailureThreshold     int
	SuccessThreshold     int
	RecentModelWindow    time.Duration
	MaxModelsPerProvider int
}

// Checker probes configured provider accounts and records account/model health.
type Checker struct {
	cfg      Config
	log      *slog.Logger
	accounts *store.AccountRepo
	health   *store.HealthRepo
	conns    ConnectorSource
	vault    *vault.Vault
	cools    CooldownSource
}

// New builds a Checker.
func New(cfg Config, log *slog.Logger, accounts *store.AccountRepo, health *store.HealthRepo, conns ConnectorSource, vault *vault.Vault) *Checker {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 8
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 1
	}
	if cfg.RecentModelWindow <= 0 {
		cfg.RecentModelWindow = 24 * time.Hour
	}
	if cfg.MaxModelsPerProvider <= 0 {
		cfg.MaxModelsPerProvider = 8
	}
	if log == nil {
		log = slog.Default()
	}
	return &Checker{cfg: cfg, log: log, accounts: accounts, health: health, conns: conns, vault: vault}
}

// SetCooldownSource attaches the dispatcher cooldown view so billable probes
// skip parked targets. Without it every probe runs as before.
func (c *Checker) SetCooldownSource(s CooldownSource) {
	if c != nil {
		c.cools = s
	}
}

// Run starts periodic probes until ctx is cancelled.
func (c *Checker) Run(ctx context.Context, tenantID string) {
	if c == nil || !c.cfg.Enabled {
		return
	}
	c.CheckOnce(ctx, tenantID, false)
	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.CheckOnce(ctx, tenantID, false)
		}
	}
}

// CheckOnce probes all enabled accounts for a tenant. Billable per-model
// probes skip targets under an active dispatcher cooldown (account or model
// level) so exhausted quotas are not hammered; skipped probes write nothing.
// Cheap "__all__" credential validations always run as an early recovery
// signal. force bypasses the cooldown skips for deliberate operator re-probes.
func (c *Checker) CheckOnce(ctx context.Context, tenantID string, force bool) {
	if c == nil || c.accounts == nil || c.health == nil || c.conns == nil || c.vault == nil {
		return
	}
	accounts, err := c.accounts.ListByTenant(ctx, tenantID)
	if err != nil {
		c.log.Warn("health check: list accounts failed", "err", err)
		return
	}
	now := time.Now()
	recent, _ := c.health.RecentAccountModels(ctx, tenantID, now.Add(-c.cfg.RecentModelWindow), len(accounts)*c.cfg.MaxModelsPerProvider)

	modelsByAccount, modelAccounts := groupProbeModels(recent, c.cfg.MaxModelsPerProvider)
	cooled := c.snapshotCooldowns(ctx, modelAccounts, force)

	sem := make(chan struct{}, c.cfg.MaxParallel)
	var wg sync.WaitGroup
	for _, acc := range accounts {
		if acc.Disabled || acc.NeedsReconnect {
			continue
		}
		models := modelsByAccount[acc.ID]
		if len(models) == 0 {
			models = []string{"__all__"}
		}
		for _, model := range models {
			if c.skipProbe(acc, model, force, now, cooled) {
				continue
			}
			acc := acc
			model := model
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}
				c.probe(ctx, acc, model)
			}()
		}
	}
	wg.Wait()
}

// groupProbeModels splits recent traffic into per-account probe lists plus a
// per-model account grouping, so each billable model costs one cooldown
// lookup. "__all__" entries only join the per-account lists: credential
// validations are cheap and always run.
func groupProbeModels(recent []store.AccountHealth, maxPerAccount int) (byAccount, byModel map[string][]string) {
	byAccount = map[string][]string{}
	byModel = map[string][]string{}
	for _, h := range recent {
		if len(byAccount[h.AccountID]) >= maxPerAccount {
			continue
		}
		byAccount[h.AccountID] = appendUnique(byAccount[h.AccountID], h.Model)
		if h.Model != "__all__" {
			byModel[h.Model] = append(byModel[h.Model], h.AccountID)
		}
	}
	return byAccount, byModel
}

// snapshotCooldowns reads active model cooldowns per probed model. Fail-open:
// a lookup error probes as before rather than silencing health.
func (c *Checker) snapshotCooldowns(ctx context.Context, modelAccounts map[string][]string, force bool) map[string]map[string]bool {
	cooled := map[string]map[string]bool{}
	if force || c.cools == nil {
		return cooled
	}
	for model, ids := range modelAccounts {
		set, err := c.cools.ActiveCooldowns(ctx, ids, model)
		if err != nil {
			c.log.Debug("health check: cooldown lookup failed, probing anyway", "model", model, "err", err)
			continue
		}
		cooled[model] = set
	}
	return cooled
}

// skipProbe reports whether a per-model probe must be skipped. Forced runs
// and cheap credential validations always proceed; cooled-down targets pause.
func (c *Checker) skipProbe(acc store.Account, model string, force bool, now time.Time, cooled map[string]map[string]bool) bool {
	if force || model == "__all__" {
		return false
	}
	return c.probeCooling(acc, model, now, cooled)
}

// probeCooling reports whether a billable per-model probe must be skipped:
// the account sits on a dispatcher cooldown, or the account+model pair does.
func (c *Checker) probeCooling(acc store.Account, model string, now time.Time, cooled map[string]map[string]bool) bool {
	if acc.CooldownUntil != nil && acc.CooldownUntil.After(now) {
		return true
	}
	return cooled[model][acc.ID]
}

func (c *Checker) probe(ctx context.Context, acc store.Account, model string) {
	conn, err := c.conns.Get(acc.Provider)
	if err != nil {
		c.record(ctx, acc, model, 0, err)
		return
	}
	creds, err := c.vault.Open(acc)
	if err != nil {
		c.record(ctx, acc, model, 0, err)
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	start := time.Now()
	if model == "__all__" {
		if v, ok := conn.(core.Validator); ok {
			err = v.Validate(probeCtx, creds)
		} else {
			err = nil
		}
	} else {
		max := 8
		req := &core.ChatRequest{
			Model: model,
			Messages: []core.Message{{
				Role: core.RoleUser,
				Content: []core.ContentPart{{
					Type: core.PartText,
					Text: "ping",
				}},
			}},
			MaxTokens: &max,
			Metadata: core.RequestMetadata{
				TenantID: acc.TenantID,
				Provider: acc.Provider,
			},
		}
		_, err = conn.Chat(probeCtx, req, creds)
	}
	c.record(ctx, acc, model, int(time.Since(start).Milliseconds()), err)
}

func (c *Checker) record(ctx context.Context, acc store.Account, model string, latencyMS int, probeErr error) {
	now := time.Now()
	prev, err := c.health.Get(ctx, acc.ID, model)
	if err != nil && err != store.ErrNotFound {
		c.log.Debug("health check: read previous state failed", "account", acc.ID, "model", model, "err", err)
	}
	h := prev
	if h.ID == "" {
		h.ID = uuid.NewString()
		h.TenantID = acc.TenantID
		h.AccountID = acc.ID
		h.Provider = acc.Provider
		h.Model = model
	}
	h.LatencyMS = latencyMS
	h.LastCheckedAt = now
	h.UpdatedAt = now
	if probeErr == nil {
		h.ConsecutiveSuccesses++
		h.ConsecutiveFailures = 0
		h.LastError = ""
		h.LastOKAt = &now
		if h.ConsecutiveSuccesses >= c.cfg.SuccessThreshold {
			h.Status = "healthy"
		} else if h.Status == "" {
			h.Status = "degraded"
		}
	} else {
		h.ConsecutiveFailures++
		h.ConsecutiveSuccesses = 0
		h.LastError = probeErr.Error()
		if h.ConsecutiveFailures >= c.cfg.FailureThreshold {
			h.Status = "unhealthy"
		} else {
			h.Status = "degraded"
		}
	}
	if err := c.health.Upsert(ctx, h); err != nil {
		c.log.Warn("health check: upsert failed", "account", acc.ID, "model", model, "err", err)
	}
}

func appendUnique(in []string, value string) []string {
	for _, existing := range in {
		if existing == value {
			return in
		}
	}
	return append(in, value)
}

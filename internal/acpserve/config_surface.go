package acpserve

// The 16-05 ConfigSurface (ACP-08 implementation half): the editor-driven
// configuration surface acpserve injects into the acp Server via
// WithConfigSurface. It owns the menu semantics the wire handler only relays:
//
//   - Menu construction from the REAL modelrouting layers (the locked D-06
//     enumeration: model, tier, permissions.mode, compaction-threshold plus
//     the `_global/` twins — A7's id-namespace scope decision).
//   - Effective currentValue resolution through the precedence chain (D-11):
//     project > global > embedded floor, plus the in-memory _meta blob overlay
//     with fills-unset semantics (D-10 — the blob is the default-of-last-
//     resort and is NEVER persisted).
//   - Set: validate (D-09 typed reject) → idempotence guard (a redundant
//     client default re-push never churns the operator's files nor promotes a
//     blob-derived value into persisted explicit config) → persist through
//     providerfactory.WriteLayerOption (atomic 0600, D-07) → live apply hook
//     → out-of-band config_option_update.
//
// Threat-surface notes (T-16-13/14/15): the writer is only reachable with a
// whitelisted option id mapped to a fixed key path — arbitrary config keys
// never reach WriteLayerOption; the menu carries no credential options; the
// blob channel never writes to disk.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
)

// Menu vocabulary (D-06 enumeration + A7 scope namespace + v1 categories).
const (
	optModel            = "model"
	optTier             = "tier"
	optPermissionsMode  = "permissions.mode"
	optCompactionThresh = "compaction-threshold"
	optGlobalPrefix     = "_global/"

	scopeGlobal  = "global"
	scopeProject = "project"

	permModeUngated = "ungated"
	permModeGated   = "gated"

	compactionOff      = "off"
	compactionDefault  = "80"
	compactionMidHigh  = "95"
	compactionMid      = "65"
	compactionMidLower = "50"

	categoryModel       = "model"
	categoryModelConfig = "model_config"
	categoryMode        = "mode"
	categoryCustom      = "_custom"

	keyTiers       = "tiers"
	keyModel       = "model"
	keySessionTier = "session_tier"

	phasePendingMode       = "Phase 17"
	phasePendingCompaction = "Phase 19"
)

// errNoGlobalLayer guards a global-scoped write when the global path could not
// be resolved at startup (the surface degrades to project-only).
var errNoGlobalLayer = errors.New("global config layer unavailable")

// ConfigSurface implements acp.ConfigSurface over the operator's two config
// layer files. All mutating operations (blob apply + Set) serialize through
// one mutex so the caller-serializes contract of WriteLayerOption holds
// against racing initialize traffic and interleaved sets (no torn YAML, no
// half-applied state).
type ConfigSurface struct {
	mu           sync.Mutex
	globalPath   string // "" = global layer unavailable (degrade to project-only)
	projectPath  string
	providerName string // the serve's session provider (the live-apply guard)
	stderr       io.Writer

	// notify emits the out-of-band config_option_update (wired by the Run
	// composition after the acp Server exists — the SetEmitter precedent).
	notify func(sessionID string, opts []acp.ConfigOptionFrame)

	// applyHook is the live-apply seam (Task 3 wires it to
	// runner.ApplyTurnModel). Called ONLY after a successful persist, and only
	// for models on the session's own provider.
	applyHook func(model string) error

	// blobRaw holds EVERY initialize _meta key verbatim (D-10 round-trip
	// survival: unknown keys are retained byte-identical, never executed).
	blobRaw map[string]json.RawMessage

	// blobFills holds recognized option values (keyed by bare option id) that
	// fill UNSET slots at resolution time — in-memory only, never persisted.
	blobFills map[string]string
}

// NewConfigSurface constructs the surface over explicit layer paths.
// Construction reads nothing and cannot fail; layer-load errors degrade loudly
// at use (advertisement falls back to the embedded floor, writes fail typed).
func NewConfigSurface(globalPath, projectPath, providerName string, stderr io.Writer) *ConfigSurface {
	return &ConfigSurface{
		globalPath:   globalPath,
		projectPath:  projectPath,
		providerName: providerName,
		stderr:       stderr,
		blobRaw:      map[string]json.RawMessage{},
		blobFills:    map[string]string{},
	}
}

// SetNotify wires the out-of-band config_option_update emitter (the Run
// composition calls this once the acp Server exists).
func (s *ConfigSurface) SetNotify(n func(sessionID string, opts []acp.ConfigOptionFrame)) {
	s.notify = n
}

// SetApplyHook wires the live-apply seam (the Run composition binds
// runner.ApplyTurnModel here).
func (s *ConfigSurface) SetApplyHook(h func(model string) error) {
	s.applyHook = h
}

// Options returns the full eight-entry menu in v1 SessionConfigOption shapes,
// every currentValue the option's current EFFECTIVE value (D-11).
func (s *ConfigSurface) Options() []acp.ConfigOptionFrame {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.optionsLocked()
}

// Set persists+applies one option (D-07 persist-then-apply) and returns the
// refreshed FULL set on every non-error outcome: an applied write, a pending
// no-op, or an idempotent re-push. See the package-level doc for the ordering
// and the D-09/D-10 guards.
//
// 16-REVIEW WR-03: the surface mutex spans only validation, persist, and the
// refreshed-frame computation. The live apply (blocks on every live session's
// turn mutex) and the out-of-band notify (a blocking send on the emitter lane)
// run OUTSIDE it — other config operations (including initialize's
// applyMetaBlob) never queue behind a slow client or an in-flight turn. The
// persist→apply→notify order for the responding caller is unchanged.
func (s *ConfigSurface) Set(sessionID, optionID string, value any) ([]acp.ConfigOptionFrame, error) {
	s.mu.Lock()

	outcome, serr := s.setLocked(optionID, value)

	notify, hook := s.notify, s.applyHook

	s.mu.Unlock()

	if serr != nil {
		return nil, serr
	}

	if outcome.doApply && hook != nil {
		herr := hook(outcome.applyModel)
		if herr != nil {
			s.logf("live apply of model %q failed (config persisted, live state unchanged): %v",
				outcome.applyModel, herr)
		}
	}

	// Only an APPLIED write emits out-of-band (the pending no-op and the
	// idempotent re-push answer the caller without a config_option_update —
	// the pre-WR-03 emit discipline, unchanged).
	if outcome.doNotify && notify != nil {
		notify(sessionID, outcome.frames)
	}

	return outcome.frames, nil
}

// ApplyBlobDefaults applies the initialize _meta object (D-10): every key is
// retained verbatim (unknown keys survive round-trip, never executed);
// recognized option keys fill UNSET slots in-memory. changed reports whether
// any effective value moved — the signal behind the out-of-band
// config_option_update carrying the full refreshed set.
//
// 16-REVIEW WR-03: the surface mutex spans only the retention/fill/snapshot
// work; the out-of-band notify (a blocking send on the emitter lane) runs
// OUTSIDE it on frames computed under the lock.
func (s *ConfigSurface) ApplyBlobDefaults(meta map[string]json.RawMessage) (bool, error) {
	s.mu.Lock()

	before, err := s.snapshotLocked()
	if err != nil {
		s.mu.Unlock()

		return false, fmt.Errorf("resolve config before blob application: %w", err)
	}

	for k, raw := range meta {
		s.blobRaw[k] = append(json.RawMessage(nil), raw...) // verbatim, byte-identical

		bare := strings.TrimPrefix(k, optGlobalPrefix)
		if !isMenuOption(bare) {
			continue // unknown: retained above, never executed
		}

		var str string

		uerr := json.Unmarshal(raw, &str)
		if uerr != nil || str == "" {
			continue // non-string blob values are tolerated but never fill a select
		}

		s.blobFills[bare] = str

		if isPendingOption(bare) {
			phase := phasePendingMode
			if bare == optCompactionThresh {
				phase = phasePendingCompaction
			}

			s.logf("option %q: blob default %q accepted as a pending-handler no-op (handler lands in %s, D-05)",
				k, str, phase)
		}
	}

	after, err := s.snapshotLocked()
	if err != nil {
		s.mu.Unlock()

		return false, fmt.Errorf("resolve config after blob application: %w", err)
	}

	if before == after {
		s.mu.Unlock()

		return false, nil // explicit config won in every slot — nothing moved
	}

	frames := s.optionsLocked()

	notify := s.notify

	s.mu.Unlock()

	// Out-of-band change: the full set follows application (sessionless at
	// initialize). Sent lock-free — see the WR-03 note above.
	if notify != nil {
		notify("", frames)
	}

	return true, nil
}

// setOutcome carries Set's lock-free tail inputs from setLocked: the refreshed
// frame set (always — every non-error outcome answers with the FULL set), the
// live-apply target + whether one applies, and whether the applied write emits
// the out-of-band update.
type setOutcome struct {
	frames     []acp.ConfigOptionFrame
	applyModel string
	doApply    bool
	doNotify   bool
}

// setLocked is Set's lock-holding half (callers hold s.mu): validate →
// idempotence guard → persist → drop the superseded blob fill → resolve the
// live-apply target → compute the refreshed frame set. The hook is NOT invoked
// here — the outcome hands the apply to the lock-free caller.
func (s *ConfigSurface) setLocked(optionID string, value any) (setOutcome, error) {
	parsed := splitScope(optionID)

	scope, bare := parsed.scope, parsed.bare

	if !isMenuOption(bare) {
		return setOutcome{}, &acp.ConfigViolationError{OptionID: optionID, Violation: "unknown option id"}
	}

	val, ok := value.(string)
	if !ok || val == "" {
		return setOutcome{}, &acp.ConfigViolationError{
			OptionID: optionID, Violation: "value must be a non-empty string option id",
		}
	}

	res, rerr := s.resolveLocked()
	if rerr != nil {
		return setOutcome{}, fmt.Errorf("resolve current config: %w", rerr)
	}

	if isPendingOption(bare) {
		frames, perr := s.setPendingLocked(optionID, scope, bare, val)

		return setOutcome{frames: frames}, perr
	}

	verr := s.validateSettableLocked(bare, optionID, val, res.cfg)
	if verr != nil {
		return setOutcome{}, verr
	}

	// Idempotence basis: the ADDRESSED layer's current value, not the combined
	// resolution (WR-05 gap closure — see the helper's contract).
	basis, berr := s.idempotenceBasisLocked(bare, scope, res)
	if berr != nil {
		return setOutcome{}, berr
	}

	if val == basis.value {
		// D-10 guard on the set channel: a redundant client default (Zed
		// re-pushes its stored defaults every connection) must not churn the
		// operator's file — nor promote a blob-derived effective value into
		// persisted explicit config. One structured line, refreshed set, done.
		s.logf("option %q: value %q equals %s — idempotent re-push, no layer write (D-10)",
			optionID, val, basis.where)

		return setOutcome{frames: s.optionsLocked()}, nil
	}

	perr := s.persistLocked(scope, bare, optionID, res.tier, val)
	if perr != nil {
		return setOutcome{}, perr
	}

	// An explicit editor write supersedes any blob fill for this option (the
	// fill would be inert anyway — explicit wins — dropping it keeps the
	// overlay honest without promoting the value anywhere).
	delete(s.blobFills, bare)

	applyModel, doApply := s.applyTargetLocked(bare, val, res.cfg)

	return setOutcome{
		frames: s.optionsLocked(), applyModel: applyModel, doApply: doApply, doNotify: true,
	}, nil
}

// --- resolution (callers hold s.mu) ---

// effectiveState is the comparable snapshot behind blob-application change
// detection.
type effectiveState struct {
	tier, model, permMode, compaction string
}

func (s *ConfigSurface) snapshotLocked() (effectiveState, error) {
	res, err := s.resolveLocked()
	if err != nil {
		return effectiveState{}, err
	}

	return effectiveState{
		tier:       res.tier,
		model:      res.model,
		permMode:   s.pendingCurrentLocked(optPermissionsMode),
		compaction: s.pendingCurrentLocked(optCompactionThresh),
	}, nil
}

// resolvedConfig is the effective (tier, model) pair over its config.
type resolvedConfig struct {
	tier  string
	model string
	cfg   *modelrouting.Config
}

// resolveLocked computes the effective (tier, model) pair: files through
// modelrouting.Load (project > global > embedded floor, time-windows inside
// the resolver), then the blob overlay fills any slot no LAYER file sets
// explicitly (D-10 fills-unset; the embedded floor is not operator config).
func (s *ConfigSurface) resolveLocked() (*resolvedConfig, error) {
	cfg, err := modelrouting.Load(s.layerPaths()...)
	if err != nil {
		return nil, fmt.Errorf("load config layers: %w", err)
	}

	tier := cfg.SessionTier
	if fill, ok := s.blobFills[optTier]; ok && !s.explicitInLayers(keySessionTier) {
		tier = fill
	}

	model := s.resolveModelLocked(cfg, tier)
	if fill, ok := s.blobFills[optModel]; ok && !s.explicitInLayers(keyTiers, tier, keyModel) {
		model = fill
	}

	return &resolvedConfig{tier: tier, model: model, cfg: cfg}, nil
}

// idempotenceBasis is the addressed-layer value a set is compared against for
// the idempotence guard, with the basis's human name for the log line.
type idempotenceBasis struct {
	value string
	where string
}

// idempotenceBasisLocked returns the value a set is compared against for the
// idempotence guard. The basis is the ADDRESSED layer's current value (WR-05
// gap closure): the project (default) scope keeps the combined comparison —
// D-10's anti-promotion guard on the Zed re-push scope; the global scope
// compares against the global layer ALONE (D-08: the layers are independently
// addressable write targets), so a legitimate global mutation is never
// classified as redundant and silently dropped.
func (s *ConfigSurface) idempotenceBasisLocked(
	bare, scope string, res *resolvedConfig,
) (idempotenceBasis, error) {
	if scope != scopeGlobal {
		return idempotenceBasis{
			value: s.effectiveFor(bare, res.tier, res.model),
			where: "the currently-effective value",
		}, nil
	}

	g, gerr := s.globalOnlyResolvedLocked()
	if gerr != nil {
		return idempotenceBasis{}, fmt.Errorf("resolve global config layer: %w", gerr)
	}

	return idempotenceBasis{
		value: s.effectiveFor(bare, g.tier, g.model),
		where: "the global layer's current value",
	}, nil
}

// globalOnlyResolvedLocked resolves the GLOBAL layer alone: the layer file
// through modelrouting.Load when it exists, the embedded floor when the path
// is empty or the file is absent. Tier/model resolve exactly like resolveLocked
// but WITHOUT the project layer and WITHOUT the blob overlay (D-10 fills are
// in-memory defaults for the combined view, never layer truth) — the
// addressed-layer basis behind scope-aware idempotence and the _global twins.
func (s *ConfigSurface) globalOnlyResolvedLocked() (*resolvedConfig, error) {
	var paths []string

	if s.globalPath != "" {
		_, serr := os.Stat(s.globalPath)
		if serr == nil {
			paths = append(paths, s.globalPath)
		}
	}

	cfg, err := modelrouting.Load(paths...)
	if err != nil {
		return nil, fmt.Errorf("load global config layer: %w", err)
	}

	return &resolvedConfig{
		tier:  cfg.SessionTier,
		model: s.resolveModelLocked(cfg, cfg.SessionTier),
		cfg:   cfg,
	}, nil
}

// resolveModelLocked resolves one tier's model through the resolver (the
// time-window substitution lives inside modelrouting), falling back to the
// tier's static binding when the resolver declines.
func (s *ConfigSurface) resolveModelLocked(cfg *modelrouting.Config, tier string) string {
	tgt, _, rerr := modelrouting.NewResolver(cfg).Resolve(tier, "", time.Now(), modelrouting.CapabilityReq{})
	if rerr == nil && tgt.Model != "" {
		return tgt.Model
	}

	if b, ok := cfg.Tiers[tier]; ok {
		return b.Model
	}

	return ""
}

// pendingCurrentLocked resolves a pending option's advertised value: the blob
// fill when present (nothing else can set it), else the fixed default.
func (s *ConfigSurface) pendingCurrentLocked(bare string) string {
	if fill, ok := s.blobFills[bare]; ok {
		return fill
	}

	if bare == optCompactionThresh {
		return compactionDefault
	}

	return permModeUngated
}

// --- mutation helpers (callers hold s.mu) ---

// setPendingLocked handles an advertised-but-unhandled id: validate the value
// (never accept garbage into a pending slot), log one structured line, persist
// nothing, return the set unchanged (D-05).
func (s *ConfigSurface) setPendingLocked(
	optionID, scope, bare, val string,
) ([]acp.ConfigOptionFrame, error) {
	if !slices.Contains(pendingValues(bare), val) {
		return nil, &acp.ConfigViolationError{
			OptionID:  optionID,
			Violation: fmt.Sprintf("value %q is not one of the offered options", val),
		}
	}

	phase := phasePendingMode
	if bare == optCompactionThresh {
		phase = phasePendingCompaction
	}

	s.logf("option %q (scope %s): pending handler (lands in %s) — value %q accepted as a logged no-op (D-05)",
		optionID, scope, phase, val)

	return s.optionsLocked(), nil
}

// validateSettableLocked applies the D-09 menu membership check for the
// day-1-handled ids.
func (s *ConfigSurface) validateSettableLocked(
	bare, optionID, val string, cfg *modelrouting.Config,
) error {
	switch bare {
	case optModel:
		if !slices.Contains(sortedConfigKeys(cfg.Models), val) {
			return &acp.ConfigViolationError{
				OptionID:  optionID,
				Violation: fmt.Sprintf("value %q is not a declared model", val),
			}
		}
	case optTier:
		if !slices.Contains(sortedConfigKeys(cfg.Tiers), val) {
			return &acp.ConfigViolationError{
				OptionID:  optionID,
				Violation: fmt.Sprintf("value %q is not a declared tier", val),
			}
		}
	}

	return nil
}

func (s *ConfigSurface) effectiveFor(bare, tier, model string) string {
	if bare == optTier {
		return tier
	}

	return model
}

// persistLocked routes one validated write to its addressed layer: the
// `configId` prefix selects the global layer (D-08), the default target is the
// project layer. Model writes go through the CURRENT tier's binding
// (tiers.<tier>.model) — editor writes are simply another writer into the
// operator's layers (D-12).
func (s *ConfigSurface) persistLocked(scope, bare, optionID, tier, val string) error {
	layerPath, err := s.layerForScope(scope)
	if err != nil {
		return &acp.ConfigPersistError{OptionID: optionID, Err: err}
	}

	keyPath := []string{keyTiers, tier, keyModel}
	if bare == optTier {
		keyPath = []string{keySessionTier}
	}

	werr := providerfactory.WriteLayerOption(layerPath, keyPath, val)
	if werr != nil {
		return &acp.ConfigPersistError{OptionID: optionID, Err: werr}
	}

	return nil
}

// applyTargetLocked resolves the live-apply target for a successful persist
// (callers hold s.mu): the model to apply is the written value (model option)
// or the newly-selected tier's resolved model (tier option). ok=false (with the
// skip reason logged) marks the degrade cases — an undeclared model, or a
// target bound to a DIFFERENT provider (cross-provider switches degrade loudly,
// they never rewire the live provider — the resolveSubagentModel precedent).
// The CALLER invokes the apply hook OUTSIDE s.mu (16-REVIEW WR-03: the hook
// blocks on every live session's turn mutex — the surface lock must never
// serialize other config operations behind it).
func (s *ConfigSurface) applyTargetLocked(bare, val string, cfg *modelrouting.Config) (string, bool) {
	target := val
	if bare == optTier {
		target = s.resolveModelLocked(cfg, val)
	}

	mc, ok := cfg.Models[target]
	if !ok {
		s.logf("live apply skipped: model %q is not declared", target)

		return "", false
	}

	if mc.Provider != s.providerName {
		s.logf("model %q is bound to provider %q but the session provider is %q — live apply SKIPPED "+
			"(model unchanged; cross-provider switches degrade loudly, they do not rewire the live provider)",
			target, mc.Provider, s.providerName)

		return "", false
	}

	return target, true
}

// --- advertisement (callers hold s.mu) ---

// floorResolvedLocked resolves the embedded floor alone (no layer files, no
// blob overlay) — the degradation target when a layer file cannot be loaded.
func (s *ConfigSurface) floorResolvedLocked() (*resolvedConfig, error) {
	cfg, err := modelrouting.Load()
	if err != nil {
		return nil, fmt.Errorf("load embedded floor: %w", err)
	}

	return &resolvedConfig{
		tier:  cfg.SessionTier,
		model: s.resolveModelLocked(cfg, cfg.SessionTier),
		cfg:   cfg,
	}, nil
}

// optionsLocked builds the eight-entry menu. A layer-load failure degrades to
// the embedded floor (loudly); only a floor failure leaves the advertisement
// empty.
func (s *ConfigSurface) optionsLocked() []acp.ConfigOptionFrame {
	res, err := s.resolveLocked()
	if err != nil {
		s.logf("layer load failed during advertisement (falling back to the embedded floor): %v", err)

		f, ferr := s.floorResolvedLocked()
		if ferr != nil {
			s.logf("advertisement unavailable (embedded floor failed: %v)", ferr)

			return nil
		}

		res = f
	}

	// The _global twins describe the GLOBAL LAYER alone (WR-05 gap 4a): their
	// current values come from the global layer's own resolution — never the
	// project-won combined view, never the blob overlay (the twins describe a
	// layer FILE; the blob channel stays on the bare options). An unresolvable
	// global layer degrades loudly to the embedded floor, then to the combined
	// view as the last resort so the menu stays whole.
	gRes, gerr := s.globalOnlyResolvedLocked()
	if gerr != nil {
		s.logf("global layer load failed during advertisement (twins fall back to the embedded floor): %v", gerr)

		gRes, gerr = s.floorResolvedLocked()
		if gerr != nil {
			s.logf("embedded floor failed for the _global twins (%v); keeping the combined values", gerr)

			gRes = res
		}
	}

	models := sortedConfigKeys(res.cfg.Models)
	tiers := sortedConfigKeys(res.cfg.Tiers)

	build := func(id, name, desc, category, current string, values []string) acp.ConfigOptionFrame {
		opts := make([]acp.ConfigOptionValue, 0, len(values))
		for _, v := range values {
			opts = append(opts, acp.ConfigOptionValue{Value: v, Name: v})
		}

		return acp.ConfigOptionFrame{
			ID: id, Name: name, Description: desc, Category: category,
			Type: acp.ConfigOptionTypeSelect, CurrentValue: current, Options: opts,
		}
	}

	return []acp.ConfigOptionFrame{
		build(optModel, "Model", "Model the agent sends requests to", categoryModel, res.model, models),
		build(optTier, "Session tier", "Model scheduling tier", categoryModelConfig, res.tier, tiers),
		build(optPermissionsMode, "Permission mode", "Tool permission gating (phase "+phasePendingMode+")",
			categoryMode, s.pendingCurrentLocked(optPermissionsMode), pendingValues(optPermissionsMode)),
		build(optCompactionThresh, "Compaction threshold",
			"Context compaction trigger (phase "+phasePendingCompaction+")",
			categoryCustom, s.pendingCurrentLocked(optCompactionThresh), pendingValues(optCompactionThresh)),
		build(optGlobalPrefix+optModel, "Model (global default)", "Model default in the global config layer",
			categoryModel, gRes.model, models),
		build(optGlobalPrefix+optTier, "Session tier (global default)", "Tier default in the global config layer",
			categoryModelConfig, gRes.tier, tiers),
		build(optGlobalPrefix+optPermissionsMode, "Permission mode (global default)",
			"Global permission gating default",
			categoryMode, s.pendingCurrentLocked(optPermissionsMode), pendingValues(optPermissionsMode)),
		build(optGlobalPrefix+optCompactionThresh, "Compaction threshold (global default)", "Global compaction default",
			categoryCustom, s.pendingCurrentLocked(optCompactionThresh), pendingValues(optCompactionThresh)),
	}
}

// --- files & vocabulary ---

// layerPaths returns the existing layer files in overlay order (global then
// project) for modelrouting.Load; the embedded floor always applies.
func (s *ConfigSurface) layerPaths() []string {
	var paths []string

	for _, p := range []string{s.globalPath, s.projectPath} {
		if p == "" {
			continue
		}

		_, serr := os.Stat(p)
		if serr == nil {
			paths = append(paths, p)
		}
	}

	return paths
}

// explicitInLayers reports whether ANY layer file sets the key path explicitly
// — the D-10 explicitness boundary: the blob fills slots no OPERATOR file
// sets; the embedded floor is not operator config.
func (s *ConfigSurface) explicitInLayers(keyPath ...string) bool {
	for _, p := range s.layerPaths() {
		m, err := readLayerMap(p)
		if err != nil {
			continue // unreadable layer: Load fails loudly elsewhere; treat as not-explicit here
		}

		if mapHasPath(m, keyPath...) {
			return true
		}
	}

	return false
}

func (s *ConfigSurface) layerForScope(scope string) (string, error) {
	if scope == scopeGlobal {
		if s.globalPath == "" {
			return "", errNoGlobalLayer
		}

		return s.globalPath, nil
	}

	return s.projectPath, nil
}

func (s *ConfigSurface) logf(format string, args ...any) {
	if s.stderr == nil {
		return
	}

	_, _ = fmt.Fprintf(s.stderr, "ass-guard/acpserve/config: "+format+"\n", args...)
}

// splitScope parses A7's `_global/` id-namespace: the prefix addresses the
// global layer, everything else targets the project layer (D-08's default).
// optionScope is a parsed option id: its addressed layer scope and bare menu id.
type optionScope struct {
	scope string
	bare  string
}

func splitScope(optionID string) optionScope {
	if bare, found := strings.CutPrefix(optionID, optGlobalPrefix); found {
		return optionScope{scope: scopeGlobal, bare: bare}
	}

	return optionScope{scope: scopeProject, bare: optionID}
}

func isMenuOption(bare string) bool {
	return bare == optModel || bare == optTier || bare == optPermissionsMode || bare == optCompactionThresh
}

// isPendingOption reports the advertised-but-unhandled ids (D-05): accepted
// and logged, never persisted — real handlers land in Phases 17/19.
func isPendingOption(bare string) bool {
	return bare == optPermissionsMode || bare == optCompactionThresh
}

// pendingValues is the fixed offered set of the pending options.
func pendingValues(bare string) []string {
	if bare == optPermissionsMode {
		return []string{permModeUngated, permModeGated}
	}

	return []string{compactionOff, compactionMidLower, compactionMid, compactionDefault, compactionMidHigh}
}

// readLayerMap parses one layer file as a generic map (the writer's read
// discipline).
func readLayerMap(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	m := make(map[string]any)

	uerr := yaml.Unmarshal(raw, &m)
	if uerr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, uerr)
	}

	return m, nil
}

// mapHasPath reports whether the generic map carries the nested key path.
func mapHasPath(m map[string]any, keyPath ...string) bool {
	cur := m

	for i, k := range keyPath {
		v, ok := cur[k]
		if !ok {
			return false
		}

		if i == len(keyPath)-1 {
			return true
		}

		next, ok := v.(map[string]any)
		if !ok {
			return false
		}

		cur = next
	}

	return false
}

// sortedConfigKeys returns a config map's keys in sorted order (deterministic
// menus and stable violation messages).
func sortedConfigKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	return keys
}

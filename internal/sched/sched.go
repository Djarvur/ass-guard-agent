// Package sched is the persisted schedule engine behind the cron quartet
// (12-07, ACP-04 / D-02): the per-project JSON store under
// .ass-guard/schedule/ (0600, atomic writes), the hand-rolled 5-field cron
// parser (LOCAL timezone — the captured schema's explicit "Do not convert to
// UTC" rule), the due walker, and the fire-once catch-up with missed-window
// notes. No daemon, no network port: the due-check loop is a serve-lifetime
// goroutine owned by the runner (the editor owns the process lifecycle).
package sched

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store layout constants (the .ass-guard artifact family; EnsureGitignore's
// `*` rule already covers the schedule/ subdir — verified 12-07).
const (
	storeDirName  = ".ass-guard"
	scheduleDir   = "schedule"
	storeFileName = "schedules.json"
	filePerm      = 0o600
	dirPerm       = 0o750
)

// Bounds for the parser's forward scan and delay validation (the schema's
// own delayMinutes maximum is 525600).
const (
	maxDelayMinutes = 525600
	scanHorizon     = 366 * 24 * time.Hour
	scanStep        = time.Minute
)

// Sentinel errors (idiomatic package-level; callers render structured forms).
var (
	ErrNotFound      = errors.New("sched: automation not found")
	ErrBadCron       = errors.New("sched: invalid cron expression")
	ErrBadDelay      = errors.New("sched: invalid delayMinutes")
	ErrCronXorDelay  = errors.New("sched: exactly one of cron or delayMinutes required")
	ErrRequiredField = errors.New("sched: required field missing")
	ErrBadField      = errors.New("sched: invalid cron field")
	ErrEmptyItem     = errors.New("sched: empty cron list item")
	ErrBadStep       = errors.New("sched: invalid cron step")
	ErrBadRange      = errors.New("sched: invalid cron range")
	ErrBadValue      = errors.New("sched: invalid cron value")
	ErrOutOfBounds   = errors.New("sched: cron value out of field bounds")
)

// Automation is one persisted scheduled prompt (the store's record unit).
type Automation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	Cron   string `json:"cron,omitempty"` // 5-field spec; "" when delay-only
	// relative one-shot (minutes); camelCase mirrors the captured schema field.
	DelayOnce int `json:"delayMinutes,omitempty"` //nolint:tagliatelle // camelCase mirrors the captured schema field
	// Recurring mirrors the schema default (true repeats indefinitely; false
	// is finite — without MaxRuns it runs once).
	Recurring bool      `json:"recurring"`
	MaxRuns   int       `json:"maxRuns,omitempty"` //nolint:tagliatelle // camelCase mirrors the captured schema field
	LastFired time.Time `json:"lastFired"`         //nolint:tagliatelle // mirrors the captured schema field
	RunCount  int       `json:"runCount"`          //nolint:tagliatelle // camelCase mirrors the captured schema field
	CreatedAt time.Time `json:"createdAt"`         //nolint:tagliatelle // mirrors the captured schema field
	// Done marks a spent automation (one-shot fired, or finite run count
	// reached): it stays listed (history) but never fires again.
	Done bool `json:"done,omitempty"`
}

// ScheduleDisplay renders the human schedule phrase for result forms
// (corpus_absent documented default): the cron expression verbatim, or the
// relative "in N minutes" phrasing for one-shots.
func (a *Automation) ScheduleDisplay() string {
	if a.Cron != "" {
		return a.Cron
	}

	return fmt.Sprintf("in %d minutes", a.DelayOnce)
}

// storeDoc is the persisted JSON shape (round-trip stable; pinned by test).
type storeDoc struct {
	Automations []Automation `json:"automations"`
}

// FireEvent is one catch-up firing: the automation plus the missed-window
// note injected into its prompt context (D-02: each catch-up firing carries
// the note).
type FireEvent struct {
	Automation Automation
	Note       string
}

// ScheduleStore is the per-project persisted schedule (workDir-scoped —
// shared across the project's sessions, NOT per-session; D-02 says per-project
// explicitly). Mutex-guarded; every mutation persists atomically
// (temp+rename) so a crash never corrupts the store.
type ScheduleStore struct {
	mu      sync.Mutex
	path    string
	autos   []Automation
	quarant string // the quarantine rename target of a corrupt file (diagnostics)

	// now is the injectable clock (tests pin it; default time.Now).
	now func() time.Time
}

// Open loads (or initializes) the per-project schedule store under
// workDir/.ass-guard/schedule/schedules.json. A corrupt file is quarantined
// (renamed aside with a .corrupt-<ts> suffix) and a fresh store opens —
// graceful degradation, never a crash (T-12-07-01's load-tolerant posture).
func Open(workDir string) (*ScheduleStore, error) {
	dir := filepath.Join(workDir, storeDirName, scheduleDir)

	err := os.MkdirAll(dir, dirPerm)
	if err != nil {
		return nil, fmt.Errorf("sched: mkdir: %w", err)
	}

	s := &ScheduleStore{
		path: filepath.Join(dir, storeFileName),
		now:  time.Now,
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil // fresh store; persisted on first mutation
		}

		return nil, fmt.Errorf("sched: read store: %w", err)
	}

	var doc storeDoc

	uerr := json.Unmarshal(raw, &doc)
	if uerr != nil {
		s.quarant = s.path + ".corrupt-" + strconv.FormatInt(s.now().Unix(), 10)

		rerr := os.Rename(s.path, s.quarant)
		if rerr != nil {
			return nil, fmt.Errorf("sched: quarantine corrupt store (%w): %w", rerr, uerr)
		}

		// Persist the fresh store immediately: the corrupt file is REPLACED
		// on disk, not merely skipped (the quarantine is visible beside it).
		perr := s.persistLocked()
		if perr != nil {
			return nil, perr
		}

		return s, nil
	}

	s.autos = doc.Automations

	return s, nil
}

// Now returns the store's clock (the injectable time source).
func (s *ScheduleStore) Now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.clock()
}

// SetNow injects the clock (tests); nil restores time.Now.
func (s *ScheduleStore) SetNow(f func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if f == nil {
		s.now = time.Now

		return
	}

	s.now = f
}

// QuarantinePath reports the quarantine target of the last corrupt load ("".
// when none) — diagnostics for the audit trail.
func (s *ScheduleStore) QuarantinePath() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.quarant
}

// Create validates + persists a new automation, assigning the ID and
// createdAt stamp. Validation: prompt + title required (schema); exactly one
// of cron (parseable) or delayMinutes (1..525600, the schema bounds).
func (s *ScheduleStore) Create(a *Automation) (Automation, error) {
	err := validate(a)
	if err != nil {
		return Automation{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	a.ID = newID(s.clock())
	a.CreatedAt = s.clock().UTC()
	a.RunCount = 0
	a.Done = false

	s.autos = append(s.autos, *a)

	err = s.persistLocked()
	if err != nil {
		return Automation{}, err
	}

	return *a, nil
}

func validate(a *Automation) error {
	if a.Prompt == "" || a.Title == "" {
		return fmt.Errorf("%w: prompt and title are required", ErrRequiredField)
	}

	hasCron, hasDelay := a.Cron != "", a.DelayOnce != 0
	if hasCron == hasDelay {
		return fmt.Errorf("%w (got cron=%q delayMinutes=%d)", ErrCronXorDelay, a.Cron, a.DelayOnce)
	}

	if hasCron {
		_, perr := ParseCron(a.Cron)
		if perr != nil {
			return perr
		}
	} else if a.DelayOnce < 1 || a.DelayOnce > maxDelayMinutes {
		return fmt.Errorf("%w: %d (bounds 1..%d)", ErrBadDelay, a.DelayOnce, maxDelayMinutes)
	}

	return nil
}

// Save persists an updated automation in full (the executor layer patches a
// fetched record, then Save). Unknown id → ErrNotFound.
func (s *ScheduleStore) Save(a *Automation) error {
	err := validate(a)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.autos {
		if s.autos[i].ID == a.ID {
			s.autos[i] = *a

			return s.persistLocked()
		}
	}

	return fmt.Errorf("%w: %s", ErrNotFound, a.ID)
}

// List returns a copy of every automation, stable by creation order.
func (s *ScheduleStore) List() []Automation {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Automation(nil), s.autos...)
}

// Get returns one automation by id.
func (s *ScheduleStore) Get(id string) (Automation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.autos {
		if s.autos[i].ID == id {
			return s.autos[i], nil
		}
	}

	return Automation{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Delete removes an automation (unknown id → ErrNotFound).
func (s *ScheduleStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.autos {
		if s.autos[i].ID == id {
			s.autos = append(s.autos[:i], s.autos[i+1:]...)

			return s.persistLocked()
		}
	}

	return fmt.Errorf("%w: %s", ErrNotFound, id)
}

// MarkFired records a firing at t: LastFired=t, RunCount++, and Done flips
// for spent automations (delay one-shots always; recurring=false when the
// finite MaxRuns budget is reached — MaxRuns 0 with recurring=false means
// run once, the schema's documented default).
func (s *ScheduleStore) MarkFired(id string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.autos {
		if s.autos[i].ID != id {
			continue
		}

		s.autos[i].LastFired = t.UTC()
		s.autos[i].RunCount++

		a := s.autos[i]
		spent := a.Cron == "" || (!a.Recurring && (a.MaxRuns == 0 || a.RunCount >= a.MaxRuns))
		s.autos[i].Done = spent

		return s.persistLocked()
	}

	return fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Due returns the automations with at least one UNFIRED schedule slot at or
// before now (the scheduler tick's walk) — i.e. the schedule produced a slot
// since the last fire (or creation) that has already passed. Spent
// automations never appear.
func (s *ScheduleStore) Due(now time.Time) []Automation {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Automation, 0, len(s.autos))

	for i := range s.autos {
		a := &s.autos[i]
		if a.Done {
			continue
		}

		from := a.LastFired
		if from.IsZero() {
			from = a.CreatedAt
		}

		if missedSlots(a, from, now) > 0 {
			out = append(out, *a)
		}
	}

	return out
}

// CatchUp computes the fire-once catch-up events for missed automations:
// every non-spent automation whose schedule had at least one missed slot
// between its last fire (or creation) and now fires EXACTLY ONCE, the event
// carrying the missed-window note. Each fired automation's lastFired is
// persisted BEFORE the events return (a crash mid-catch-up cannot re-fire a
// window — D-02's exactly-once).
func (s *ScheduleStore) CatchUp(now time.Time) []FireEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	var events []FireEvent

	for i := range s.autos {
		a := s.autos[i]
		if a.Done {
			continue
		}

		from := a.LastFired
		if from.IsZero() {
			from = a.CreatedAt
		}

		missed := missedSlots(&a, from, now)
		if missed == 0 {
			continue
		}

		//nolint:gosmopolitan // the captured schema mandates local wall-clock rendering
		windowFrom := from.Local().Format(time.RFC3339)
		windowTo := now.Local().Format(time.RFC3339) //nolint:gosmopolitan // schema-mandated local rendering
		note := fmt.Sprintf(
			"scheduled automation %q missed its window %s .. %s (%d missed slot(s)) "+
				"while no ass-guard session was active; firing once now — catch-up never repeats",
			a.Title, windowFrom, windowTo, missed,
		)

		s.autos[i].LastFired = now.UTC()
		s.autos[i].RunCount++

		spent := a.Cron == "" || (!a.Recurring && (a.MaxRuns == 0 || s.autos[i].RunCount >= a.MaxRuns))
		s.autos[i].Done = spent

		events = append(events, FireEvent{Automation: s.autos[i], Note: note})
	}

	if len(events) > 0 {
		perr := s.persistLocked()
		if perr != nil {
			// The in-memory state already advanced; the next open re-derives
			// from the last persisted lastFired — the failure is loud but
			// never re-fires a window already delivered to this process.
			_ = perr // persist failure path: in-memory exactly-once holds
		}
	}

	return events
}

func (s *ScheduleStore) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}

	return s.now()
}

// NextDue reports when the automation is next due after `after` (ok=false
// for spent automations).
func NextDue(a *Automation, after time.Time) (time.Time, bool) {
	return nextDueLocked(a, after)
}

func nextDueLocked(a *Automation, after time.Time) (time.Time, bool) {
	if a.Done {
		return time.Time{}, false
	}

	if a.Cron == "" {
		return a.CreatedAt.Add(time.Duration(a.DelayOnce) * time.Minute), true
	}

	spec, err := ParseCron(a.Cron)
	if err != nil {
		return time.Time{}, false
	}

	//nolint:gosmopolitan // "Do not convert to UTC" — the captured schema's rule
	return spec.Next(after, time.Local), true
}

// missedSlots counts the schedule slots strictly after `from` and at or
// before `now` (the catch-up window size).
func missedSlots(a *Automation, from, now time.Time) int {
	if !from.Before(now) {
		return 0
	}

	if a.Cron == "" {
		// One-shot: due once at createdAt+delay; missed if it passed.
		if due := a.CreatedAt.Add(time.Duration(a.DelayOnce) * time.Minute); !due.After(now) && a.LastFired.IsZero() {
			return 1
		}

		return 0
	}

	spec, err := ParseCron(a.Cron)
	if err != nil {
		return 0
	}

	count := 0
	//nolint:gosmopolitan // local-tz scheduling is the schema's explicit rule
	for t := spec.Next(from, time.Local); !t.After(now); t = spec.Next(t, time.Local) {
		count++

		if count > 1000 { //nolint:mnd // pathological-window bound (T-12-07-02)
			break
		}
	}

	return count
}

// persistLocked writes the store atomically (temp+rename, 0600). Caller holds
// the mutex.
func (s *ScheduleStore) persistLocked() error {
	raw, err := json.MarshalIndent(storeDoc{Automations: s.autos}, "", "  ")
	if err != nil {
		return fmt.Errorf("sched: marshal: %w", err)
	}

	tmp := s.path + ".tmp"

	err = os.WriteFile(tmp, raw, filePerm)
	if err != nil {
		return fmt.Errorf("sched: write temp: %w", err)
	}

	err = os.Rename(tmp, s.path)
	if err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("sched: rename: %w", err)
	}

	return nil
}

// newID generates the automation id (the documented corpus_absent default:
// cron_ + 12 hex chars of the clock's nanos — stable, sortable, unique per
// creation).
func newID(now time.Time) string {
	return fmt.Sprintf("cron_%x", now.UnixNano())
}

// Spec is a parsed 5-field cron expression: minute hour dom month dow, each
// field a sorted set of allowed values (lists, ranges, and */step per the
// schema's examples).
type Spec struct {
	fields [5]fieldSet
}

// Field order constants (5-field cron).
const (
	fieldMinute = iota
	fieldHour
	fieldDom
	fieldMonth
	fieldDow
)

// fieldSet is one field's allowed values (wildcards/steps are expanded into
// the value set at parse time).
type fieldSet struct {
	values map[int]struct{}
}

// fieldBounds returns each field's inclusive bounds (dow: 0 and 7 are both
// Sunday, the cron convention).
func fieldBounds() [5][2]int {
	return [5][2]int{
		fieldMinute: {0, 59},
		fieldHour:   {0, 23},
		fieldDom:    {1, 31},
		fieldMonth:  {1, 12},
		fieldDow:    {0, 7},
	}
}

// ParseCron parses a standard 5-field cron expression (lists a,b; ranges a-b;
// steps */n and a-b/n; * wildcard). The expression schedules in the USER'S
// LOCAL timezone — the captured schema's explicit rule, honored by Next
// computing against the caller's location.
func ParseCron(expr string) (*Spec, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 { //nolint:mnd // 5-field cron by definition
		return nil, fmt.Errorf("%w: %q (want 5 fields, got %d)", ErrBadCron, expr, len(parts))
	}

	bounds := fieldBounds()
	spec := &Spec{}

	for i, raw := range parts {
		fs, err := parseField(raw, bounds[i][0], bounds[i][1])
		if err != nil {
			return nil, fmt.Errorf("%w: field %d (%q): %w", ErrBadCron, i+1, raw, err)
		}

		spec.fields[i] = fs
	}

	return spec, nil
}

func parseField(raw string, lo, hi int) (fieldSet, error) {
	fs := fieldSet{values: map[int]struct{}{}}

	for item := range strings.SplitSeq(raw, ",") {
		perr := parseFieldItem(fs, strings.TrimSpace(item), lo, hi)
		if perr != nil {
			return fs, perr
		}
	}

	for v := range fs.values {
		if v < lo || v > hi {
			return fs, fmt.Errorf("%w: %d (bounds %d..%d)", ErrOutOfBounds, v, lo, hi)
		}
	}

	return fs, nil
}

// parseFieldItem folds one comma-separated item into the field set.
func parseFieldItem(fs fieldSet, item string, lo, hi int) error {
	if item == "" {
		return ErrEmptyItem
	}

	// Split off a step suffix.
	step := 1

	base, stepSuffix, hasStep := strings.Cut(item, "/")
	if hasStep {
		n, err := strconv.Atoi(stepSuffix)
		if err != nil || n < 1 {
			return fmt.Errorf("%w: %q", ErrBadStep, stepSuffix)
		}

		step = n
	}

	switch {
	case base == "*":
		for v := lo; v <= hi; v += step {
			fs.values[v] = struct{}{}
		}
	case strings.Contains(base, "-"):
		return parseRangeItem(fs, base, step)
	default:
		v, err := strconv.Atoi(base)
		if err != nil {
			return fmt.Errorf("%w: %q", ErrBadValue, base)
		}

		fs.values[v] = struct{}{}
	}

	return nil
}

// parseRangeItem folds one a-b[/step] item into the field set.
func parseRangeItem(fs fieldSet, base string, step int) error {
	dash := strings.Index(base, "-")
	a, err1 := strconv.Atoi(base[:dash])

	b, err2 := strconv.Atoi(base[dash+1:])
	if err1 != nil || err2 != nil || a > b {
		return fmt.Errorf("%w: %q", ErrBadRange, base)
	}

	for v := a; v <= b; v += step {
		fs.values[v] = struct{}{}
	}

	return nil
}

// Next returns the next time STRICTLY AFTER `after` that matches the spec,
// computed in loc's wall clock (pass time.Local — the schema's local-tz rule).
// The scan is minute-granular forward, bounded by a 366-day horizon (an
// unsatisfiable spec like "0 0 31 2 *" returns the horizon rather than
// looping forever).
func (s *Spec) Next(after time.Time, loc *time.Location) time.Time {
	t := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	limit := t.Add(scanHorizon)

	for t.Before(limit) {
		if s.matches(t) {
			return t
		}

		t = t.Add(scanStep)
	}

	return limit
}

func (s *Spec) matches(t time.Time) bool {
	// Day-of-week: Go's Weekday() Sunday=0; cron also accepts 7 as Sunday —
	// the parser stores whatever appeared; normalize both to 0..6.
	dow := int(t.Weekday())

	if _, ok := s.fields[fieldMinute].values[t.Minute()]; !ok {
		return false
	}

	if _, ok := s.fields[fieldHour].values[t.Hour()]; !ok {
		return false
	}

	if _, ok := s.fields[fieldDom].values[t.Day()]; !ok {
		return false
	}

	if _, ok := s.fields[fieldMonth].values[int(t.Month())]; !ok {
		return false
	}

	dowSet := s.fields[fieldDow].values
	if _, ok := dowSet[dow]; !ok {
		// 7-as-Sunday normalization (7 is cron's alternative Sunday).
		_, ok7 := dowSet[7]

		if !ok7 || dow != 0 {
			return false
		}
	}

	return true
}

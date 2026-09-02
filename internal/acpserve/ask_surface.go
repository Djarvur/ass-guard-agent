package acpserve

// The 17-02 permission-ask surface (ACP-01): builds the v1
// session/request_permission frame (exactly the four neutral option kinds;
// the ToolCallUpdate toolCall showing what is being authorized) and dispatches
// it through 16-03's outbound registry under the HUMAN-ASK class — ids, the
// D-14 ladder, the 16-D-19 cascade, and degradation are the registry's, never
// rebuilt here.
//
// The outcome mapping is fail-safe by construction: cancelled / -32800 → the
// cancelled family; -32601 (a pre-permission client) / D-14 timeout / any
// transport failure → the session-native Err outcome, which the gate's resume
// renders as a decline — never a silent allow, never a retry storm (the
// stickiness policy is the session's, 16-D-18 discipline).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// PermissionAsk dispatches permission asks through the registry (the queue's
// fire callback target). One per serve.
type PermissionAsk struct {
	registry *acp.Registry

	// ctx is the serve-lifetime context: the ask outlives its suspending turn
	// (the turn ctx died with the prompt response), so the registry round-trip
	// runs under the serve ctx — the askResumeCtx precedent.
	//
	//nolint:containedctx // deliberate serve-lifetime ctx storage (see NewPermissionAsk)
	ctx context.Context

	log *log.Logger
}

// NewPermissionAsk constructs the surface over the server's outbound registry.
// serveCtx bounds every ask round-trip; stderr (nil-tolerated) receives the
// structured diagnostics — never stdout (transport discipline).
func NewPermissionAsk(serveCtx context.Context, registry *acp.Registry, stderr io.Writer) *PermissionAsk {
	lg := log.New(io.Discard, "", 0)
	if stderr != nil {
		lg = log.New(stderr, "ass-guard/acpserve/ask: ", log.LstdFlags|log.Lmsgprefix)
	}

	return &PermissionAsk{registry: registry, ctx: serveCtx, log: lg}
}

// BuildPermissionAsk builds the v1 frame for one gated call. The option list
// IS the dialog: Zed renders the agent-supplied options as the dialog buttons
// (17-RESEARCH Zed fact 1) — exactly four, optionId == kind, neutral and
// symmetric labels (the ACP-01 values prohibition: never asymmetric-friction
// or steered framing).
func BuildPermissionAsk(sessionID, callID, title string, input json.RawMessage) acp.RequestPermissionFrame {
	toolCall := acp.ToolCallUpdateFrame{
		ToolCallID: callID,
		Title:      title,
		Kind:       acp.ToolKindExecute,
		Status:     acp.StatusPending,
	}

	if len(input) > 0 {
		// The raw input is the dialog's substance (the threat model's
		// "untrusted content DISPLAYED in the dialog's toolCall shape" —
		// shown for authorization, never executed here).
		toolCall.Content = []acp.ToolCallContent{{
			Type:    "content",
			Content: &acp.ContentBlock{Type: "text", Text: string(input)},
		}}
	}

	return acp.RequestPermissionFrame{
		SessionID: sessionID,
		ToolCall:  toolCall,
		Options: []acp.PermissionOption{
			{OptionID: acp.PermOptionAllowOnce, Name: "Allow once", Kind: acp.PermOptionAllowOnce},
			{OptionID: acp.PermOptionAllowAlways, Name: "Always allow", Kind: acp.PermOptionAllowAlways},
			{OptionID: acp.PermOptionRejectOnce, Name: "Reject once", Kind: acp.PermOptionRejectOnce},
			{OptionID: acp.PermOptionRejectAlways, Name: "Always reject", Kind: acp.PermOptionRejectAlways},
		},
	}
}

// Permission-ask failure sentinels — wrapped (never dynamic) so callers can
// errors.Is/As them; each maps to the fail-safe decline outcome.
var (
	errPermissionUnsupported    = errors.New("client does not implement session/request_permission")
	errPermissionOutcomeBad     = errors.New("malformed permission outcome")
	errPermissionOutcomeUnknown = errors.New("unknown permission outcome")
)

// Fire performs one permission-ask round-trip: frame build → Registry.Call
// under HUMAN-ASK → outcome mapping. It runs on the ask queue's pump
// goroutine (one outstanding ask, D-11); the HUMAN-ASK window means a human
// may think for the full configured wait (16-D-17) — the queue holds no lock
// while they do. ctx is the queue-owned per-firing cancellation seam (17-03
// D-13: the turn-death drain cancels an OPEN dialog through it — the registry
// resolves the entry cancelled AND cascades $/cancel_request); nil falls back
// to the serve-lifetime ctx.
func (p *PermissionAsk) Fire(ctx context.Context, e *session.AskEntry) session.AskOutcome { //nolint:contextcheck,lll // ctx is the queue's drain seam; nil falls back to the serve ctx
	if ctx == nil {
		ctx = p.ctx
	}

	frame := BuildPermissionAsk(e.SessionID, e.CallID, e.Title, e.Input)
	frame.ToolCall.Kind = e.Kind

	msg, err := p.registry.Call(ctx, acp.MethodRequestPermission, frame, acp.TimeoutHumanAsk)
	if err != nil {
		if errors.Is(err, acp.ErrRequestCancelled) {
			// Turn death / client cancel / the -32800 answer / shutdown drain:
			// the cancelled family (the session appends the cancelled-NORMAL
			// form, never an error).
			return session.AskOutcome{Cancelled: true}
		}

		// D-14's terminal fallback (unanswered after ONE retry) and every
		// other transport failure: Err → the gate's fail-safe decline.
		p.log.Printf("permission ask failed (failing safe, never allow): tool=%s callID=%s: %v",
			e.Tool, e.CallID, err)

		return session.AskOutcome{Err: err}
	}

	if msg.Error != nil {
		if msg.Error.Code == acp.CodeMethodNotFound {
			// A pre-permission client answers -32601 (the 16-D-18 degrade
			// family): Err+Unsupported → decline-with-note; the session
			// records the degradation sticky for the session.
			p.log.Printf("client does not implement %s (-32601) — failing safe (decline, sticky)",
				acp.MethodRequestPermission)

			return session.AskOutcome{
				Err:         fmt.Errorf("%w (-32601)", errPermissionUnsupported),
				Unsupported: true,
			}
		}

		return session.AskOutcome{Err: fmt.Errorf("%w: code %d: %s",
			errPermissionOutcomeBad, msg.Error.Code, msg.Error.Message)}
	}

	var of acp.PermissionOutcomeFrame

	uerr := json.Unmarshal(msg.Result, &of)
	if uerr != nil {
		p.log.Printf("malformed permission outcome (failing safe): %v", uerr)

		return session.AskOutcome{Err: fmt.Errorf("%w: %w", errPermissionOutcomeBad, uerr)}
	}

	switch of.Outcome.Outcome {
	case acp.PermissionOutcomeCancelled:
		return session.AskOutcome{Cancelled: true}
	case acp.PermissionOutcomeSelected:
		// An empty optionId needs no guard here: it falls through to the
		// session gate's default unknown-option fail-safe branch (internal/
		// session/ask.go resolvePermissionOutcome) — the correct decline.
		return session.AskOutcome{Selected: of.Outcome.OptionID}
	default:
		p.log.Printf("unknown permission outcome %q (failing safe)", of.Outcome.Outcome)

		return session.AskOutcome{Err: fmt.Errorf("%w %q", errPermissionOutcomeUnknown, of.Outcome.Outcome)}
	}
}

// --- 17-04 elicitation surface (ACP-02, D-08/D-09): the D-08 typed mapping
// from AskQuestion shapes to ElicitationSchema properties, and the
// capability-gated dispatcher — sticky elicitation-form capability (16-D-13/
// D-18) → elicitation/create under HUMAN-ASK; degraded → today's plain-text
// path verbatim (the v1.1 route is the FALLBACK, not the primary). ---

// Wire-verbatim property-variant shapes (the D-08 mapping builds exactly
// these; the MCP-legacy enumNames key never appears — RFD ban). Field names
// pinned against schema/v1 ElicitationPropertySchema variants.

// elicChoiceOption is one titled const of a single-select oneOf (or a
// multi-select items.anyOf): the option label is the const AND the title —
// the enum names are the labels, never a separate name table.
type elicChoiceOption struct {
	Const       string `json:"const"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// JSON-schema type discriminators (shared by the builder variants, the
// validator, and the goldens).
const (
	propTypeString = "string"
	propTypeObject = "object"
	propTypeArray  = "array"
	propTypeBool   = "boolean"
)

// elicStringProp is the string property variant: free-text when OneOf is
// empty, a single-select enum otherwise ("When enum or oneOf is set, this
// represents a single-select enum").
type elicStringProp struct {
	Type  string             `json:"type"`
	Title string             `json:"title,omitempty"`
	OneOf []elicChoiceOption `json:"oneOf,omitempty"` //nolint:tagliatelle // ACP wire field
}

// elicBoolProp is the boolean property variant (ONLY when the client
// advertised boolean support — D-08's conservative gate).
type elicBoolProp struct {
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// elicItemsEnum is a multi-select's items without per-option descriptions.
type elicItemsEnum struct {
	Type string   `json:"type"`
	Enum []string `json:"enum"`
}

// elicItemsAnyOf is a multi-select's items WITH descriptions (the RFD's
// "anyOf for titled multi-select").
type elicItemsAnyOf struct {
	AnyOf []elicChoiceOption `json:"anyOf"` //nolint:tagliatelle // ACP wire field
}

// elicArrayProp is the array property variant ("Multi-select enums use the
// Array variant").
type elicArrayProp struct {
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	Items any    `json:"items"` // elicItemsEnum | elicItemsAnyOf
}

// isBooleanAsk reports whether a question is a BOOLEAN ask (17-04, D-08): a
// single-choice question whose two option labels are exactly yes/no
// (case-insensitive). The captured AskQuestion schema carries no boolean flag,
// so the authored yes/no pair is the pinned convention; anything else maps per
// its own row of the D-08 table.
func isBooleanAsk(q session.AskQuestion) bool {
	if q.MultiSelect || len(q.Options) != 2 {
		return false
	}

	return strings.EqualFold(q.Options[0].Label, "yes") && strings.EqualFold(q.Options[1].Label, "no")
}

// elicitationMessage composes the request message: the question text(s)
// verbatim (the D-08 row "the question text lands in the request message"),
// prefixed by the D-10 re-ask note when one rides the entry.
func elicitationMessage(qs []session.AskQuestion, note string) string {
	texts := make([]string, 0, len(qs))
	for _, q := range qs {
		texts = append(texts, q.Question)
	}

	msg := strings.Join(texts, " · ")
	if note != "" {
		msg = "previous answer invalid: " + note + " — " + msg
	}

	return msg
}

// mustPropJSON marshals a built property variant. Marshaling these plain
// structs cannot fail; the fallback keeps the schema valid regardless.
func mustPropJSON(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{"type":"` + propTypeString + `"}`)
	}

	return out
}

// BuildElicitationForm builds the elicitation/create frame for one ask (THE
// D-08 mapping; must_haves' frame builder). Every ask kind has a defined
// rendering: single-choice → string property with oneOf titled consts;
// multiSelect → array property (items enum, anyOf-titled when descriptions
// exist); free-text / empty-Options → string property; boolean ask → boolean
// property ONLY when boolAdvertised, else a two-value string oneOf. Headers
// become property titles; the question text lands in the message; N questions
// → N properties (stable keys q1..qN, session.StructuredPropertyKey), every
// one required. The builder consumes exactly the AskQuestion slice + the
// capability bit + the re-ask note — no other data source (the disclosure-
// parity bound: an elicitation form must not say more than the plain-text ask).
func BuildElicitationForm(
	qs []session.AskQuestion, boolAdvertised bool, note, sessionID, callID string,
) acp.ElicitationFormFrame {
	schema := acp.ElicitationSchema{
		Type:       propTypeObject,
		Properties: make(map[string]json.RawMessage, len(qs)),
		Required:   make([]string, 0, len(qs)),
	}

	for i, q := range qs {
		key := session.StructuredPropertyKey(i)

		switch {
		case q.MultiSelect:
			schema.Properties[key] = mustPropJSON(elicArrayProp{
				Type: propTypeArray, Title: q.Header, Items: multiSelectItems(q),
			})
		case isBooleanAsk(q) && boolAdvertised:
			schema.Properties[key] = mustPropJSON(elicBoolProp{Type: propTypeBool, Title: q.Header})
		case len(q.Options) > 0:
			// Single-choice AND the boolean-not-advertised branch (the
			// conservative two-value string oneOf — D-08 as locked).
			schema.Properties[key] = mustPropJSON(oneOfStringProperty(q.Header, q.Options))
		default:
			schema.Properties[key] = mustPropJSON(elicStringProp{Type: propTypeString, Title: q.Header})
		}

		schema.Required = append(schema.Required, key)
	}

	return acp.ElicitationFormFrame{
		Message:         elicitationMessage(qs, note),
		Mode:            acp.ElicitationModeForm,
		RequestedSchema: schema,
		SessionID:       sessionID,
		ToolCallID:      callID,
	}
}

// multiSelectItems picks the items variant: anyOf-titled when any option
// carries a description, else the plain enum of labels.
func multiSelectItems(q session.AskQuestion) any {
	titled := false

	for _, o := range q.Options {
		if o.Description != "" {
			titled = true

			break
		}
	}

	if !titled {
		labels := make([]string, 0, len(q.Options))
		for _, o := range q.Options {
			labels = append(labels, o.Label)
		}

		return elicItemsEnum{Type: propTypeString, Enum: labels}
	}

	opts := make([]elicChoiceOption, 0, len(q.Options))
	for _, o := range q.Options {
		opts = append(opts, elicChoiceOption{Const: o.Label, Title: o.Label, Description: o.Description})
	}

	return elicItemsAnyOf{AnyOf: opts}
}

// oneOfStringProperty builds the single-select string property: one titled
// const per option label (the enum names ARE the labels).
func oneOfStringProperty(title string, opts []session.AskOption) elicStringProp {
	oneOf := make([]elicChoiceOption, 0, len(opts))
	for _, o := range opts {
		oneOf = append(oneOf, elicChoiceOption{Const: o.Label, Title: o.Label, Description: o.Description})
	}

	return elicStringProp{Type: propTypeString, Title: title, OneOf: oneOf}
}

// The D-10 validator's closed-subset schema views (parsed from the raw
// property variants the builder produced). Only the fields the subset reads
// are modeled; unknown schema fields are ignored by construction.

// elicConstView is one const option of a oneOf/anyOf choice set.
type elicConstView struct {
	Const string `json:"const"`
}

// (elicItemsView/elicPropView carry wire-verbatim camelCase tags — see the
// per-field //nolint:tagliatelle markers.)

// elicItemsView is the multi-select items schema view.
type elicItemsView struct {
	Type  string          `json:"type"`
	Enum  []string        `json:"enum,omitempty"`
	AnyOf []elicConstView `json:"anyOf,omitempty"` //nolint:tagliatelle // ACP wire field
	OneOf []elicConstView `json:"oneOf,omitempty"` //nolint:tagliatelle // ACP wire field
}

// elicPropView is one property schema view: the closed validation subset —
// type, choice membership, and length bounds. Membership uses EXACT Go string
// equality; lengths are RUNE counts; there is deliberately no pattern engine
// of any kind and no Unicode rewriting (the encoding edge is pinned by test; the RFD's
// bounded-matcher MUST is satisfied by the matcher's absence — T-17-13).
type elicPropView struct {
	Type      string          `json:"type"`
	OneOf     []elicConstView `json:"oneOf,omitempty"` //nolint:tagliatelle // ACP wire field
	Enum      []string        `json:"enum,omitempty"`
	Items     *elicItemsView  `json:"items,omitempty"`
	MinLength *int            `json:"minLength,omitempty"` //nolint:tagliatelle // ACP wire field
	MaxLength *int            `json:"maxLength,omitempty"` //nolint:tagliatelle // ACP wire field
}

// ValidateElicitationContent re-validates one accept payload against the
// requested schema it claims to answer (17-04 D-10, T-17-11): the closed
// subset — required presence, per-property type, enum/oneOf const membership
// by exact string equality, array item-wise checks, minLength/maxLength by
// rune count. Unknown content keys are IGNORED (never rendered — the renderer
// walks the schema's own property order), so extra data can neither execute
// nor surface. Returns "" when the content is valid, else one human-readable
// violation (the first, in required-then-sorted-key order).
func ValidateElicitationContent(schema acp.ElicitationSchema, content map[string]json.RawMessage) string {
	if len(schema.Required) == 0 && len(schema.Properties) == 0 {
		return ""
	}

	if content == nil {
		// A null/absent content object on accept is itself a violation — the
		// empty edge is the missing-required family.
		return missingRequiredViolation(schema.Required)
	}

	for _, name := range schema.Required {
		if _, ok := content[name]; !ok {
			return fmt.Sprintf("missing required property %q", name)
		}
	}

	keys := make([]string, 0, len(content))
	for k := range content {
		if _, known := schema.Properties[k]; known {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	for _, k := range keys {
		var prop elicPropView

		uerr := json.Unmarshal(schema.Properties[k], &prop)
		if uerr != nil {
			continue // an unparseable property schema validates as free-form
		}

		if v := validatePropValue(k, &prop, content[k]); v != "" {
			return v
		}
	}

	return ""
}

// missingRequiredViolation names the first required property (deterministic).
func missingRequiredViolation(required []string) string {
	if len(required) == 0 {
		return "missing content object"
	}

	return fmt.Sprintf("missing required property %q", required[0])
}

// validatePropValue checks one value against one property view.
func validatePropValue(key string, prop *elicPropView, raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Sprintf("property %q: a JSON null is not a content value", key)
	}

	switch prop.Type {
	case propTypeString:
		return validateStringValue(key, prop, raw)
	case propTypeBool:
		var b bool

		if json.Unmarshal(raw, &b) != nil {
			return fmt.Sprintf("property %q: expected a boolean", key)
		}

		return ""
	case propTypeArray:
		return validateArrayValue(key, prop, raw)
	default:
		// An unrecognized property type validates as free-form (the builder
		// only emits string/boolean/array).
		return ""
	}
}

// validateStringValue checks a string value: type, choice membership, bounds.
func validateStringValue(key string, prop *elicPropView, raw json.RawMessage) string {
	var s string

	if json.Unmarshal(raw, &s) != nil {
		return fmt.Sprintf("property %q: expected a string", key)
	}

	if len(prop.OneOf) > 0 && !elicConstMatch(prop.OneOf, s) {
		return fmt.Sprintf("property %q: value %q is not one of the offered choices", key, s)
	}

	if len(prop.Enum) > 0 && !slices.Contains(prop.Enum, s) {
		return fmt.Sprintf("property %q: value %q is not one of the offered choices", key, s)
	}

	if prop.MinLength != nil && utf8.RuneCountInString(s) < *prop.MinLength {
		return fmt.Sprintf("property %q: length %d is below the minimum of %d",
			key, utf8.RuneCountInString(s), *prop.MinLength)
	}

	if prop.MaxLength != nil && utf8.RuneCountInString(s) > *prop.MaxLength {
		return fmt.Sprintf("property %q: length %d exceeds the maximum of %d",
			key, utf8.RuneCountInString(s), *prop.MaxLength)
	}

	return ""
}

// validateArrayValue checks a multi-select value: array-of-strings shape plus
// item-wise enum/anyOf membership.
func validateArrayValue(key string, prop *elicPropView, raw json.RawMessage) string {
	var arr []json.RawMessage

	if json.Unmarshal(raw, &arr) != nil {
		return fmt.Sprintf("property %q: expected an array of strings", key)
	}

	for i, item := range arr {
		var s string

		if json.Unmarshal(item, &s) != nil {
			return fmt.Sprintf("property %q: item %d is not a string", key, i)
		}

		if prop.Items == nil {
			continue
		}

		switch {
		case len(prop.Items.Enum) > 0 && !slices.Contains(prop.Items.Enum, s):
			return fmt.Sprintf("property %q: item %q is not one of the offered choices", key, s)
		case len(prop.Items.AnyOf) > 0 && !elicConstMatch(prop.Items.AnyOf, s):
			return fmt.Sprintf("property %q: item %q is not one of the offered choices", key, s)
		case len(prop.Items.OneOf) > 0 && !elicConstMatch(prop.Items.OneOf, s):
			return fmt.Sprintf("property %q: item %q is not one of the offered choices", key, s)
		}
	}

	return ""
}

// elicConstMatch reports membership by EXACT Go string equality (the encoding
// edge: no normalization, no case folding).
func elicConstMatch(consts []elicConstView, s string) bool {
	for _, c := range consts {
		if c.Const == s {
			return true
		}
	}

	return false
}

// ElicitationAskConfig carries the dispatcher's injected dependencies.
type ElicitationAskConfig struct {
	// Ctx is the serve-lifetime context: the ask outlives its suspending turn
	// (the askResumeCtx precedent). Fire's ctx (the queue's per-firing drain
	// seam) takes precedence when non-nil.
	//
	//nolint:containedctx // deliberate serve-lifetime ctx storage (see NewPermissionAsk)
	Ctx context.Context

	// Registry is the 16-03 outbound registry (ids, D-14 ladder, 16-D-19
	// cascade — never rebuilt here).
	Registry *acp.Registry

	// Stderr (nil-tolerated) receives structured diagnostics — never stdout
	// (transport discipline).
	Stderr io.Writer

	// CapOK reads the sticky elicitation-form capability (16-D-13/D-18): the
	// initialize-time advertisement-or-probe result for the connection. nil =
	// unknown → conservative fallback (today's plain-text path).
	CapOK func() bool

	// BoolAdvertised reads the boolean config-option capability (D-08's
	// conservative boolean gate). nil = not advertised.
	BoolAdvertised func() bool

	// Fallback publishes the plain-text ask surface — the v1.1 route VERBATIM
	// (same bus shape, byte-parity content). The composition binds it to the
	// runner's subscriber-backed publish seam.
	Fallback func(turnID, text string)
}

// ElicitationAsk dispatches question-family asks (AskUserQuestion, engine/
// learning asks) through elicitation/create, gated by the sticky capability,
// with today's plain-text path as the verbatim fallback branch. One per serve.
type ElicitationAsk struct {
	cfg      ElicitationAskConfig
	registry *acp.Registry

	//nolint:containedctx // deliberate serve-lifetime ctx storage (see ElicitationAskConfig.Ctx)
	ctx context.Context

	log  *log.Logger
	fb   func(turnID, text string)
	capF func() bool
	bcap func() bool

	// degraded is the STICKY mid-session degradation (16-D-18 family): a
	// client that answered -32601 despite a successful negotiation cannot
	// answer elicitation at all — later asks skip the round-trip.
	mu       sync.Mutex
	degraded bool
}

// NewElicitationAsk constructs the dispatcher.
func NewElicitationAsk(cfg ElicitationAskConfig) *ElicitationAsk {
	lg := log.New(io.Discard, "", 0)
	if cfg.Stderr != nil {
		lg = log.New(cfg.Stderr, "ass-guard/acpserve/elicitation: ", log.LstdFlags|log.Lmsgprefix)
	}

	return &ElicitationAsk{
		cfg:      cfg,
		registry: cfg.Registry,
		ctx:      cfg.Ctx,
		log:      lg,
		fb:       cfg.Fallback,
		capF:     cfg.CapOK,
		bcap:     cfg.BoolAdvertised,
	}
}

// Fire performs one question-ask surface round: capability-gated
// elicitation/create (HUMAN-ASK window) with the plain-text fallback routing.
// ctx is the queue-owned per-firing cancellation seam (17-03 D-13); nil falls
// back to the serve-lifetime ctx. The outcome mapping degrades fail-safe:
// EVERY degraded or malformed case lands on the plain-text fallback (the ask
// stays answerable exactly as v1.1) — never a dead ask, never a fabricated
// answer.
func (e *ElicitationAsk) Fire(ctx context.Context, entry *session.AskEntry) session.AskOutcome {
	ctx = e.fireCtx(ctx)

	qs := parseAskQuestions(entry.Input)

	if !e.capable() {
		// Probe-degraded / -32601-sticky client: today's plain-text path
		// verbatim (the D-09 fallback; byte-parity pinned by test).
		return e.fallback(entry, qs)
	}

	frame := BuildElicitationForm(qs, e.boolAdvertised(), entry.Note, entry.SessionID, entry.CallID)

	msg, err := e.registry.Call(ctx, acp.MethodElicitationCreate, frame, acp.TimeoutHumanAsk)
	if err != nil {
		if errors.Is(err, acp.ErrRequestCancelled) {
			// Turn death / client cancel / the -32800 answer / the shutdown
			// drain: the cancelled family.
			return session.AskOutcome{Cancelled: true}
		}

		e.log.Printf("elicitation ask failed — falling back to plain text: callID=%s: %v", entry.CallID, err)

		return e.fallback(entry, qs)
	}

	if msg.Error != nil {
		return e.rpcErrorFallback(entry, qs, msg.Error)
	}

	return e.parseOutcome(entry, qs, frame.RequestedSchema, msg.Result)
}

// fallback publishes the plain-text surface and reports the fallback outcome
// (the dispatcher's degrade landing).
func (e *ElicitationAsk) fallback(entry *session.AskEntry, qs []session.AskQuestion) session.AskOutcome {
	e.publishFallback(entry, qs)

	return session.AskOutcome{Fallback: true}
}

// rpcErrorFallback maps a JSON-RPC error response: -32601 degrades STICKY
// (16-D-18 — the client cannot answer elicitation at all), every other error
// degrades transiently; both land on the plain-text fallback.
func (e *ElicitationAsk) rpcErrorFallback(
	entry *session.AskEntry, qs []session.AskQuestion, rpcErr *acp.RPCError,
) session.AskOutcome {
	if rpcErr.Code == acp.CodeMethodNotFound {
		e.markDegraded()
		e.log.Printf("client answered -32601 for %s — degrading sticky (plain-text fallback)",
			acp.MethodElicitationCreate)

		return e.fallback(entry, qs)
	}

	e.log.Printf("elicitation ask answered with jsonrpc error %d — falling back to plain text", rpcErr.Code)

	return e.fallback(entry, qs)
}

// parseOutcome maps the response body: accept (untrusted content rides to the
// D-10 resolution), decline, cancel; malformed or unknown actions land on the
// plain-text fallback.
func (e *ElicitationAsk) parseOutcome(
	entry *session.AskEntry, qs []session.AskQuestion, schema acp.ElicitationSchema, raw json.RawMessage,
) session.AskOutcome {
	var of acp.ElicitationOutcomeFrame

	uerr := json.Unmarshal(raw, &of)
	if uerr != nil {
		e.log.Printf("malformed elicitation outcome — falling back to plain text: %v", uerr)

		return e.fallback(entry, qs)
	}

	switch of.Action {
	case acp.ElicitationActionAccept:
		// The content is UNTRUSTED input claiming to match requestedSchema —
		// re-validated HERE (T-17-11, the RFD's defense-in-depth); the D-10
		// loop at the resolution boundary turns a violation into the one
		// bounded re-ask.
		violation := ValidateElicitationContent(schema, of.Content)

		return session.AskOutcome{Elicit: acp.ElicitationActionAccept, Content: of.Content, Violation: violation}
	case acp.ElicitationActionDecline:
		return session.AskOutcome{Elicit: acp.ElicitationActionDecline}
	case acp.ElicitationActionCancel:
		return session.AskOutcome{Cancelled: true}
	default:
		e.log.Printf("unknown elicitation action %q — falling back to plain text", of.Action)

		return e.fallback(entry, qs)
	}
}

// fireCtx resolves the round-trip ctx: the queue's per-firing drain seam takes
// precedence; nil falls back to the stored serve-lifetime ctx (the
// askResumeCtx precedent — the suspending turn's ctx died with its response).
func (e *ElicitationAsk) fireCtx(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}

	return e.ctx
}

// capable reports the sticky capability: the initialize-time negotiation AND
// no mid-session -32601 degradation.
func (e *ElicitationAsk) capable() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.degraded {
		return false
	}

	return e.capF != nil && e.capF()
}

// markDegraded records the sticky -32601 degradation.
func (e *ElicitationAsk) markDegraded() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.degraded = true
}

// boolAdvertised resolves the D-08 boolean gate (nil reader = conservative
// not-advertised).
func (e *ElicitationAsk) boolAdvertised() bool { return e.bcap != nil && e.bcap() }

// publishFallback delivers the plain-text ask surface — today's path verbatim
// (the dispatcher's fallback branch, never the primary). Unwired fallback =
// the surface simply does not publish (the broker's reply routing + D-01
// timer still own a question ask).
func (e *ElicitationAsk) publishFallback(entry *session.AskEntry, qs []session.AskQuestion) {
	if e.fb == nil {
		return
	}

	e.fb(entry.TurnID, coreexec.RenderAskSurface(qs))
}

// parseAskQuestions decodes the entry's question payload (the AskUserQuestion
// executor's Output shape — the marshaled []AskQuestion).
func parseAskQuestions(input json.RawMessage) []session.AskQuestion {
	var qs []session.AskQuestion

	_ = json.Unmarshal(input, &qs)

	return qs
}

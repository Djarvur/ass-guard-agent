package profilecheckcmd

import "errors"

// ErrNightlyDrift is the typed drift verdict: the pinned bundle or the zcode
// version moved (or the probe failed — an unprovable environment never reads
// as parity). The cobra shell maps it to exit code 7 so CI can distinguish
// drift from operational failure (exit 1).
var ErrNightlyDrift = errors.New("nightly parity drift")

// NightlyCheckOptions configures the nightly upstream-parity check (TAIL-02,
// D-09 light path: zcode version probe + bundle structure hash vs the pinned
// capture — zero API spend). VersionFunc defaults to the paritycli exec seam;
// tests inject a fake probe.
type NightlyCheckOptions struct {
	ProfilesDir string
	PinPath     string
	ReportPath  string
	WritePin    bool
	VersionFunc func() (string, error)
}

// structurePin is the committed structure-pin.json shape: the pinned capture's
// version source and one sha256 digest per pinned bundle file.
type structurePin struct {
	GeneratedAt  string            `json:"generated_at"`
	ZcodeVersion string            `json:"zcode_version"`
	Files        map[string]string `json:"files"`
}

// nightlyVersionState is the report's version row: pin expectation vs probe.
type nightlyVersionState struct {
	Expected string `json:"expected"`
	Found    string `json:"found"`
	Match    bool   `json:"match"`
	Reason   string `json:"reason,omitempty"`
}

// nightlyFileState is the report's per-file row: pin digest vs bundle digest.
type nightlyFileState struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Match    bool   `json:"match"`
}

// NightlyReport is the drift report JSON written on every check run (D-10:
// the artifact is the belt — it exists whether or not drift was found). File
// names, sha256 digests, and version strings only — never capture bytes
// (T-24-04-04).
type NightlyReport struct {
	GeneratedAt  string               `json:"generated_at"`
	ZcodeVersion nightlyVersionState `json:"zcode_version"`
	Files        []nightlyFileState   `json:"files"`
	Drift        bool                 `json:"drift"`
	Summary      string               `json:"summary"`
}

// RunNightlyCheck runs the nightly drift core. RED stub: the failing
// nightly_check_test.go suite is the intentional RED gate (24-04); the
// implementation lands in GREEN.
func RunNightlyCheck(opts NightlyCheckOptions) error {
	_ = opts

	return nil
}

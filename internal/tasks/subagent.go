package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// The async subagent leg (22-03, PAR-07): registration on the 22-01
// tracker (queued-or-launch, D-10), the nested loop under a SERVE-LIFETIME
// ctx (the dispatching turn's ctx dies at dispatch return — a turn-scoped
// ctx would kill the subagent instantly, RESEARCH Anti-Pattern), the
// progressive output file (header from t=0, progress as produced, terminal
// marker before the notification — the pointer never dangles, Pitfall 9),
// and the kind-tagged completion through the SAME subsystem as bash.

// SubagentDeps carries the launch inputs. Run is the nested-loop closure
// (the session-side runner wrapped by the runtime adapter); progress
// receives streamed text/tool-result lines for the output file.
type SubagentDeps struct {
	Run      func(ctx context.Context, progress func(text string)) (string, error)
	WorkDir  string
	Tracker  *Tracker
	ServeCtx func() context.Context
}

// SubagentLaunch is the discriminated dispatch result's source data.
type SubagentLaunch struct {
	TaskID     string
	OutputFile string
	Queued     bool
	Note       string
	Err        error
}

// subagentTailWindow bounds the in-memory tail the notification carries
// (the tracker's TailBytes truncates further at the rune boundary).
const subagentTailWindow = 16 * 1024

// RunBackgroundSubagent launches one background subagent: mint the id
// (crypto/rand hex, the background.go idiom — never model input), create
// the output file with a header (the pointer resolves from t=0), register
// on the tracker (under cap → start immediately; over cap → FIFO queue
// with the D-10 note), run the loop on the serve-lifetime ctx, and fire
// Tracker.Complete (KindSubagent) on every terminal path AFTER the marker
// write (the D-02 tail includes it).
func RunBackgroundSubagent(dep SubagentDeps) SubagentLaunch {
	id := newSubagentTaskID()
	logPath := filepath.Join(dep.WorkDir, ".ass-guard", "outputs", id+".log")

	w, werr := newSubagentWriter(logPath, id)
	if werr != nil {
		return SubagentLaunch{Err: fmt.Errorf("tasks: background subagent log: %w", werr)}
	}

	started := time.Now()

	var cancelled atomic.Bool

	start := func() func() {
		ctx, cancel := context.WithCancel(dep.ServeCtx())

		var once sync.Once

		cancelFn := func() {
			cancelled.Store(true)
			once.Do(cancel)
		}

		go func() {
			defer cancelFn() // natural completion releases the ctx resources

			// G-22-3 (CR-03): the D-13 invariant, background leg — a panic
			// in the nested loop NEVER kills the process (mirroring the
			// foreground recover, session/subagent.go). LIFO: this defer
			// runs FIRST during unwind (the panic unwinds past the normal
			// Complete below, so exactly ONE error notification fires from
			// here); cancelFn still runs after on both paths.
			defer func() {
				if p := recover(); p != nil {
					w.finish("error", fmt.Sprintf("subagent panic: %v\n%s", p, debug.Stack()))

					dep.Tracker.Complete(Notification{
						TaskID:     id,
						Kind:       KindSubagent,
						ExitStatus: "error",
						Duration:   time.Since(started),
						Tail:       w.tail(),
						OutputFile: logPath,
					})
				}
			}()

			result, rerr := dep.Run(ctx, w.append)

			status, marker := "0", "completed"
			switch {
			case cancelled.Load():
				status, marker = "killed", "killed"
			case rerr != nil:
				status, marker = "error", "error: "+rerr.Error()
			}

			w.finish(marker, result)

			dep.Tracker.Complete(Notification{
				TaskID:     id,
				Kind:       KindSubagent,
				ExitStatus: status,
				Duration:   time.Since(started),
				Tail:       w.tail(),
				OutputFile: logPath,
			})
		}()

		return cancelFn
	}

	queued, qerr := dep.Tracker.RegisterSubagent(id, start)
	if qerr != nil {
		_ = w.f.Close()

		return SubagentLaunch{TaskID: id, OutputFile: logPath, Queued: true, Err: qerr}
	}

	note := ""
	if queued {
		note = dep.Tracker.QueuedNote(id)
	}

	return SubagentLaunch{TaskID: id, OutputFile: logPath, Queued: queued, Note: note}
}

// newSubagentTaskID mints the exec_<hex> id (the shared D-02 shape).
func newSubagentTaskID() string {
	var b [16]byte

	if _, rerr := rand.Read(b[:]); rerr != nil {
		return fmt.Sprintf("exec_%d", time.Now().UnixNano()) // unreachable fallback
	}

	return "exec_" + hex.EncodeToString(b[:])
}

// subagentWriter is the progressive output-file writer: mutex-guarded
// appends flushed per line (mid-run Read is truthful), rune-boundary
// buffering (a chunk split inside a multi-byte rune is held until whole —
// the file is always valid UTF-8), a bounded in-memory tail window, and
// exactly ONE terminal marker per file (the race winner).
type subagentWriter struct {
	mu      sync.Mutex
	f       *os.File
	pending []byte // held partial-rune prefix
	tailBuf []byte // bounded window (last subagentTailWindow bytes)
}

func newSubagentWriter(logPath, id string) (*subagentWriter, error) {
	if merr := os.MkdirAll(filepath.Dir(logPath), 0o750); merr != nil {
		return nil, fmt.Errorf("mkdir: %w", merr)
	}

	f, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}

	header := fmt.Sprintf("task_id: %s\nkind: subagent\nstarted: %s\n---\n", id, time.Now().UTC().Format(time.RFC3339))

	w := &subagentWriter{f: f}
	w.writeLocked([]byte(header))

	return w, nil
}

// append buffers to rune boundaries and writes whole runes only.
func (w *subagentWriter) append(text string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.writeRuneBuffered([]byte(text))
}

// writeRuneBuffered appends bytes, holding back any trailing INCOMPLETE
// rune for the next call — the file only ever receives whole runes (callers
// hold w.mu). A trailing byte sequence is incomplete iff a lead byte sits
// within UTFMax-1 bytes of the end without its full continuation.
func (w *subagentWriter) writeRuneBuffered(p []byte) {
	buf := append(w.pending, p...)
	w.pending = nil

	cut := len(buf)

	for i := 1; i < utf8.UTFMax && i <= len(buf); i++ {
		b := buf[len(buf)-i]

		if !utf8.RuneStart(b) {
			continue // continuation byte — keep walking to the lead
		}

		if r, size := utf8.DecodeRune(buf[len(buf)-i:]); r == utf8.RuneError && size == 1 {
			// the lead announces more bytes than remain — incomplete
			cut = len(buf) - i
		}

		break // the first lead byte from the tail decides
	}

	if cut < len(buf) {
		w.pending = append([]byte(nil), buf[cut:]...)
	}

	if cut > 0 {
		w.writeLocked(buf[:cut])
	}
}

// writeLocked writes + flushes + maintains the bounded tail (callers hold
// w.mu).
func (w *subagentWriter) writeLocked(p []byte) {
	if _, err := w.f.Write(p); err == nil {
		_ = w.f.Sync() // flush per write — mid-run Read is truthful
	}

	w.tailBuf = append(w.tailBuf, p...)
	if len(w.tailBuf) > subagentTailWindow {
		w.tailBuf = w.tailBuf[len(w.tailBuf)-subagentTailWindow:]
	}
}

// finish writes the ONE terminal marker + the final result, then closes.
func (w *subagentWriter) finish(marker, result string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.pending) > 0 { // flush any held partial rune before the marker
		w.tailBuf = append(w.tailBuf, w.pending...)
		w.pending = nil
	}

	line := fmt.Sprintf("\n---\n[%s]\n", marker)
	if result != "" {
		line += result + "\n"
	}

	w.writeLocked([]byte(line))
	_ = w.f.Close()
}

// tail returns the bounded in-memory window (the notification's D-02 tail
// source; the tracker truncates to TailBytes at the rune boundary).
func (w *subagentWriter) tail() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return string(w.tailBuf)
}

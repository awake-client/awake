// Package jsruntime holds the helpers every JavaScript runtime of Awake shares:
// the request scripts and the plugins both run on goja and need the same
// time bound, console and value conversions.
package jsruntime

import (
	"time"

	"github.com/dop251/goja"
)

// Deadline bounds how long a runtime executes. It can stop counting while the
// script waits on work with its own timeout, such as a request it runs, so a
// slow call does not trip the script timeout.
type Deadline struct {
	runtime   *goja.Runtime
	reason    string
	timer     *time.Timer
	expiresAt time.Time
	remaining time.Duration
	paused    bool
}

// NewDeadline starts counting timeout for runtime. Once it expires the runtime
// is interrupted with reason as its error.
func NewDeadline(runtime *goja.Runtime, timeout time.Duration, reason string) *Deadline {
	deadline := &Deadline{runtime: runtime, reason: reason, remaining: timeout}
	deadline.arm()
	return deadline
}

func (deadline *Deadline) arm() {
	deadline.paused = false
	deadline.expiresAt = time.Now().Add(deadline.remaining)
	deadline.timer = time.AfterFunc(deadline.remaining, func() {
		deadline.runtime.Interrupt(deadline.reason)
	})
}

// Pause stops counting and keeps the time left for Resume.
func (deadline *Deadline) Pause() {
	if deadline.paused {
		return
	}
	deadline.paused = true
	if deadline.timer.Stop() {
		deadline.remaining = time.Until(deadline.expiresAt)
	}
}

// Resume counts the time left again after Pause.
func (deadline *Deadline) Resume() {
	if !deadline.paused {
		return
	}
	deadline.arm()
}

// Stop releases the timer once the runtime is done.
func (deadline *Deadline) Stop() {
	deadline.timer.Stop()
}

// Package sandbox executes the obfuscator's own string-array/rotator/decoder
// code in a goja VM to obtain decoded string values. This is the fallback path
// when static decoding cannot handle custom encoders (webcrack's approach).
//
// Safety measures: compact setup code generation (to survive self-defending
// fn.toString() regex checks), per-call Interrupt timeout, capped call stack,
// deterministic rand/time sources, no setInterval/setTimeout host APIs.
package sandbox

import (
	"fmt"
	"time"

	"github.com/dop251/goja"
)

// Run executes setupCode (defining the string array, rotator and decoder
// functions) plus a batch of call expressions, returning the decoded values.
// Each callExpr is JavaScript source that should evaluate to a string (or
// other value) when run inside the sandbox with setupCode already defined.
//
// timeout bounds the whole execution; on timeout or error the corresponding
// result is nil and the error is returned alongside.
func Run(setupCode string, callExprs []string, timeout time.Duration) ([]any, error) {
	if len(callExprs) == 0 {
		return nil, nil
	}
	vm := goja.New()
	vm.SetMaxCallStackSize(512)
	vm.SetRandSource(func() float64 { return 0.5 })
	vm.SetTimeSource(func() time.Time { return time.Unix(0, 0).UTC() })
	// Stub console so decoder side-code that logs doesn't throw.
	stubConsole := map[string]any{
		"log":   func(goja.FunctionCall) goja.Value { return goja.Undefined() },
		"error": func(goja.FunctionCall) goja.Value { return goja.Undefined() },
		"warn":  func(goja.FunctionCall) goja.Value { return goja.Undefined() },
		"info":  func(goja.FunctionCall) goja.Value { return goja.Undefined() },
	}
	vm.Set("console", stubConsole)

	// Compile setup once.
	if _, err := vm.RunString(setupCode); err != nil {
		return nil, fmt.Errorf("sandbox setup: %w", err)
	}

	// Watchdog: interrupt the VM after timeout.
	stop := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		select {
		case <-stop:
		default:
			vm.Interrupt("jd sandbox timeout")
		}
	})
	defer func() {
		timer.Stop()
		close(stop)
	}()

	// Build a batch expression that evaluates all calls and returns an array.
	batch := "(["
	for i, expr := range callExprs {
		if i > 0 {
			batch += ","
		}
		batch += "(" + expr + ")"
	}
	batch += "])"

	v, err := vm.RunString(batch)
	if err != nil {
		return nil, fmt.Errorf("sandbox exec: %w", err)
	}

	out := make([]any, len(callExprs))
	if arr, ok := v.Export().([]any); ok {
		for i := 0; i < len(out) && i < len(arr); i++ {
			out[i] = arr[i]
		}
	}
	return out, nil
}

// DecodeString extracts a Go string from a sandbox result value. Non-string
// values return "" and false.
func DecodeString(v any) (string, bool) {
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

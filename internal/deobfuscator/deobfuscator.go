// Package deobfuscator is the top-level API: parse → prepare → deobfuscate →
// unminify → post → generate. Pipeline mirrors webcrack's src/index.ts.
package deobfuscator

import (
	"time"
)

// SandboxMode selects how the decoder is executed.
type SandboxMode int

const (
	// SandboxAuto runs static simulation first, falling back to the goja
	// sandbox when the static path cannot fully decode.
	SandboxAuto SandboxMode = iota
	// SandboxOnly skips static simulation and always executes the decoder
	// in the goja sandbox.
	SandboxOnly
	// SandboxOff disables execution entirely; only static simulation runs.
	SandboxOff
)

// Options controls a deobfuscation run.
type Options struct {
	Deobfuscate bool
	Unminify    bool
	Sandbox     SandboxMode
	Timeout     time.Duration
	Verbose     bool
}

// Result holds the deobfuscated source and any non-fatal warnings.
type Result struct {
	Code     string
	Warnings []string
}

// Deobfuscate runs the full pipeline on src and returns the result.
func Deobfuscate(src string, opts Options) (*Result, error) {
	return DeobfuscatePipeline(src, opts)
}

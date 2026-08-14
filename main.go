// Package main is the entry point for the jd JavaScript deobfuscator.
//
// jd is a Go port of webcrack (https://github.com/j4k0xb/webcrack) and
// synchrony (https://github.com/relative/synchrony). Phase 1 targets
// obfuscator.io deobfuscation plus readability unminification.
package main

import "github.com/ejfkdev/jd/internal/cli"

func main() { cli.Main() }

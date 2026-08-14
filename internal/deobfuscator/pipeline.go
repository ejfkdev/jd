// pipeline.go implements the full deobfuscation pipeline:
//
//	parse → prepare → deobfuscate → unminify → post → generate
package deobfuscator

import (
	"strings"

	"github.com/dop251/goja/parser"
	"github.com/ejfkdev/jd/internal/codegen"
	"github.com/ejfkdev/jd/internal/deobfuscate"
	"github.com/ejfkdev/jd/internal/unminify"
)

// DeobfuscatePipeline replaces the placeholder with the real implementation.
func DeobfuscatePipeline(src string, opts Options) (*Result, error) {
	// goja's parser does not support ES module syntax (import/export). We
	// pre-extract import/export statements, parse the remaining script, run
	// transforms, then prepend the import/export statements to the output.
	preamble, body := splitModuleStatements(src)

	var warnings []string
	prog, err := parser.ParseFile(nil, "", body, parser.IgnoreRegExpErrors)
	if err != nil {
		// Parser had errors (typically regex literals in Monaco language
		// definitions, or top-level await). Format at source level — splitting
		// on semicolons only, preserving regex/string/comment literals.
		// This is safe because ; inside regex/strings is skipped by the tokenizer.
		warnings = append(warnings, "parse errors detected, source-level formatting applied")
		out := formatSourcePassthrough(src)
		if preamble != "" {
			out = preamble + "\n" + out
		}
		return &Result{Code: out, Warnings: warnings}, nil
	}

	// 1. Deobfuscate (obfuscator.io).
	if opts.Deobfuscate {
		w := deobfuscate.Pipeline(prog, opts.Timeout)
		warnings = append(warnings, w...)
	}

	// 2. Unminify.
	if opts.Unminify {
		w := unminify.Pipeline(prog)
		warnings = append(warnings, w...)
	}

	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModePretty, Source: body})
	// Post-process: fix number-dot-access syntax errors (e.g. 1024.toFixed →
	// (1024).toFixed) that arise when raw-literals converts hex to decimal and
	// the expression was a BadExpression in goja's parser.
	out = fixNumberDotInCode(out)
	if preamble != "" {
		out = preamble + "\n" + out
	}
	return &Result{Code: out, Warnings: warnings}, nil
}

// fixNumberDotInCode wraps number-dot-identifier patterns in parens:
// "1024.toFixed" → "(1024).toFixed". It scans the code, skipping string
// literals, template literals, comments, and regex literals.
func fixNumberDotInCode(s string) string {
	var out []byte
	i := 0
	for i < len(s) {
		c := s[i]
		// Skip string literals.
		if c == '"' || c == '\'' {
			end := i + 1
			for end < len(s) {
				if s[end] == '\\' && end+1 < len(s) {
					end += 2
					continue
				}
				if s[end] == c {
					end++
					break
				}
				end++
			}
			out = append(out, s[i:end]...)
			i = end
			continue
		}
		// Skip template literals.
		if c == '`' {
			end := i + 1
			for end < len(s) {
				if s[end] == '\\' && end+1 < len(s) {
					end += 2
					continue
				}
				if s[end] == '`' {
					end++
					break
				}
				end++
			}
			out = append(out, s[i:end]...)
			i = end
			continue
		}
		// Skip comments.
		if c == '/' && i+1 < len(s) {
			if s[i+1] == '/' {
				end := i
				for end < len(s) && s[end] != '\n' {
					end++
				}
				out = append(out, s[i:end]...)
				i = end
				continue
			}
			if s[i+1] == '*' {
				end := i + 2
				for end < len(s)-1 && !(s[end] == '*' && s[end+1] == '/') {
					end++
				}
				if end < len(s)-1 {
					end += 2
				} else {
					end = len(s)
				}
				out = append(out, s[i:end]...)
				i = end
				continue
			}
			// Regex literal: / after certain context (not division).
			// Check if the previous output char indicates regex context.
			prev := byte(0)
			if len(out) > 0 {
				// Skip whitespace to find the real previous char.
				for j := len(out) - 1; j >= 0; j-- {
					if out[j] != ' ' && out[j] != '\t' && out[j] != '\n' && out[j] != '\r' {
						prev = out[j]
						break
					}
				}
			}
			if isRegexContextByte(prev) {
				end := i + 1
				inClass := false
				for end < len(s) {
					if s[end] == '\\' && end+1 < len(s) {
						end += 2
						continue
					}
					if s[end] == '[' {
						inClass = true
					}
					if s[end] == ']' {
						inClass = false
					}
					if s[end] == '/' && !inClass {
						end++
						// Skip flags.
						for end < len(s) && ((s[end] >= 'a' && s[end] <= 'z') || (s[end] >= 'A' && s[end] <= 'Z') || s[end] == '_') {
							end++
						}
						break
					}
					end++
				}
				out = append(out, s[i:end]...)
				i = end
				continue
			}
		}
		// Detect number followed by .identifier.
		if c >= '0' && c <= '9' {
			prev := byte(0)
			if len(out) > 0 {
				for j := len(out) - 1; j >= 0; j-- {
					if out[j] != ' ' && out[j] != '\t' && out[j] != '\n' && out[j] != '\r' {
						prev = out[j]
						break
					}
				}
			}
			isIdent := (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') ||
				prev == '_' || prev == '$' || (prev >= '0' && prev <= '9')
			if isIdent {
				out = append(out, c)
				i++
				continue
			}
			// Consume the number.
			start := i
			if c == '0' && i+1 < len(s) && (s[i+1] == 'x' || s[i+1] == 'X') {
				i += 2
				for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || (s[i] >= 'a' && s[i] <= 'f') || (s[i] >= 'A' && s[i] <= 'F')) {
					i++
				}
			} else {
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
				// Consume fractional part (digit after dot).
				if i < len(s) && s[i] == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
					i++
					for i < len(s) && s[i] >= '0' && s[i] <= '9' {
						i++
					}
				}
			}
			// Check: number followed by .identifier (member access).
			if i < len(s) && s[i] == '.' && i+1 < len(s) && ((s[i+1] >= 'a' && s[i+1] <= 'z') || (s[i+1] >= 'A' && s[i+1] <= 'Z') || s[i+1] == '_') {
				out = append(out, '(')
				out = append(out, s[start:i]...)
				out = append(out, ')')
			} else {
				out = append(out, s[start:i]...)
			}
			continue
		}
		out = append(out, c)
		i++
	}
	return string(out)
}

// isRegexContextByte reports whether a / following this byte is likely a
// regex literal (not division).
func isRegexContextByte(c byte) bool {
	return c == '(' || c == ',' || c == '=' || c == ':' || c == '[' || c == '!' ||
		c == '&' || c == '|' || c == '<' || c == '>' || c == '+' || c == '-' ||
		c == '*' || c == '%' || c == '~' || c == '^' || c == '?' || c == '{' ||
		c == ';' || c == '\n' || c == 0
}

// splitModuleStatements extracts import/export statements from src (which
// goja cannot parse) and returns them as a preamble plus the remaining body.
// It uses a lightweight tokenizer to skip string/regex/comment literals and
// find import/export keywords at statement boundaries.
func splitModuleStatements(src string) (preamble, body string) {
	var extracted []string
	s := src
	out := make([]byte, 0, len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		// Skip string literals.
		if c == '"' || c == '\'' {
			end := skipString(s, i)
			out = append(out, s[i:end]...)
			i = end
			continue
		}
		// Skip template literals.
		if c == '`' {
			end := skipTemplate(s, i)
			out = append(out, s[i:end]...)
			i = end
			continue
		}
		// Skip comments (don't write to out — they don't affect parsing).
		if c == '/' && i+1 < len(s) {
			if s[i+1] == '/' {
				end := indexOfByte(s, '\n', i)
				if end < 0 {
					end = len(s)
				}
				i = end
				continue
			}
			if s[i+1] == '*' {
				end := indexOfStr(s, "*/", i+2)
				if end < 0 {
					end = len(s)
				} else {
					end += 2
				}
				i = end
				continue
			}
		}
		// Check for import/export at statement boundary.
		if isStmtBoundary(i, s, out) {
			if _, matched := matchModuleKeyword(s, i); matched {
				stmt, end := extractUntilSemiOrNewline(s, i)
				extracted = append(extracted, stmt)
				out = append(out, ';')
				i = end
				continue
			}
		}
		out = append(out, c)
		i++
	}
	preamble = strings.Join(extracted, "\n")
	body = string(out)
	return preamble, body
}

func isStmtBoundary(i int, s string, out []byte) bool {
	if i == 0 {
		return true
	}
	for j := len(out) - 1; j >= 0; j-- {
		c := out[j]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c == ';' || c == '}' || c == '{' || c == '(' || c == ')' || c == ',' || c == '\n'
	}
	return true
}

func matchModuleKeyword(s string, i int) (string, bool) {
	// Match import declarations (import x from "y", import {x} from, import "y")
	// but NOT dynamic import expressions: import("y") — those have '(' after 'import'.
	if strings.HasPrefix(s[i:], "import") {
		next := byte(0)
		if i+6 < len(s) {
			next = s[i+6]
		}
		// Import declarations: followed by space, {, ", '
		if next == ' ' || next == '\t' || next == '\n' || next == '{' || next == '"' || next == '\'' {
			return "import", true
		}
	}
	if strings.HasPrefix(s[i:], "export") {
		next := byte(0)
		if i+6 < len(s) {
			next = s[i+6]
		}
		if next == ' ' || next == '\t' || next == '\n' || next == '{' {
			return "export", true
		}
	}
	return "", false
}

func extractUntilSemiOrNewline(s string, idx int) (string, int) {
	i := idx
	inStr := byte(0)
	for i < len(s) {
		c := s[i]
		if inStr != 0 {
			if c == inStr && (i == 0 || s[i-1] != '\\') {
				inStr = 0
			}
			i++
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			inStr = c
			i++
			continue
		}
		if c == ';' {
			return s[idx : i+1], i + 1
		}
		if c == '\n' {
			return s[idx:i], i
		}
		i++
	}
	return s[idx:], len(s)
}

func skipString(s string, i int) int {
	quote := s[i]
	i++
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] == quote {
			return i + 1
		}
		i++
	}
	return len(s)
}

func skipTemplate(s string, i int) int {
	i++ // skip `
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] == '`' {
			return i + 1
		}
		// Skip ${...}
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			i += 2
			depth := 1
			for i < len(s) && depth > 0 {
				if s[i] == '{' {
					depth++
				} else if s[i] == '}' {
					depth--
				}
				i++
			}
			continue
		}
		i++
	}
	return len(s)
}

func indexOfByte(s string, b byte, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func indexOfStr(s, sub string, start int) int {
	for i := start; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

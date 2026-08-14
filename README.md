# jd

[中文](README.zh-CN.md)

jd is a Go tool that deobfuscates javascript-obfuscator (obfuscator.io) output and unminifies JavaScript. It is a Go port of [webcrack](https://github.com/j4k0xb/webcrack) and [synchrony](https://github.com/relative/synchrony).

## Features

### Obfuscator.io Deobfuscation

Detects and reverses javascript-obfuscator output:

- **String array** detection (function-wrapped and simple array forms, `var`/`const`/`let`)
- **Array rotator** detection (push/shift IIFE with break condition)
- **Decoder** detection (index-shift, Base64/RC4/custom encodings, two-parameter decoders)
- **Wrapper alias** resolution (var/function aliases to decoders)
- **Hybrid decoding** — static simulation (SIMPLE/Base64/RC4) with goja sandbox fallback for custom encoders
- **Helper removal** — string array, rotator, and decoder declarations removed after decoding

### Unminify

20+ readability transforms:

| Transform | Example |
|---|---|
| computed-properties | `console["log"]` → `console.log` |
| merge-strings | `"a" + "b"` → `"ab"` |
| unminify-booleans | `!0` → `true`, `!1` → `false` |
| number-expressions | `1 + 2` → `3` |
| void-to-undefined | `void 0` → `undefined` |
| raw-literals | `0x1` → `1` |
| sequence | `a(), b(), c()` → separate statements |
| split-variable-declarations | `var a=1, b=2` → `var a=1; var b=2` |
| block-statements | wrap single-statement bodies in `{ }` |
| logical-to-if | `a && b()` → `if (a) b()` |
| ternary-to-if | `a ? b() : c()` → `if (a) b() else c()` |
| merge-else-if | `else { if (...) }` → `else if (...)` |
| for-to-while | `for(;;)` → `while(true)` |
| yoda | `5 === x` → `x === 5` |
| infinity | `1/0` → `Infinity` |
| invert-boolean-logic | `!(a == b)` → `a != b` |
| unary-expressions | drop no-op `void`/`!`/`typeof` at statement level |
| remove-double-not | `!!true` → `true` |

### ES Module Support

goja's parser does not support ES module syntax (`import`/`export`). jd pre-extracts import/export statements, parses the remaining script, runs transforms, then prepends the statements to the output. Dynamic `import()` expressions are correctly distinguished from import declarations.

### Regex-Safe Formatting

For files goja cannot fully parse (e.g. Monaco editor language definitions with heavy regex usage), jd falls back to source-level formatting: a tokenizer splits on semicolons while preserving regex literals, strings, and comments verbatim.

## Installation

### macOS (Homebrew)

```bash
brew install ejfkdev/tap/jd
```

### Pre-built binaries

Download from [GitHub Releases](https://github.com/ejfkdev/jd/releases) (Linux/macOS/Windows, x86_64/ARM64).

### Build from source

```bash
go build -o jd .
```

## Usage

```bash
# Deobfuscate a file (output to stdout)
jd obfuscated.js

# Write to a file
jd obfuscated.js -o cleaned.js

# Read from stdin
cat obfuscated.js | jd -

# Process a directory — recursively processes all .js/.mjs/.cjs files,
# mirrors the directory tree to the output directory.
jd src/ -o dist/

# Directory mode with default output: creates <input>-deobfuscated
jd src/

# Specify file extensions
jd src/ -o dist/ --ext .js,.mjs

# Parallel processing (N workers, default: number of CPUs)
jd src/ -o dist/ -j 8

# Skip non-code files (only output processed JS)
jd src/ -o dist/ --copy-noncode=false

# Only deobfuscate, skip unminify
jd --unminify=false obfuscated.js

# JSON output with warnings
jd --json obfuscated.js
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `-o, --output` | stdout | output file or directory (default for dir: `<input>-deobfuscated`) |
| `--deobfuscate` | true | run obfuscator.io deobfuscation |
| `--unminify` | true | run readability transforms |
| `--sandbox` | auto | decoder execution: `auto` (static then sandbox), `only` (always sandbox), `off` (only static) |
| `--timeout` | 10s | per-file sandbox timeout |
| `--ext` | .js,.mjs,.cjs | file extensions to process in directory mode |
| `-j, --workers` | 0 (=CPU) | parallel workers for directory mode |
| `--copy-noncode` | true | copy non-code files to output directory |
| `-v, --verbose` | false | print diagnostics to stderr |
| `--json` | false | emit `{code, warnings}` JSON |

## Architecture

```
jd/
├── main.go                     # CLI entry
├── internal/
│   ├── cli/                    # cobra CLI (i18n: English/Chinese)
│   ├── deobfuscator/           # top-level pipeline + ES module preprocessing
│   ├── jsast/                  # AST walker (Cursor, Replace, Remove, Clone)
│   ├── codegen/                # AST → JavaScript printer (pretty/compact)
│   ├── scope/                  # lexical scope & binding analysis
│   ├── sandbox/                # goja VM wrapper for decoder execution
│   ├── decoder/                # static decoding: Base64/RC4/rotation
│   ├── deobfuscate/            # string-array/rotator/decoder detection + transforms
│   ├── unminify/               # 20 readability transforms + fixpoint runner
│   └── transform/              # Transform abstraction + ApplyFixpoint
└── testdata/samples/           # test fixtures
```

### Pipeline

```
parse → splitModuleStatements → deobfuscate → unminify → generate
```

1. **Parse** — goja parser with `IgnoreRegExpErrors`; falls back to source-level formatting for unparseable files
2. **Deobfuscate** — detect obfuscator.io string array/rotator/decoders, decode all call sites (static first, sandbox fallback), remove helpers
3. **Unminify** — fixpoint loop (≤20 passes) of merged readability transforms
4. **Generate** — pretty-mode codegen with operator precedence and correct parenthesisation

### Hybrid Decoding

The decoder uses a two-stage approach:

1. **Static path** (synchrony-style): simulate array rotation via push/shift, decode SIMPLE/Base64/RC4 in pure Go — fast and deterministic.
2. **Sandbox fallback** (webcrack-style): extract the string array, rotator, and decoder code, execute in a goja VM with `Interrupt` timeout, `SetMaxCallStackSize`, deterministic rand/time, and no setInterval/setTimeout — handles custom encoders the static path can't decode.

## Dependencies

- [goja](https://github.com/dop251/goja) — pure-Go JavaScript parser & runtime (no CGO)
- [cobra](https://github.com/spf13/cobra) — CLI framework

## License

MIT

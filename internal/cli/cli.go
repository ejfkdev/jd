// Package cli implements the jd command-line interface with i18n support.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ejfkdev/jd/internal/deobfuscator"
)

// Version is the jd version, set at build time via -ldflags.
var Version = "dev"

// Options parsed from CLI flags.
type Options struct {
	output      string
	deobfuscate bool
	unminify    bool
	sandboxMode string
	timeout     time.Duration
	verbose     bool
	json        bool
	extensions  []string
	workers     int
	copyNonCode bool
}

// Main parses args and runs the deobfuscator.
func Main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	opts := &Options{}
	root := &cobra.Command{
		Use:                   "jd [file|directory...]",
		Short:                 t("JavaScript deobfuscator (obfuscator.io + unminify)", "JavaScript 反混淆工具 (obfuscator.io + unminify)"),
		Long:                  longHelp(),
		Example:               exampleHelp(),
		Version:               Version,
		DisableFlagsInUseLine: true,
		Args:                  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return run(cmd, args, opts)
		},
	}
	root.Flags().StringVarP(&opts.output, "output", "o", "",
		t("output file or directory (default: stdout for single file; <input>-deobfuscated for directory)",
			"输出文件或目录 (默认: 单文件输出到 stdout; 目录输出到 <输入目录>-deobfuscated)"))
	root.Flags().BoolVar(&opts.deobfuscate, "deobfuscate", true,
		t("run obfuscator.io deobfuscation", "运行 obfuscator.io 反混淆"))
	root.Flags().BoolVar(&opts.unminify, "unminify", true,
		t("run readability transforms", "运行可读性还原变换"))
	root.Flags().StringVar(&opts.sandboxMode, "sandbox", "auto",
		t("decoder execution: auto|only|off", "解码器执行方式: auto|only|off"))
	root.Flags().DurationVar(&opts.timeout, "timeout", 10*time.Second,
		t("per-file sandbox timeout", "单文件沙箱超时时间"))
	root.Flags().BoolVarP(&opts.verbose, "verbose", "v", false,
		t("print diagnostics to stderr", "输出诊断信息到 stderr"))
	root.Flags().BoolVar(&opts.json, "json", false,
		t("emit {code, warnings} JSON to stdout", "输出 {code, warnings} JSON 到 stdout"))
	root.Flags().StringSliceVar(&opts.extensions, "ext", []string{".js", ".mjs", ".cjs"},
		t("file extensions to process in directory mode (comma-separated)", "目录模式下处理的文件扩展名 (逗号分隔)"))
	root.Flags().IntVarP(&opts.workers, "workers", "j", 0,
		t("parallel workers for directory mode (0 = number of CPUs)", "目录模式并行 worker 数 (0 = CPU 核数)"))
	root.Flags().BoolVar(&opts.copyNonCode, "copy-noncode", true,
		t("copy non-code files (assets, configs, etc.) to output directory", "复制非代码文件 (资源、配置等) 到输出目录"))
	return root
}

func longHelp() string {
	if isChinese() {
		return `jd 是 webcrack 和 synchrony 的 Go 移植版，用于反混淆和还原 JavaScript 代码。

支持处理单个文件或目录。输入目录时递归处理所有匹配的 JS 文件，
并将目录结构镜像到输出目录。

仓库: https://github.com/ejfkdev/jd`
	}
	return `jd is a Go port of webcrack and synchrony. It deobfuscates
javascript-obfuscator (obfuscator.io) output and unminifies JavaScript.

Accepts files and/or directories. When given a directory, recursively processes
all matching files and mirrors the directory structure to the output directory.

Repo: https://github.com/ejfkdev/jd`
}

func exampleHelp() string {
	if isChinese() {
		return `  # 反混淆单个文件 (输出到 stdout)
  jd obfuscated.js

  # 写入到文件
  jd obfuscated.js -o cleaned.js

  # 从 stdin 读取
  cat obfuscated.js | jd -

  # 处理目录 — 递归处理所有 .js/.mjs/.cjs 文件
  jd src/ -o dist/

  # 目录模式默认输出: 在输入目录同级创建 <输入目录>-deobfuscated
  jd src/

  # 指定文件扩展名
  jd src/ -o dist/ --ext .js,.mjs

  # 并行处理 (N 个 worker，默认 CPU 核数)
  jd src/ -o dist/ -j 8

  # 跳过非代码文件
  jd src/ -o dist/ --copy-noncode=false

  # 只反混淆，不做可读性还原
  jd --unminify=false obfuscated.js`
	}
	return `  # Deobfuscate a file (output to stdout)
  jd obfuscated.js

  # Write to a file
  jd obfuscated.js -o cleaned.js

  # Read from stdin
  cat obfuscated.js | jd -

  # Process a directory — recursively processes all .js/.mjs/.cjs files
  jd src/ -o dist/

  # Directory mode with default output: creates <input>-deobfuscated
  jd src/

  # Specify file extensions
  jd src/ -o dist/ --ext .js,.mjs

  # Parallel processing (N workers, default: number of CPUs)
  jd src/ -o dist/ -j 8

  # Skip non-code files
  jd src/ -o dist/ --copy-noncode=false

  # Only deobfuscate, skip unminify
  jd --unminify=false obfuscated.js`
}

// isChinese detects if the system locale is Chinese.
func isChinese() bool {
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = os.Getenv("LC_ALL")
	}
	if lang == "" {
		lang = os.Getenv("LC_MESSAGES")
	}
	return strings.Contains(lang, "zh") || strings.Contains(lang, "ZH") || strings.Contains(lang, "Chinese")
}

// t returns English or Chinese text based on system locale.
func t(en, zh string) string {
	if isChinese() {
		return zh
	}
	return en
}

func run(cmd *cobra.Command, args []string, opts *Options) error {
	if !opts.deobfuscate && !opts.unminify {
		return errors.New(t("--deobfuscate=false and --unminify=false together produce no work",
			"--deobfuscate=false 和 --unminify=false 同时使用不会产生任何效果"))
	}
	sandboxMode, err := parseSandbox(opts.sandboxMode)
	if err != nil {
		return err
	}

	// Collect all input files, expanding directories.
	var files []fileInput
	for _, arg := range args {
		if arg == "-" {
			files = append(files, fileInput{path: "-", isStdin: true})
			continue
		}
		info, err := os.Stat(arg)
		if err != nil {
			return fmt.Errorf("stat %s: %w", arg, err)
		}
		if info.IsDir() {
			dirFiles, err := collectDirFiles(arg, opts.extensions)
			if err != nil {
				return err
			}
			files = append(files, dirFiles...)
		} else {
			files = append(files, fileInput{path: arg})
		}
	}

	// stdin: single file to stdout
	if len(files) == 1 && files[0].isStdin {
		return processStdin(opts, sandboxMode)
	}

	// Single file (no directory): output to stdout or -o file
	if len(files) == 1 && !files[0].isStdin {
		if opts.output == "" || isDir(opts.output) {
			return processFileToStdout(files[0].path, opts, sandboxMode)
		}
		return processFile(files[0].path, opts.output, opts, sandboxMode)
	}

	// Directory mode: compute output dir, mirror structure
	outDir := opts.output
	if outDir == "" {
		for _, arg := range args {
			if arg != "-" {
				if info, err := os.Stat(arg); err == nil && info.IsDir() {
					outDir = arg + "-deobfuscated"
					break
				}
			}
		}
	}
	if outDir == "" {
		return errors.New(t("no output directory specified (use -o or provide a directory input)",
			"未指定输出目录 (使用 -o 或提供目录输入)"))
	}

	return processDir(files, outDir, opts, sandboxMode)
}

// fileInput represents one input file.
type fileInput struct {
	path    string
	isStdin bool
	baseDir string
	isCode  bool
}

// collectDirFiles walks dir recursively, returning all files. Code files
// (matching exts) are marked isCode=true; non-code files are marked isCode=false.
func collectDirFiles(dir string, exts []string) ([]fileInput, error) {
	var files []fileInput
	extSet := make(map[string]bool)
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "node_modules" || name == ".git" || strings.HasSuffix(name, "-deobfuscated") || strings.HasSuffix(name, "-deobfuscates") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		files = append(files, fileInput{
			path:    path,
			baseDir: dir,
			isCode:  extSet[ext],
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	return files, nil
}

func processStdin(opts *Options, sandboxMode deobfuscator.SandboxMode) error {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	res, err := deobfuscator.Deobfuscate(string(b), deobfuscator.Options{
		Deobfuscate: opts.deobfuscate,
		Unminify:    opts.unminify,
		Sandbox:     sandboxMode,
		Timeout:     opts.timeout,
		Verbose:     opts.verbose,
	})
	if err != nil {
		return err
	}
	logWarnings(opts, res)
	if opts.json {
		return emitJSON(res, opts)
	}
	return emitCode(res, opts)
}

func processFileToStdout(input string, opts *Options, sandboxMode deobfuscator.SandboxMode) error {
	src, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read %s: %w", input, err)
	}
	res, err := deobfuscator.Deobfuscate(string(src), deobfuscator.Options{
		Deobfuscate: opts.deobfuscate,
		Unminify:    opts.unminify,
		Sandbox:     sandboxMode,
		Timeout:     opts.timeout,
		Verbose:     opts.verbose,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", input, err)
	}
	logWarnings(opts, res)
	if opts.json {
		return emitJSON(res, opts)
	}
	_, err = fmt.Print(res.Code)
	return err
}

func processFile(input, output string, opts *Options, sandboxMode deobfuscator.SandboxMode) error {
	src, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read %s: %w", input, err)
	}
	res, err := deobfuscator.Deobfuscate(string(src), deobfuscator.Options{
		Deobfuscate: opts.deobfuscate,
		Unminify:    opts.unminify,
		Sandbox:     sandboxMode,
		Timeout:     opts.timeout,
		Verbose:     opts.verbose,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", input, err)
	}
	logWarnings(opts, res)
	if opts.json {
		return emitJSON(res, opts)
	}
	return os.WriteFile(output, []byte(res.Code), 0o644)
}

// processDir processes all files in parallel, mirroring the directory
// structure to outDir. Code files are processed first; then non-code files
// are copied (unless --copy-noncode=false).
func processDir(files []fileInput, outDir string, opts *Options, sandboxMode deobfuscator.SandboxMode) error {
	start := time.Now()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", outDir, err)
	}

	// Pre-create all output directories (serial, fast) so workers don't race.
	for _, fi := range files {
		outPath := outRelPath(outDir, fi)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
		}
	}

	workers := opts.workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Partition: code files first (parallel deobfuscate), then non-code (parallel copy).
	var codeFiles, otherFiles []fileInput
	for _, fi := range files {
		if fi.isCode {
			codeFiles = append(codeFiles, fi)
		} else if opts.copyNonCode {
			otherFiles = append(otherFiles, fi)
		}
	}

	fmt.Fprintf(os.Stderr, "jd: %s %d %s (%d %s)...\n",
		t("processing", "处理中"), len(codeFiles),
		t("code files", "个代码文件"), workers,
		t("workers", "个 worker"))

	codePass, codeFail := runWorkerPool(codeFiles, outDir, opts, workers,
		func(fi fileInput, outPath string) error {
			return processFile(fi.path, outPath, opts, sandboxMode)
		})

	otherPass, otherFail := 0, 0
	if len(otherFiles) > 0 {
		fmt.Fprintf(os.Stderr, "jd: %s %d %s...\n",
			t("copying", "复制中"), len(otherFiles),
			t("non-code files", "个非代码文件"))
		otherPass, otherFail = runWorkerPool(otherFiles, outDir, opts, workers,
			func(fi fileInput, outPath string) error {
				return copyFile(fi.path, outPath)
			})
	}

	elapsed := time.Since(start)
	absOut, _ := filepath.Abs(outDir)
	fmt.Fprintf(os.Stderr, "\njd: %s %s\n", t("done in", "完成，耗时"), elapsed.Round(time.Millisecond))
	fmt.Fprintf(os.Stderr, "  %s: %s\n", t("output", "输出目录"), absOut)
	fmt.Fprintf(os.Stderr, "  %s: %d %s (%d ok",
		t("code files", "代码文件"), len(codeFiles), t("processed", "已处理"), codePass)
	if codeFail > 0 {
		fmt.Fprintf(os.Stderr, ", %d %s", codeFail, t("failed", "失败"))
	}
	fmt.Fprintf(os.Stderr, ")\n")
	if len(otherFiles) > 0 {
		fmt.Fprintf(os.Stderr, "  %s: %d %s (%d ok",
			t("non-code files", "非代码文件"), len(otherFiles), t("copied", "已复制"), otherPass)
		if otherFail > 0 {
			fmt.Fprintf(os.Stderr, ", %d %s", otherFail, t("failed", "失败"))
		}
		fmt.Fprintf(os.Stderr, ")\n")
	}
	fmt.Fprintf(os.Stderr, "  %s: %d\n", t("total files", "总文件数"), len(codeFiles)+len(otherFiles))
	return nil
}

// outRelPath computes the output path for a file input.
func outRelPath(outDir string, fi fileInput) string {
	rel, err := filepath.Rel(fi.baseDir, fi.path)
	if err != nil {
		rel = filepath.Base(fi.path)
	}
	return filepath.Join(outDir, rel)
}

// runWorkerPool runs fn over files in parallel using a worker pool of n workers.
func runWorkerPool(files []fileInput, outDir string, opts *Options, workers int,
	fn func(fi fileInput, outPath string) error) (int, int) {
	if len(files) == 0 {
		return 0, 0
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}

	type result struct {
		path string
		err  error
	}
	jobs := make(chan fileInput, len(files))
	results := make(chan result, len(files))

	for i := 0; i < workers; i++ {
		go func() {
			for fi := range jobs {
				outPath := outRelPath(outDir, fi)
				if opts.verbose {
					fmt.Fprintf(os.Stderr, "processing: %s -> %s\n", fi.path, outPath)
				}
				err := fn(fi, outPath)
				if err != nil && fi.isCode {
					src, _ := os.ReadFile(fi.path)
					_ = os.WriteFile(outPath, src, 0o644)
				}
				results <- result{path: fi.path, err: err}
			}
		}()
	}
	for _, fi := range files {
		jobs <- fi
	}
	close(jobs)

	var pass, fail int
	for i := 0; i < len(files); i++ {
		r := <-results
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s: %v\n", r.path, r.err)
			fail++
		} else {
			pass++
		}
	}
	return pass, fail
}

// copyFile copies src to dst, preserving file mode.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func parseSandbox(s string) (deobfuscator.SandboxMode, error) {
	switch strings.ToLower(s) {
	case "auto":
		return deobfuscator.SandboxAuto, nil
	case "only":
		return deobfuscator.SandboxOnly, nil
	case "off":
		return deobfuscator.SandboxOff, nil
	default:
		return 0, fmt.Errorf(t("invalid --sandbox %q (want auto|only|off)", "无效的 --sandbox %q (应为 auto|only|off)"), s)
	}
}

func logWarnings(opts *Options, res *deobfuscator.Result) {
	if opts.verbose && len(res.Warnings) > 0 {
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
	}
}

func emitCode(res *deobfuscator.Result, opts *Options) error {
	if opts.output == "" {
		_, err := fmt.Print(res.Code)
		return err
	}
	return os.WriteFile(opts.output, []byte(res.Code), 0o644)
}

func emitJSON(res *deobfuscator.Result, opts *Options) error {
	out, err := json.MarshalIndent(struct {
		Code     string   `json:"code"`
		Warnings []string `json:"warnings"`
	}{res.Code, res.Warnings}, "", "  ")
	if err != nil {
		return err
	}
	if opts.output == "" {
		fmt.Println(string(out))
		return nil
	}
	return os.WriteFile(opts.output, append(out, '\n'), 0o644)
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

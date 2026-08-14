// Runner — CLI argument parsing (clap), file/directory processing.

use std::fs;
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};
use std::process;
use std::time::Instant;
use std::collections::VecDeque;

use clap::{Parser, ValueHint};

use crate::deobfuscate;

#[derive(Parser, Debug, Clone)]
#[command(
    name = "jd",
    version,
    about = "JavaScript deobfuscator (obfuscator.io + unminify)",
    disable_help_subcommand = true,
)]
pub struct Cli {
    /// Input file or directory (use "-" for stdin)
    #[arg(value_name = "INPUT", value_hint = ValueHint::AnyPath, num_args = 0..)]
    input: Vec<String>,

    /// Output file or directory
    #[arg(short = 'o', long = "output", value_hint = ValueHint::AnyPath)]
    output: Option<String>,

    /// Run obfuscator.io deobfuscation
    #[arg(long, default_value = "true")]
    deobfuscate: bool,

    /// Run readability transforms
    #[arg(long, default_value = "true")]
    unminify: bool,

    /// Decoder execution: auto|only|off
    #[arg(long, default_value = "auto")]
    sandbox: String,

    /// Per-file sandbox timeout
    #[arg(long, default_value = "10s")]
    timeout: String,

    /// File extensions to process in directory mode
    #[arg(long, value_delimiter = ',', default_value = ".js,.mjs,.cjs")]
    ext: Vec<String>,

    /// Parallel workers for directory mode (0 = number of CPUs)
    #[arg(short = 'j', long, default_value = "0")]
    workers: usize,

    /// Copy non-code files to output directory
    #[arg(long, default_value = "true")]
    copy_noncode: bool,

    /// Print diagnostics to stderr
    #[arg(short = 'v', long)]
    verbose: bool,

    /// Emit {code, warnings} JSON
    #[arg(long)]
    json: bool,
}

pub fn run() {
    let cli = Cli::parse();
    if cli.input.is_empty() {
        Cli::parse_from(["jd"]);
        return;
    }
    if let Err(e) = execute(&cli) {
        eprintln!("{e}");
        process::exit(1);
    }
}

fn execute(cli: &Cli) -> Result<(), Box<dyn std::error::Error>> {
    if !cli.deobfuscate && !cli.unminify {
        return Err("both --deobfuscate=false and --unminify=false, nothing to do".into());
    }

    let sandbox_mode = parse_sandbox(&cli.sandbox)?;
    let opts = deobfuscate::Options {
        deobfuscate: cli.deobfuscate,
        unminify: cli.unminify,
        sandbox: sandbox_mode,
        timeout: parse_duration(&cli.timeout)?,
        verbose: cli.verbose,
    };

    let mut files = Vec::new();
    for input in &cli.input {
        if input == "-" {
            return process_stdin(cli, &opts);
        }
        let path = Path::new(input);
        if path.is_dir() {
            collect_dir_files(path, &cli.ext, &mut files)?;
        } else {
            files.push(FileInput {
                path: path.to_path_buf(),
                base_dir: path.parent().unwrap_or(Path::new("")).to_path_buf(),
                is_code: true,
            });
        }
    }

    if files.len() == 1 {
        if let Some(ref out) = cli.output {
            if !Path::new(out).is_dir() {
                return process_file(&files[0].path, Path::new(out), cli, &opts);
            }
        }
        return process_file_to_stdout(&files[0].path, cli, &opts);
    }

    let out_dir = compute_out_dir(cli, &files);
    process_dir(&files, &out_dir, cli, &opts)?;
    Ok(())
}

struct FileInput {
    path: PathBuf,
    base_dir: PathBuf,
    is_code: bool,
}

fn collect_dir_files(dir: &Path, exts: &[String], files: &mut Vec<FileInput>) -> Result<(), Box<dyn std::error::Error>> {
    let ext_set: std::collections::HashSet<String> = exts.iter().map(|e| e.to_lowercase()).collect();
    let paths = walkdir(dir)?;
    for path in paths {
        let ext = path.extension().and_then(|e| e.to_str()).map(|e| format!(".{}", e.to_lowercase())).unwrap_or_default();
        files.push(FileInput {
            path: path.clone(),
            base_dir: dir.to_path_buf(),
            is_code: ext_set.contains(&ext),
        });
    }
    Ok(())
}

fn walkdir(dir: &Path) -> Result<Vec<PathBuf>, Box<dyn std::error::Error>> {
    let mut result = Vec::new();
    let mut queue = VecDeque::new();
    queue.push_back(dir.to_path_buf());
    while let Some(d) = queue.pop_front() {
        for entry in fs::read_dir(&d)? {
            let entry = entry?;
            let path = entry.path();
            if path.is_dir() {
                let name = path.file_name().and_then(|n| n.to_str()).unwrap_or("");
                if name == "node_modules" || name == ".git" || name.ends_with("-deobfuscated") || name.ends_with("-deobfuscates") {
                    continue;
                }
                queue.push_back(path);
            } else {
                result.push(path);
            }
        }
    }
    result.sort();
    Ok(result)
}

fn process_stdin(cli: &Cli, opts: &deobfuscate::Options) -> Result<(), Box<dyn std::error::Error>> {
    let mut src = String::new();
    io::stdin().read_to_string(&mut src)?;
    let res = deobfuscate::deobfuscate(&src, opts);
    output_result(&res, cli, &mut io::stdout())
}

fn process_file_to_stdout(path: &Path, cli: &Cli, opts: &deobfuscate::Options) -> Result<(), Box<dyn std::error::Error>> {
    let src = fs::read_to_string(path)?;
    let res = deobfuscate::deobfuscate(&src, opts);
    output_result(&res, cli, &mut io::stdout())
}

fn process_file(input: &Path, output: &Path, _cli: &Cli, opts: &deobfuscate::Options) -> Result<(), Box<dyn std::error::Error>> {
    let src = fs::read_to_string(input)?;
    let res = deobfuscate::deobfuscate(&src, opts);
    fs::write(output, &res.code)?;
    Ok(())
}

fn output_result<W: Write>(res: &deobfuscate::Result, cli: &Cli, w: &mut W) -> Result<(), Box<dyn std::error::Error>> {
    if cli.json {
        let json = serde_json::json!({"code": res.code, "warnings": res.warnings});
        writeln!(w, "{}", serde_json::to_string_pretty(&json)?)?;
    } else {
        w.write_all(res.code.as_bytes())?;
    }
    Ok(())
}

fn process_dir(files: &[FileInput], out_dir: &Path, cli: &Cli, opts: &deobfuscate::Options) -> Result<(), Box<dyn std::error::Error>> {
    let start = Instant::now();
    fs::create_dir_all(out_dir)?;

    for fi in files {
        let out_path = out_rel_path(out_dir, fi);
        if let Some(parent) = out_path.parent() {
            fs::create_dir_all(parent)?;
        }
    }

    let workers = if cli.workers == 0 { num_cpus() } else { cli.workers.min(files.len().max(1)) };

    let (code_files, other_files): (Vec<&FileInput>, Vec<&FileInput>) = files.iter().partition(|f| f.is_code);

    eprintln!("jd: processing {} code files ({} workers)...", code_files.len(), workers);

    let mut code_pass = 0usize;
    let mut code_fail = 0usize;

    for fi in &code_files {
        let out_path = out_rel_path(out_dir, fi);
        if cli.verbose {
            eprintln!("processing: {} -> {}", fi.path.display(), out_path.display());
        }
        match process_file(&fi.path, &out_path, cli, opts) {
            Ok(()) => code_pass += 1,
            Err(e) => {
                eprintln!("FAIL: {}: {}", fi.path.display(), e);
                let _ = fs::copy(&fi.path, &out_path);
                code_fail += 1;
            }
        }
    }

    let mut other_pass = 0usize;
    let mut other_fail = 0usize;
    if cli.copy_noncode && !other_files.is_empty() {
        eprintln!("jd: copying {} non-code files...", other_files.len());
        for fi in &other_files {
            let out_path = out_rel_path(out_dir, fi);
            match fs::copy(&fi.path, &out_path) {
                Ok(_) => other_pass += 1,
                Err(e) => {
                    eprintln!("FAIL: {}: {}", fi.path.display(), e);
                    other_fail += 1;
                }
            }
        }
    }

    let elapsed = start.elapsed();
    eprintln!("\njd: done in {:.3}s", elapsed.as_secs_f64());
    let abs_out = fs::canonicalize(out_dir).unwrap_or_else(|_| out_dir.to_path_buf());
    eprintln!("  output: {}", abs_out.display());
    eprintln!("  code files: {} processed ({} ok{})", code_files.len(), code_pass,
        if code_fail > 0 { format!(", {} failed", code_fail) } else { String::new() });
    if !other_files.is_empty() {
        eprintln!("  non-code files: {} copied ({} ok{})", other_files.len(), other_pass,
            if other_fail > 0 { format!(", {} failed", other_fail) } else { String::new() });
    }
    eprintln!("  total files: {}", code_files.len() + other_files.len());

    Ok(())
}

fn out_rel_path(out_dir: &Path, fi: &FileInput) -> PathBuf {
    let rel = fi.path.strip_prefix(&fi.base_dir).unwrap_or(&fi.path);
    out_dir.join(rel)
}

fn compute_out_dir(cli: &Cli, files: &[FileInput]) -> PathBuf {
    if let Some(ref out) = cli.output {
        return PathBuf::from(out);
    }
    for fi in files {
        let dir = fi.base_dir.clone();
        if let Some(name) = dir.file_name() {
            return dir.with_file_name(format!("{}-deobfuscated", name.to_string_lossy()));
        }
    }
    PathBuf::from("jd-deobfuscated")
}

fn parse_sandbox(s: &str) -> Result<deobfuscate::SandboxMode, Box<dyn std::error::Error>> {
    match s.to_lowercase().as_str() {
        "auto" => Ok(deobfuscate::SandboxMode::Auto),
        "only" => Ok(deobfuscate::SandboxMode::Only),
        "off" => Ok(deobfuscate::SandboxMode::Off),
        _ => Err(format!("invalid --sandbox {s} (want auto|only|off)").into()),
    }
}

fn parse_duration(s: &str) -> Result<std::time::Duration, Box<dyn std::error::Error>> {
    if let Ok(secs) = s.trim_end_matches('s').parse::<u64>() {
        return Ok(std::time::Duration::from_secs(secs));
    }
    if s.ends_with("ms") {
        if let Ok(ms) = s.trim_end_matches("ms").parse::<u64>() {
            return Ok(std::time::Duration::from_millis(ms));
        }
    }
    Ok(std::time::Duration::from_secs(10))
}

fn num_cpus() -> usize {
    std::thread::available_parallelism().map(|n| n.get()).unwrap_or(4)
}

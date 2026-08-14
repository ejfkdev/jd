# jd

[English](README.md)

jd 是一个 Go 工具，用于反混淆 javascript-obfuscator (obfuscator.io) 输出并还原 JavaScript 代码的可读性。它是 [webcrack](https://github.com/j4k0xb/webcrack) 和 [synchrony](https://github.com/relative/synchrony) 的 Go 移植版。

## 功能

### obfuscator.io 反混淆

检测并还原 javascript-obfuscator 输出：

- **字符串数组**检测 (函数包装形式和简单数组形式，支持 `var`/`const`/`let`)
- **数组旋转器**检测 (push/shift IIFE + break 条件)
- **解码器**检测 (索引偏移、Base64/RC4/自定义编码、双参数解码器)
- **包装器别名**解析 (var/函数别名)
- **混合解码** — 静态模拟 (SIMPLE/Base64/RC4) 优先，goja 沙箱兜底支持自定义编码
- **辅助函数移除** — 解码后移除字符串数组、旋转器和解码器声明

### 可读性还原 (Unminify)

20+ 个可读性变换：

| 变换 | 示例 |
|---|---|
| 计算属性还原 | `console["log"]` → `console.log` |
| 字符串合并 | `"a" + "b"` → `"ab"` |
| 布尔值还原 | `!0` → `true`, `!1` → `false` |
| 数字表达式折叠 | `1 + 2` → `3` |
| void 转 undefined | `void 0` → `undefined` |
| 原始字面量 | `0x1` → `1` |
| 序列拆分 | `a(), b(), c()` → 独立语句 |
| 变量声明拆分 | `var a=1, b=2` → `var a=1; var b=2` |
| 块语句包裹 | 单语句体包裹 `{ }` |
| 逻辑转 if | `a && b()` → `if (a) b()` |
| 三元转 if | `a ? b() : c()` → `if (a) b() else c()` |
| 合并 else-if | `else { if (...) }` → `else if (...)` |
| for 转 while | `for(;;)` → `while(true)` |
| 尤达反转 | `5 === x` → `x === 5` |
| 无穷大 | `1/0` → `Infinity` |
| 反转布尔逻辑 | `!(a == b)` → `a != b` |
| 一元表达式 | 移除无意义 `void`/`!`/`typeof` |
| 移除双重否定 | `!!true` → `true` |

### ES Module 支持

goja 的解析器不支持 ES module 语法 (`import`/`export`)。jd 会预先提取 import/export 语句，解析剩余脚本，执行变换后，将语句重新添加到输出开头。动态 `import()` 表达式能正确区分于 import 声明。

### 正则安全格式化

对于 goja 无法完整解析的文件 (如 Monaco 编辑器语言定义中大量正则)，jd 回退到源码级格式化：tokenizer 按分号拆分，同时保留正则字面量、字符串和注释原文。

## 安装

### macOS (Homebrew)

```bash
brew install ejfkdev/tap/jd
```

### 预编译二进制

从 [GitHub Releases](https://github.com/ejfkdev/jd/releases) 下载 (Linux/macOS/Windows, x86_64/ARM64)。

### 源码编译

```bash
go build -o jd .
```

## 用法

```bash
# 反混淆单个文件 (输出到 stdout)
jd obfuscated.js

# 写入到文件
jd obfuscated.js -o cleaned.js

# 从 stdin 读取
cat obfuscated.js | jd -

# 处理目录 — 递归处理所有 .js/.mjs/.cjs 文件，
# 将目录结构镜像到输出目录。
jd src/ -o dist/

# 目录模式默认输出: 在输入目录同级创建 <输入目录>-deobfuscated
jd src/

# 指定文件扩展名
jd src/ -o dist/ --ext .js,.mjs

# 并行处理 (N 个 worker，默认 CPU 核数)
jd src/ -o dist/ -j 8

# 跳过非代码文件 (只输出处理后的 JS)
jd src/ -o dist/ --copy-noncode=false

# 只反混淆，不做可读性还原
jd --unminify=false obfuscated.js

# JSON 输出 (含警告)
jd --json obfuscated.js
```

## 参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-o, --output` | stdout | 输出文件或目录 (目录默认: `<输入目录>-deobfuscated`) |
| `--deobfuscate` | true | 运行 obfuscator.io 反混淆 |
| `--unminify` | true | 运行可读性还原变换 |
| `--sandbox` | auto | 解码器执行方式: `auto` (静态优先再沙箱), `only` (始终沙箱), `off` (仅静态) |
| `--timeout` | 10s | 单文件沙箱超时时间 |
| `--ext` | .js,.mjs,.cjs | 目录模式下处理的文件扩展名 |
| `-j, --workers` | 0 (=CPU) | 目录模式并行 worker 数 |
| `--copy-noncode` | true | 复制非代码文件到输出目录 |
| `-v, --verbose` | false | 输出诊断信息到 stderr |
| `--json` | false | 输出 `{code, warnings}` JSON |

## 架构

```
jd/
├── main.go                     # CLI 入口
├── internal/
│   ├── cli/                    # cobra CLI (中英文 i18n)
│   ├── deobfuscator/           # 顶层 pipeline + ES module 预处理
│   ├── jsast/                  # AST 遍历器 (Cursor, Replace, Remove, Clone)
│   ├── codegen/                # AST → JavaScript 打印器 (pretty/compact)
│   ├── scope/                  # 作用域 & 绑定分析
│   ├── sandbox/                # goja VM 沙箱 (解码器执行)
│   ├── decoder/                # 静态解码: Base64/RC4/旋转模拟
│   ├── deobfuscate/            # 字符串数组/旋转器/解码器检测 + 变换
│   ├── unminify/               # 20 个可读性变换 + fixpoint runner
│   └── transform/              # Transform 抽象 + ApplyFixpoint
└── testdata/samples/           # 测试样本
```

### 流水线

```
解析 → 模块语句分离 → 反混淆 → 可读性还原 → 生成
```

1. **解析** — goja 解析器，启用 `IgnoreRegExpErrors`；无法解析时回退到源码级格式化
2. **反混淆** — 检测 obfuscator.io 字符串数组/旋转器/解码器，解码所有调用点 (静态优先，沙箱兜底)，移除辅助函数
3. **可读性还原** — fixpoint 循环 (≤20 轮) 合并可读性变换
4. **生成** — pretty 模式 codegen，运算符优先级和正确括号

### 混合解码

解码器使用两阶段方案：

1. **静态路径** (synchrony 方式): 通过 push/shift 模拟数组旋转，在纯 Go 中解码 SIMPLE/Base64/RC4 — 快速且确定性。
2. **沙箱兜底** (webcrack 方式): 提取字符串数组、旋转器和解码器代码，在 goja VM 中执行，配有 `Interrupt` 超时、`SetMaxCallStackSize`、确定性随机/时间源，且无 setInterval/setTimeout — 处理静态路径无法解码的自定义编码器。

## 依赖

- [goja](https://github.com/dop251/goja) — 纯 Go JavaScript 解析器 & 运行时 (无 CGO)
- [cobra](https://github.com/spf13/cobra) — CLI 框架

## 许可证

MIT

// Command xlang 是 xlang 语言的解释器。
//
//	xlang [flags] program.xl     # 编译并执行（旁边有可用缓存时直接用缓存）
//	xlang [flags] program.xlc    # 直接执行缓存文件
//
// 缓存规则（对照 CPython 的 .pyc）：
//   - 源文件旁的 <name>.xlc 只有在"源文件内容没变 + 格式版本一致 + 优化开关一致"
//     时才会被采用；
//   - 否则重新编译（默认不写缓存，加 -write-cache 才写）；
//   - 缓存损坏 / 被截断 / 版本不符一律当成"没有缓存"，绝不把坏字节码交给 VM。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"Interpreter/bytecode"
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	"Interpreter/vm"
)

const usage = `xlang —— xlang 语言的解释器

用法:
  xlang [flags] program.xl     编译并执行（旁边有可用的 .xlc 就直接用）
  xlang [flags] program.xlc    直接执行缓存文件

flags:
  -write-cache   编译成功后把 .xlc 写到源文件旁边
  -no-cache      既不读也不写 .xlc（总是从源码编译）
  -opt           是否启用 IR/SSA 优化（默认 true）
  -v             打印缓存命中/失效的原因
  -q             不打印最后一条语句的值
`

// 退出码约定：0 成功，1 运行/编译错误，2 用法错误。
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("xlang", flag.ExitOnError)
	writeCache := fs.Bool("write-cache", false, "编译成功后写出 .xlc 缓存")
	noCache := fs.Bool("no-cache", false, "不读也不写 .xlc 缓存")
	optimize := fs.Bool("opt", true, "启用 IR/SSA 优化管线")
	verbose := fs.Bool("v", false, "打印缓存命中/失效原因")
	quiet := fs.Bool("q", false, "不打印最后一条语句的值")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitUsage
	}
	opts := options{writeCache: *writeCache, noCache: *noCache, optimize: *optimize, verbose: *verbose}

	bc, err := load(&opts, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}

	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}
	if !*quiet {
		fmt.Println(m.LastPop().Inspect())
	}
	return exitOK
}

type options struct {
	writeCache bool
	noCache    bool
	optimize   bool
	verbose    bool
}

func (o *options) logf(format string, args ...interface{}) {
	if o.verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// load 取得可执行的字节码：优先用缓存，失败则从源码编译。
func load(o *options, path string) (*bytecode.Bytecode, error) {
	if strings.EqualFold(filepath.Ext(path), ".xlc") {
		return loadCacheFile(o, path)
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cachePath := cachePathFor(path)
	hash := bytecode.SourceHash(src)

	if !o.noCache {
		if bc, ok := tryCache(o, cachePath, path, hash); ok {
			return bc, nil
		}
	}

	bc, err := compile(src, path, o.optimize)
	if err != nil {
		return nil, err
	}
	if o.writeCache && !o.noCache {
		meta := bytecode.CacheMeta{Optimized: o.optimize, SourceHash: hash}
		if info, statErr := os.Stat(path); statErr == nil {
			meta.SourceSize = info.Size()
			meta.SourceMTime = info.ModTime().UnixNano()
		}
		if err := writeCacheFile(cachePath, bc, meta); err != nil {
			// 写缓存失败不影响运行（例如目录只读）
			o.logf("xlc: 写缓存失败（忽略）: %v", err)
		} else {
			o.logf("xlc: 已写出 %s", cachePath)
		}
	}
	return bc, nil
}

// tryCache 检查缓存是否仍然可用；任何异常都返回 ok=false，由调用方重新编译。
func tryCache(o *options, cachePath, srcPath string, hash [32]byte) (*bytecode.Bytecode, bool) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			o.logf("xlc: 读不到缓存 %s: %v", cachePath, err)
		}
		return nil, false
	}
	bc, meta, err := bytecode.Decode(data)
	if err != nil {
		o.logf("xlc: 忽略缓存 %s: %v", cachePath, err)
		return nil, false
	}
	if meta.Optimized != o.optimize {
		o.logf("xlc: 忽略缓存 %s: 优化开关不一致（缓存 optimized=%v）", cachePath, meta.Optimized)
		return nil, false
	}
	// size/mtime 只作快速路径；不一致时再比内容哈希（touch 过但内容没变仍可命中）。
	info, statErr := os.Stat(srcPath)
	fresh := statErr == nil && meta.SourceSize == info.Size() && meta.SourceMTime == info.ModTime().UnixNano()
	if !fresh && meta.SourceHash != hash {
		o.logf("xlc: 忽略缓存 %s: 源文件已变化", cachePath)
		return nil, false
	}
	o.logf("xlc: 命中缓存 %s", cachePath)
	return bc, true
}

// loadCacheFile 直接执行一个 .xlc；旁边有同名源码时先校验它是否仍然匹配。
func loadCacheFile(o *options, path string) (*bytecode.Bytecode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	bc, meta, err := bytecode.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	srcPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".xl"
	if src, err := os.ReadFile(srcPath); err == nil {
		if bytecode.SourceHash(src) != meta.SourceHash {
			return nil, fmt.Errorf("%s: 与 %s 不一致，请重新编译：xlang -write-cache %s",
				path, filepath.Base(srcPath), srcPath)
		}
	}
	if meta.Optimized != o.optimize {
		o.logf("xlc: 注意：缓存是按 optimized=%v 生成的", meta.Optimized)
	}
	return bc, nil
}

func compile(src []byte, name string, optimize bool) (*bytecode.Bytecode, error) {
	p := parser.NewParser(lexer.NewLexer(string(src)))
	node := p.Parse()
	if p.HasError() {
		return nil, fmt.Errorf("%s: %v", name, errsText(p.Errs()))
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.SetOptimize(optimize)
	c.Compile(node)
	if c.HasError() {
		return nil, fmt.Errorf("%s: %v", name, errsText(c.Errs()))
	}
	return c.ByteCode(), nil
}

func errsText(errs []error) string {
	if len(errs) == 1 {
		return errs[0].Error()
	}
	var sb strings.Builder
	for i, e := range errs {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(e.Error())
	}
	return sb.String()
}

func cachePathFor(srcPath string) string {
	return strings.TrimSuffix(srcPath, filepath.Ext(srcPath)) + ".xlc"
}

// writeCacheFile 原子写入：先写临时文件再改名，避免中途失败留下半个缓存。
func writeCacheFile(path string, bc *bytecode.Bytecode, meta bytecode.CacheMeta) error {
	data, err := bytecode.Encode(bc, meta)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".xlc-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 改名成功后这里就是空操作
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

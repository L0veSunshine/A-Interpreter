# xlang

一步一步实现一个解释器：词法分析 → Pratt 解析 → AST → 字节码 → 栈式虚拟机。

## 目录结构

| 包 | 职责 |
| --- | --- |
| `lexer` | 词法分析。`Char` 是值类型，`unicode` 分类，最长匹配运算符表，支持 `\xNN`/`\uNNNN` 转义与 `0x`/`0o`/`0b`/`_` 数字字面量 |
| `parser` | Pratt 解析器。统一的游标契约（`cur` 恒为下一个待消费 token）、`token(k)` 前瞻、`endStatement()` 收束语句、`synchronize()` 错误恢复，报错带 line/col |
| `ast` | 语法树。每个节点都能给出源码行号（`Line()`），供运行时错误定位 |
| `parser/symtable.go` | 符号表。函数级共享的局部槽位计数、块作用域、`FindScope` |
| `compiler` | AST → 栈式字节码。块作用域、循环上下文、复合赋值、行号表 |
| `bytecode` / `code` | 字节码容器、操作码定义与反汇编 |
| `vm` | 栈式虚拟机。栈与帧按需增长，运行时错误统一带函数名与行号 |
| `cfg` | 基本块划分、可达性、`Compact`（删死块/冗余跳转）、支配树与支配边界 |
| `ir` | 三地址码 IR、SSA 构造、优化、降级回字节码 |
| `cmd/xlang` | 极简命令行：`go run ./cmd/xlang 文件` 直接执行一个源码文件 |

## 编译管线

```
源码 ──lexer──► tokens ──parser──► AST ──compiler──► 栈字节码 ──ir──► 优化后的字节码 ──vm──► 结果
```

`ir` 是可选的一层，默认开启。它把"栈位置"变成"有名字的值"：

```
栈字节码 ──stack-to-register promotion──► 三地址码
        ──CFG + 支配树──► SSA（Cytron 算法插 phi）
        ──常量折叠 / 分支化简 / CSE / DCE / 不可达块删除──►
        ──寄存器分配（活跃分析 + 冲突图着色）──► 降级回栈字节码
```

**契约**：优化器要么返回一份**行为等价**的字节码，要么原样放弃（返回错误）。
因此调用方失败时直接忽略结果即可，正确性不依赖优化是否成功。

当前会放弃优化的情况：

- 方法调用、切片字面量、REPL 的隐式打印 —— IR 尚未建模这些指令；
- 基本块边界上操作数栈非空 —— 需要"栈上的 phi"，收益低且容易出错；
- 任何自检失败（例如读到未定义的槽位）。

单个函数放弃优化不影响其它函数：优化是函数内的局部变换。

关闭优化：

```go
c := compiler.NewCompiler()
c.SetOptimize(false)
```

实测（`ir` 包的等价性语料，25 个程序合计）：**819 → 551 字节（-32%）**。

已知的取舍：赋值表达式（`g = f()` 这种把赋值当值用的写法）在原始字节码里
直接把值留在操作数栈上，而 IR 必须"存槽位 + 读回"，每个这样的点会多 3 字节；
循环变量的 phi 拷贝同理（等实现 phi 拷贝合并后消失）。

## 性能

`vm` 包有基准测试（`go test -run XXX -bench . ./vm/`）。解释器循环里有几处结构性开销，
已经按"先测再改"的顺序处理：

| 项 | 做法 | 依据 |
| --- | --- | --- |
| 当前帧 | 把 `frame`/`ins`/`ip` 提到局部变量，只在 push/pop 帧后重新装载 | `currentFrame()` 原本每条指令要调用 3~6 次，每次都做"取下标+边界检查+乘 sizeof(Frame)"；CPython 3.11 的"帧留在局部变量里"是同一思路 |
| 帧的局部槽位 | 所有帧共享一块按栈方式分配/回收的槽位区（`vm.locals`），返回时只回退 `localsTop` | 原来每次调用都要 `make([]object.Object, n)`；CPython 3.11 的连续 frame chunk 同理 |
| 小整数/布尔 | `-256..2048` 的整数与 `true`/`false` 常驻单例 | 每次算术结果装箱都是一次堆分配 |
| 算术分派 | 用类型断言代替 `Type()` 字符串比较 | `Type()` 返回的是字符串，一次算术要比较好几轮字符串 |
| 数组字面量 | `make` + `copy` 取代 append 循环 | 避免多次扩容 |
| 调用路径 | 帧在帧栈里原地构造（`pushFrame` 返回指针），`CompiledFunc` 传指针 | `CompiledFunc` 约 104 字节、`Frame` 约 120 字节，原来每次调用要拷 6 次 |
| 栈操作 | 编译期算出每函数的操作数栈上界，建帧时一次性预留；循环里不再检查容量 | 见下节 |
| **超指令融合** | 把 `GetLocal/GetGlobal/Constant + … + 二元运算` 等固定搭配合并成一条指令 | 减少派发次数（详见下节） |

### 操作数栈：把容量检查挪到编译期

最直白的写法是"每执行一条指令就检查一次剩余栈空间"。那是一次多余的内存读 +
比较 + 分支，实测占解释器开销 5% 左右。三家工业实现都不这么做：

- **CPython**：编译期用 `stackdepth()` 做一遍数据流，把结果写进 code object 的
  `co_stacksize`；帧一次性按 `frame_size = locals + stacksize` 分配，
  求值循环里**没有任何容量检查**。
- **JVM**：`Code` 属性里直接带 `max_stack`，由 javac/校验阶段算出。
- **Lua**：不静态计算，但规定栈上永远至少留 `LUA_MINSTACK`（20）个空位，
  只在"可能一次要很多槽"的指令和函数调用边界调 `luaD_checkstack`。

本项目采用 CPython 的方案，并吸收 Lua 的余量思想：

1. `code.MaxStack`（[code/stack.go](code/stack.go)）对指令流做数据流分析，
   算出操作数栈的精确上界。每条指令的栈效应表必须与 VM 实现严格对应；
   遇到无法静态确定的形态（未知操作码、控制流汇合处深度不一致、栈下溢）
   直接返回错误。
2. 结果存进 `CompiledFunc.MaxStack` / `Bytecode.MainMaxStack`
   （超指令融合会改变指令流，所以融合之后要重算）。
3. VM 在建帧时 `reserveStack(basePoint + 参数个数 + MaxStack + 8)`，
   之后循环里不再逐条检查。那 8 个空位是防御性余量，对应 Lua 的 `LUA_MINSTACK`。
4. 算不出来时用 `code.ConservativeMaxStack` 兜底：每条指令净压栈最多 1 个，
   所以"指令总数"永远是不小于真实峰值的上界——宁可多预留，也不会不够。
5. **安全网**：万一上界算错，Go 的边界检查会把越界变成 panic 而不是内存破坏；
   `Run` 的 defer 里 `recover` 把它转成 `internal: operand stack overflow` 错误，
   不会让进程崩掉。

这套做法的正确性由三类测试守着：

- `code/stack_test.go`：直线代码、分支、循环回边、汇合深度冲突、栈下溢、
  非法跳转目标、未知操作码、融合指令的栈效应、保守兜底；
- `vm/maxstack_test.go`：静态上界与真实峰值逐程序比对（无调用的程序严格相等）、
  深表达式、循环多轮不增长，以及"故意把上界报小"时必须返回错误而不是崩溃；
- `ir/fuse_test.go::TestStaticStackBoundCoversCorpus`：全部语料跑一遍，
  不允许出现任何 `internal: operand stack overflow`。

### 超指令（superinstruction）

解释器每条指令都要付一次派发成本（读 opcode、跳转表间接跳转、读操作数、
调整栈指针）。`a < 10` 在栈式字节码里是三条指令、三次派发，而它只做一次比较。

`code.Fuse` 在 IR 降级之后做一遍模式融合（[code/fuse.go](code/fuse.go)），
把下面三类固定搭配合并成一条：

| 原序列 | 融合后 | 长度 | 派发 |
| --- | --- | --- | --- |
| `OpGetLocal/OpGetGlobal/OpConstant` + 同左 + 算术/比较 | `OpBinFused` | 7 → 7 字节 | 3 → **1** |
| `OpGetLocal/OpGetGlobal/OpConstant` + `OpReturnVal` | `OpReturnOperand` | 4 → 4 字节 | 2 → **1** |
| 算术/比较 + `OpReturnVal` | `OpBinReturn` | 2 → 2 字节 | 2 → **1** |

`OpBinFused` 用 `[kindAB][binOp][idxA][idxB]` 编码，`kindA`/`kindB` 表示操作数
来自局部变量 / 常量 / 全局变量，所以 3×3 种组合只需要一条指令，
不会让跳转表的目标数膨胀（那会伤害分支预测）。

安全约束（缺一不可，全部有 UT 覆盖）：

1. 被合并的序列里，**除第一条外不能是跳转目标** —— 否则跳到中间会错位；
2. 被合并的指令必须来自**同一源码行** —— 否则运行时错误报的行号会变；
3. 所有跳转目标都要**重新映射**到新偏移，行号表同理；
4. 只减少派发，不改变求值顺序、错误信息与行号：融合指令内部调用的是
   与 `OpBin` 完全相同的那份语义实现。

任何可疑情况（未知操作码、跳转目标越界、无法映射）都放弃该处融合；
`Fuse` 出错时原样返回入参，调用方保留未融合的指令流即可，正确性不受影响。
`ir.FusionEnabled = false` 可以关掉整个融合（基准对比与排障用）。

实测（fib(30)，同一台机器）：

```
融合开   145 ms
融合关   180 ms      → 融合本身 -19%
```

fib 函数从 12 条指令降到 7 条（3 个 `OpBinFused` + 1 个 `OpReturnOperand` + 1 个 `OpBinReturn`）。

（顺带修了一个旧问题：`cfg.Compact` 改变指令偏移后没有重映射行号表，
会导致优化后的程序报错行号漂移。现在由 `CompactLines` 一并处理，并有 UT 防护。）

实测（1000 次迭代的整数循环 / fib(18) / 数组下标 / 深递归 300 层）：

```
                    第一轮后        第二轮后        融合后
LoopArith           123771 ns/op    99680 ns/op    73700 ns/op
FibRecursive        911324 ns/op   539000 ns/op   469000 ns/op
ArrayIndex           25980 ns/op    27960 ns/op    15900 ns/op
DeepRecursion        30137 ns/op    26740 ns/op    20600 ns/op

allocs/op: 3589→1912 / 8377→5 / 151→6 / 207→7
```

### fib(30) 横向对比

同一份朴素递归 `fib(n)=fib(n-1)+fib(n-2)`，同一台机器（Ryzen 7 5800X / Windows）：

| 实现 | 端到端（含启动，5 次最好） | 进程内纯计算 | 每次调用 |
| --- | --- | --- | --- |
| xlang（本项目，字节码解释器） | 0.194 s | ~145 ms | ~54 ns |
| CPython 3.14.2 | 0.122 s | ~95 ms | ~35 ns |
| Node 24.21（V8 JIT） | 0.084 s | ~6.7 ms | ~2.5 ns |
| Rust 1.99 `-O`（原生） | 0.009 s | ~1.6 ms | ~0.6 ns |

注：fib(30) 约有 269 万次调用，两种解释器的耗时几乎全在"建帧 + 派发 + 返回"上。
CPython 3.11+ 有专用化指令（`BINARY_OP_ADD_INT`、`LOAD_FAST_BORROW`）和 opcache，
这就是它比本项目快约 1.8 倍的原因；Node/Rust 快的这 20~100 倍来自 JIT/原生编译，不是同一个量级。
启动开销：xlang 21 ms、CPython 27 ms、Node 78 ms、Rust 8 ms。

## 还没做的优化

按收益排序：**tagged value / NaN-boxing**（彻底去掉 `object.Object` 接口装箱，
这是继续逼近 CPython 的关键一步；实测浮点每次运算都会堆分配）。
直接线索化派发在 Go 里不可行（没有 computed goto，而密集 switch 本来就会编译成跳转表），
函数指针表实测比现在的 switch 慢 23%，不要做。

## 语义要点

- `OpDiv` 对整数也返回浮点；`OpMod` 对整数返回整数；`OpPow` 总返回浮点。
- `OpUpdate` 原地修改容器并把容器压回栈；`OpUpdateLocal`/`OpUpdateGlobal` 写槽位但不弹栈。
- `OpPop` 会记录 `lastPop`（REPL 靠它显示结果），因此 IR 里它是 `OpSink`，不是死代码。
- 运行时报错（除零、类型不匹配、下标越界）是程序可观察的行为，优化器不会删除可能报错的指令。

## 测试

```
go test ./...           # 全部
go test -race ./...     # 竞态
```

`ir` 包有两类防护：

- `TestOptimizePreservesBehavior`：每个语料程序在优化前后的**运行结果和错误信息**必须完全一致；
- `TestOptimizeNeverGrowsCode`：单个用例最多允许 3 字节增长，且语料总量必须下降。

`ir_internal_test.go` 覆盖内部结构：栈提升、放弃条件的触发、phi 插入位置、
同时活跃值的槽位互斥、DCE 保留可能报错的指令、常量折叠拒绝折叠会报错的运算。

超指令融合有两层防护：

- `code/fuse_test.go`：模式匹配、跳转目标重映射、拒绝跨行/跨跳转目标融合、
  幂等性、代码量不增长、未知操作码原样返回；
- `ir/fuse_test.go`：融合开/关的**逐字等价**（含错误信息与行号）、
  融合确实触发、指令条数下降、融合后字节码仍能通过 CFG 校验、不泄漏操作数栈。

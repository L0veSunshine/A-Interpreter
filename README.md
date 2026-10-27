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

实测（1000 次迭代的整数循环 / fib(18) / 数组下标 / 深递归 300 层）：

```
                    优化前          优化后
LoopArith           139558 ns/op   100917 ns/op   (-28%)
FibRecursive       1099901 ns/op   752382 ns/op   (-32%)
ArrayIndex           31590 ns/op    22202 ns/op   (-30%)
DeepRecursion        35154 ns/op    28380 ns/op   (-19%)

allocs/op: 3589→1912 / 8377→5 / 151→6 / 207→7
```

还没做、但值得做的（按收益排序）：**超指令/快速化**（把 `GetLocal+GetLocal+Add` 合成一条，
对应 CPython 3.12 的专用指令，需要新增操作码并贯穿 code/compiler/ir/反汇编）、
**tagged value / NaN-boxing**（彻底去掉 `object.Object` 接口装箱，改为 Go 里的联合表示，
收益最大但改动面也最大）、直接线索化派发（Go 没有 computed goto，
只能用函数表，实际未必比现在这个密集 switch 快，需要实测）。

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

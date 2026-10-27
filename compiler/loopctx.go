package compiler

// loopCtx 描述一层正在编译的循环：目前只需要记录待回填的 break 跳转位置。
// 每层循环一个实例，嵌套时按栈管理，避免内层循环把外层的 break 补错。
type loopCtx struct {
	breaks []int
}

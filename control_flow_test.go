package main

import "testing"

// 脚本最后一条语句是 if 时，两个分支都以"落到脚本末尾"结束（IR 里是 OpExit）。
// 降级阶段曾经在 then 分支末尾省掉跳转，于是 then 执行完会继续跑布局上紧随
// 其后的 else 代码，最终结果被 else 覆盖。
func TestIfAsFinalStatementKeepsBranchesApart(t *testing.T) {
	cases := []struct{ src, want string }{
		{"def f(){\n\treturn 1\n}\nif(f()){1}else{0}", "1"},
		{"def f(){\n\treturn 0\n}\nif(f()){1}else{0}", "0"},
		{"var x=len(\"a\")\nif(x){1}else{0}", "1"},
		{"if(len(\"a\")==1){1}else{0}", "1"},
		{"var x=[]\nif(x){1}else{0}", "1"},
		// 没有 else 时 then 块直接落到末尾
		{"def f(){\n\treturn true\n}\nif(f()){7}", "7"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

// then 分支执行后，else 的副作用不能发生。
func TestIfElseSideEffectsAtScriptEnd(t *testing.T) {
	src := `var hits=0
def f(){
	hits+=1
	return true
}
var r=0
if(f()){
	r=1
}else{
	r=2
}
[r, hits]`
	expect(t, src, "[1, 1]")

	src = `var hits=0
def f(){
	hits+=1
	return false
}
var r=0
if(f()){
	r=1
}else{
	r=2
}
[r, hits]`
	expect(t, src, "[2, 1]")
}

// 循环体内的调用作为条件（未优化/优化两条路径都要一致）。
func TestCallAsLoopCondition(t *testing.T) {
	src := `var n=0
def more(){
	n+=1
	return n<4
}
var count=0
for(more()){
	count+=1
}
[count, n]`
	expect(t, src, "[3, 4]")
}

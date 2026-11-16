package vm_test

import "testing"

// 数组是引用语义：赋值与传参共享同一个对象，原地修改对所有持有者可见。
// 切片仍然产生新数组，`==` 仍然按内容比较。两种融合模式下结果必须一致。
func TestArrayReferenceSemantics(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "assignment aliases the same array",
			src:  "var a=[1]\nvar b=a\nb.append(2)\na",
			want: "[1, 2]",
		},
		{
			name: "argument mutation is visible to caller",
			src:  "def push(x){\n\tx.append(9)\n\treturn 0\n}\nvar a=[1]\npush(a)\na",
			want: "[1, 9]",
		},
		{
			name: "index assignment aliases",
			src:  "var a=[1,2]\nvar b=a\nb[0]=5\na",
			want: "[5, 2]",
		},
		{
			name: "element alias shares nested array",
			src:  "var a=[[1]]\nvar b=a[0]\nb.append(2)\na",
			want: "[[1, 2]]",
		},
		{
			name: "slice creates an independent array",
			src:  "var a=[1,2,3]\nvar b=a[0:2]\nb.append(7)\na",
			want: "[1, 2, 3]",
		},
		{
			name: "slice result holds the appended element",
			src:  "var a=[1,2,3]\nvar b=a[0:2]\nb.append(7)\nb",
			want: "[1, 2, 7]",
		},
		{
			name: "structural equality is unchanged",
			src:  "var a=[1,2]\nvar b=[1,2]\na==b",
			want: "true",
		},
		{
			name: "equality after aliasing mutation",
			src:  "var a=[1]\nvar b=a\nb.append(2)\na==b",
			want: "true",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, fusion := range []bool{true, false} {
				bc := compileFusion(t, tc.src, fusion)
				got, errText := runResult(bc)
				if errText != "" {
					t.Fatalf("fusion=%v: unexpected error: %s", fusion, errText)
				}
				if got != tc.want {
					t.Fatalf("fusion=%v: got %q, want %q", fusion, got, tc.want)
				}
			}
		})
	}
}

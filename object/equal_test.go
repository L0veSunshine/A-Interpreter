package object

import "testing"

// Equal 必须与 RichCompare 的判定一致（同类型且 Inspect 相同）。
func TestEqualMatchesRichCompare(t *testing.T) {
	samples := []Object{
		Int{Value: 0}, Int{Value: 1}, Int{Value: -1}, Int{Value: 9007199254740993},
		Float{Value: 1.5}, Float{Value: 9007199254740992}, Float{Value: 1e13},
		Boolean{Value: true}, Boolean{Value: false},
		String{Value: []rune("")}, String{Value: []rune("a")}, String{Value: []rune("中文")},
		Null{},
		&Array{}, &Array{Elements: []Object{Int{Value: 1}}},
		&Array{Elements: []Object{Int{Value: 1}, String{Value: []rune("a")}}},
		&Array{Elements: []Object{Int{Value: 1}, &Array{Elements: []Object{Float{Value: 2}}}}},
		Map{Store: map[int]MapPair{}},
		Map{Store: map[int]MapPair{7: {Key: Int{Value: 1}, Item: Int{Value: 2}}}},
		Map{Store: map[int]MapPair{7: {Key: Int{Value: 1}, Item: Int{Value: 3}}}},
	}
	for i, v := range samples {
		for j, w := range samples {
			want := RichCompare(v, w)
			if got := ValueEqual(v, w); got != want {
				t.Errorf("ValueEqual(samples[%d], samples[%d]) = %v, RichCompare = %v", i, j, got, want)
			}
		}
	}
}

package code

import "testing"

func TestLineAt(t *testing.T) {
	entries := []LineEntry{
		{Offset: 0, Line: 1},
		{Offset: 10, Line: 5},
		{Offset: 20, Line: 9},
	}
	cases := map[int]int{
		-1: 0,
		0:  1,
		5:  1,
		9:  1,
		10: 5,
		15: 5,
		20: 9,
		99: 9,
	}
	for ip, want := range cases {
		if got := LineAt(entries, ip); got != want {
			t.Fatalf("ip=%d: want line %d, got %d", ip, want, got)
		}
	}
}

func TestLineAtEmpty(t *testing.T) {
	if got := LineAt(nil, 0); got != 0 {
		t.Fatalf("empty mapping should return 0, got %d", got)
	}
}

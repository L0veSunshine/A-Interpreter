package lexer

import (
	"fmt"
	"testing"
)

func TestCode(t *testing.T) {
	fmt.Println(Code(96).IsDigital())
	fmt.Println(Code(97).Equal(97))
}

func TestChar_IsAlNum(t *testing.T) {
	type fields struct {
		id int32
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name:   "1",
			fields: fields{id: 67},
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Char{
				id: tt.fields.id,
			}
			if got := c.IsAlNum(); got != tt.want {
				t.Errorf("IsAlNum() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestChar_IsWhitespace(t *testing.T) {
	c := Code(10)
	fmt.Println(c.IsWhitespace(), c.Quote())
}

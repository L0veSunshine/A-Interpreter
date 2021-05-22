package code

import (
	"fmt"
	"testing"
)

func TestReadOperand(t *testing.T) {
	ins := Make(OpConstant, 5)
	fmt.Println(ReadOperand(definitions[OpConstant], ins))
	fmt.Println(Make(OpConstant, 10))
}

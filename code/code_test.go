package code

import (
	"fmt"
	"testing"
)

func TestReadOperand(t *testing.T) {
	ins := Make(OpConstant, 5)
	fmt.Println(ReadOperand(Definitions[OpConstant], ins))
	fmt.Println(Make(OpConstant, 10))
}

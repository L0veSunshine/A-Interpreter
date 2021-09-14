package compiler

type PosType string

const (
	LoopStart  PosType = "Loop"
	BreakPoint PosType = "Break"
)

type InsPosInfo struct {
	Pos   int
	PType PosType
}

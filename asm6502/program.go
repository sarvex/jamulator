package asm6502

import (
	"strings"
	"errors"
	"fmt"
)

// Program is a proper program, one that you can compile
// into native code. A ProgramAST can be compiled into a
// Program and 6502 machine code can be read directly into
// a Program
type Program struct {
	Variables map[string] int
	Labels map[string] int
	Instructions []*Instruction
	Errors []error
}

type Instruction struct {
	StatementAst InstructionStatement
	OpCode int
	OpSize int
}

var impliedOpcode = map[string] int {
	"brk": 0x00,
	"clc": 0x18,
	"cld": 0xd8,
	"cli": 0x58,
	"clv": 0xb8,
	"dex": 0xca,
	"dey": 0x88,
	"inx": 0xe8,
	"iny": 0xc8,
	"nop": 0xea,
	"pha": 0x48,
	"php": 0x08,
	"pla": 0x68,
	"plp": 0x28,
	"rti": 0x40,
	"rts": 0x60,
	"sec": 0x38,
	"sed": 0xf8,
	"sei": 0x78,
	"tax": 0xaa,
	"tay": 0xa8,
	"tsx": 0xba,
	"txa": 0x8a,
	"txs": 0x9a,
	"tya": 0x98,
}

var immediateOpcode = map[string] int {
	"adc": 0x69,
	"and": 0x29,
	"cmp": 0xc9,
	"cpx": 0xe0,
	"cpy": 0xc0,
	"eor": 0x49,
	"lda": 0xa9,
	"ldx": 0xa2,
	"ldy": 0xa0,
	"ora": 0x09,
	"sbc": 0xe9,
}

var absIndexedXOpcode = map[string] int {
	"adc": 0x7d,
	"and": 0x3d,
	"asl": 0x1e,
	"cmp": 0xdd,
	"dec": 0xde,
	"eor": 0x5d,
	"inc": 0xfe,
	"lda": 0xbd,
	"ldy": 0xbc,
	"lsr": 0x5e,
	"ora": 0x1d,
	"rol": 0x3e,
	"ror": 0x7e,
	"sbc": 0xfd,
	"sta": 0x9d,
}

var absIndexedYOpcode = map[string] int {
	"adc": 0x79,
	"and": 0x39,
	"cmp": 0xd9,
	"eor": 0x59,
	"lda": 0xb9,
	"ldx": 0xbe,
	"ora": 0x19,
	"sbc": 0xf9,
	"sta": 0x99,
}

type opcodeDef struct {
	opcode int
	size int
}

func compileInstruction(s InstructionStatement) (*Instruction, error) {
	opName := s.OpName()
	lowerOpName := strings.ToLower(opName)
	switch ss := s.(type) {
	case ImpliedInstruction:
		opcode, ok := impliedOpcode[lowerOpName]
		if !ok {
			return nil, errors.New(fmt.Sprintf("Line %d: Unrecognized implied instruction: %s", ss.Line, opName))
		}
		return &Instruction{s, opcode, 1}, nil
	case ImmediateInstruction:
		opcode, ok := immediateOpcode[lowerOpName]
		if !ok {
			return nil, errors.New(fmt.Sprintf("Line %d: Unrecognized immediate instruction: %s", ss.Line, opName))
		}
		return &Instruction{s, opcode, 2}, nil
	case AbsoluteWithLabelIndexedInstruction:
		lowerRegName := strings.ToLower(ss.RegisterName)
		if lowerRegName == "x" {
			opcode, ok := absIndexedXOpcode[lowerOpName]
			if !ok {
				return nil, errors.New(fmt.Sprintf("Line %d: Unrecognized absolute, X instruction: %s", ss.Line, opName))
			}
			return &Instruction{s, opcode, 3}, nil
		} else if lowerRegName == "y" {
			opcode, ok := absIndexedYOpcode[lowerOpName]
			if !ok {
				return nil, errors.New(fmt.Sprintf("Line %d: Unrecognized absolute, Y instruction: %s", ss.Line, opName))
			}
			return &Instruction{s, opcode, 3}, nil
		} else {
			return nil, errors.New(fmt.Sprintf("Line %d: Register argument must be X or Y", ss.Line))
		}
	}
	panic("Unrecognized instruction type")
}

// collect all variable assignments into a map
func (p *Program) Visit(n Node) {
	switch ss := n.(type) {
	case AssignStatement:
		p.Variables[ss.VarName] = ss.Value
	case InstructionStatement:
		i, err := compileInstruction(ss)
		if err != nil {
			p.Errors = append(p.Errors, err)
		} else if i != nil {
			p.Instructions = append(p.Instructions, i)
		}
	}
}

func (p *Program) VisitEnd(n Node) {}

func NewProgram() *Program {
	p := Program{
		map[string]int {},
		map[string]int {},
		[]*Instruction{},
		[]error{},
	}
	return &p
}

func (ast *ProgramAST) ToProgram() (*Program, error) {
	p := NewProgram()
	ast.Ast(p)
	return p, nil
}
package asm6502

import (
	"github.com/axw/gollvm/llvm"
	"os"
	"fmt"
	"bytes"
)

type Compilation struct {
	Warnings []string
	Errors []string

	program *Program
	mod llvm.Module
	builder llvm.Builder
	mainFn llvm.Value
	labeledData map[string] llvm.Value
	currentValue *bytes.Buffer
	currentLabel string
	mode int
	// map label name to basic block
	basicBlocks map[string] llvm.BasicBlock
	currentBlock *llvm.BasicBlock
	// label names to look for
	nmiLabelName string
	resetLabelName string
	irqLabelName string
	nmiBlock *llvm.BasicBlock
	resetBlock *llvm.BasicBlock
	irqBlock *llvm.BasicBlock
}

type Compiler interface {
	Compile(*Compilation)
}

const (
	dataStmtMode = iota
	compileMode
)

func (c *Compilation) dataAddStmt(stmt *DataStatement) {
	if len(c.currentLabel) == 0 {
		// trash the data
		c.Warnings = append(c.Warnings, fmt.Sprintf("trashing data at 0x%04x", stmt.Offset))
		return
	}
	for _, item := range(stmt.dataList) {
		var err error
		switch v := item.(type) {
			case *IntegerDataItem:
				err = c.currentValue.WriteByte(byte(*v))
				if err != nil {
					c.Errors = append(c.Errors, err.Error())
					return
				}
			case *StringDataItem:
				_, err = c.currentValue.WriteString(string(*v))
				if err != nil {
					c.Errors = append(c.Errors, err.Error())
					return
				}
		}
	}
}

func (c *Compilation) dataStop() {
	if len(c.currentLabel) == 0 { return }
	if c.currentValue.Len() == 0 { return }
	text := llvm.ConstString(c.currentValue.String(), false)
	strGlobal := llvm.AddGlobal(c.mod, text.Type(), c.currentLabel)
	strGlobal.SetLinkage(llvm.PrivateLinkage)
	strGlobal.SetInitializer(text)
	c.labeledData[c.currentLabel] = strGlobal
	c.currentLabel = ""
}

func (c *Compilation) dataStart(stmt *LabeledStatement) {
	c.currentLabel = stmt.LabelName
	c.currentValue = new(bytes.Buffer)
}

func (c *Compilation) Visit(n Node) {
	switch c.mode {
	case dataStmtMode:
		c.visitForDataStmts(n)
	case compileMode:
		c.visitForCompile(n)
	}
}

func (c *Compilation) visitForCompile(n Node) {
	switch t := n.(type) {
	case Compiler:
		t.Compile(c)
	}
}

func (c *Compilation) visitForDataStmts(n Node) {
	switch t := n.(type) {
	case *DataStatement:
		c.dataAddStmt(t)
	case *LabeledStatement:
		c.dataStop()
		c.dataStart(t)
	default:
		c.dataStop()
	}
}

func (c *Compilation) VisitEnd(n Node) {}

func (i *ImmediateInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "ImmediateInstruction lacks Compile() implementation")
}

func (i *ImpliedInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "ImpliedInstruction lacks Compile() implementation")
}

func (i *DirectWithLabelIndexedInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "DirectWithLabelIndexedInstruction lacks Compile() implementation")
}

func (i *DirectIndexedInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "DirectIndexedInstruction lacks Compile() implementation")
}

func (i *DirectWithLabelInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "DirectWithLabelInstruction lacks Compile() implementation")
}

func (i *DirectInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "DirectInstruction lacks Compile() implementation")
}

func (i *IndirectXInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "IndirectXInstruction lacks Compile() implementation")
}

func (i *IndirectYInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "IndirectYInstruction lacks Compile() implementation")
}

func (i *IndirectInstruction) Compile(c *Compilation) {
	c.Errors = append(c.Errors, "IndirectInstruction lacks Compile() implementation")
}

func (s *LabeledStatement) Compile(c *Compilation) {
	// if we've already processed it as data, move on
	_, ok := c.labeledData[s.LabelName]
	if ok { return }

	bb := llvm.AddBasicBlock(c.mainFn, s.LabelName)
	if c.currentBlock != nil {
		c.builder.SetInsertPointAtEnd(*c.currentBlock)
		c.builder.CreateBr(bb)
	}
	c.currentBlock = &bb

	switch s.LabelName {
	case c.nmiLabelName:
		c.nmiBlock = &bb
		c.builder.SetInsertPointAtEnd(bb)
		c.builder.CreateUnreachable()
		c.currentBlock = nil
	case c.resetLabelName:
		c.resetBlock = &bb
	case c.irqLabelName:
		c.irqBlock = &bb
		c.builder.SetInsertPointAtEnd(bb)
		c.builder.CreateUnreachable()
		c.currentBlock = nil
	}
}

func (c *Compilation) setUpEntryPoint(p *Program, addr int, s *string) {
	n, ok := p.offsets[addr]
	if !ok {
		c.Errors = append(c.Errors, fmt.Sprintf("Missing 0x%04x entry point"))
		return
	}
	stmt, ok := n.(*DataWordStatement)
	if !ok {
		c.Errors = append(c.Errors, fmt.Sprintf("Entry point at 0x%04x must be a dc.w"))
		return
	}
	call, ok := stmt.dataList[0].(*LabelCall)
	if !ok {
		c.Errors = append(c.Errors, fmt.Sprintf("Entry point at 0x%04x must be a dc.w with a label"))
		return
	}
	*s = call.LabelName
}

func (p *Program) Compile(filename string) (c *Compilation) {
	llvm.InitializeNativeTarget()

	c = new(Compilation)
	c.program = p
	c.Warnings = []string{}
	c.Errors = []string{}
	c.mod = llvm.NewModule("asm_module")
	c.builder = llvm.NewBuilder()
	defer c.builder.Dispose()
	c.labeledData = map[string] llvm.Value{}

	// first pass to generate data declarations
	c.mode = dataStmtMode
	p.Ast.Ast(c)

	// declare i32 @putchar(i32)
	putCharType := llvm.FunctionType(llvm.Int32Type(), []llvm.Type{llvm.Int32Type()}, false)
	putCharFn := llvm.AddFunction(c.mod, "putChar", putCharType)
	putCharFn.SetLinkage(llvm.ExternalLinkage)

	// declare void @exit(i32) noreturn nounwind
	exitType := llvm.FunctionType(llvm.VoidType(), []llvm.Type{llvm.Int32Type()}, false)
	exitFn := llvm.AddFunction(c.mod, "exit", exitType)
	exitFn.AddFunctionAttr(llvm.NoReturnAttribute|llvm.NoUnwindAttribute)
	exitFn.SetLinkage(llvm.ExternalLinkage)

	// main function / entry point
	mainType := llvm.FunctionType(llvm.Int32Type(), []llvm.Type{}, false)
	c.mainFn = llvm.AddFunction(c.mod, "main", mainType)
	c.mainFn.SetFunctionCallConv(llvm.CCallConv)
	entry := llvm.AddBasicBlock(c.mainFn, "Entry")
	c.builder.SetInsertPointAtEnd(entry)
	c.builder.CreateAlloca(llvm.Int8Type(), "X")
	c.builder.CreateAlloca(llvm.Int8Type(), "Y")
	c.builder.CreateAlloca(llvm.Int8Type(), "A")
	c.builder.CreateAlloca(llvm.Int1Type(), "S_neg")
	c.builder.CreateAlloca(llvm.Int1Type(), "S_zero")

	// set up entry points
	c.setUpEntryPoint(p, 0xfffa, &c.nmiLabelName)
	c.setUpEntryPoint(p, 0xfffc, &c.resetLabelName)
	c.setUpEntryPoint(p, 0xfffe, &c.irqLabelName)

	// second pass codegen
	c.mode = compileMode
	p.Ast.Ast(c)

	// close off the final unterminated basic block
	if c.currentBlock != nil {
		c.builder.SetInsertPointAtEnd(*c.currentBlock)
		c.builder.CreateUnreachable()
	}

	// hook up entry points
	if c.nmiBlock == nil {
		c.Errors = append(c.Errors, "missing nmi entry point")
		return
	}
	if c.resetBlock == nil {
		c.Errors = append(c.Errors, "missing reset entry point")
		return
	}
	if c.irqBlock == nil {
		c.Errors = append(c.Errors, "missing irq entry point")
		return
	}

	// hook up the first entry block to the reset block
	c.builder.SetInsertPointAtEnd(entry)
	c.builder.CreateBr(*c.resetBlock)

	err := llvm.VerifyModule(c.mod, llvm.ReturnStatusAction)
	if err != nil {
		c.Errors = append(c.Errors, err.Error())
		return
	}

	engine, err := llvm.NewJITCompiler(c.mod, 3)
	if err != nil {
		c.Errors = append(c.Errors, err.Error())
		return
	}
	defer engine.Dispose()

	pass := llvm.NewPassManager()
	defer pass.Dispose()

	//pass.Add(engine.TargetData())
	//pass.AddConstantPropagationPass()
	//pass.AddInstructionCombiningPass()
	//pass.AddPromoteMemoryToRegisterPass()
	//pass.AddGVNPass()
	//pass.AddCFGSimplificationPass()
	pass.Run(c.mod)

	c.mod.Dump()

	fd, err := os.Create(filename)
	if err != nil {
		c.Errors = append(c.Errors, err.Error())
		return
	}

	err = llvm.WriteBitcodeToFile(c.mod, fd)
	if err != nil {
		c.Errors = append(c.Errors, err.Error())
		return
	}

	err = fd.Close()
	if err != nil {
		c.Errors = append(c.Errors, err.Error())
		return
	}

	return
}

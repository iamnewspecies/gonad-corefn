package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/metaleap/go-util/dev/ps"
	"github.com/metaleap/go-util/str"
)

const (
	areOverlappingInterfacesSupportedByGo = true // technically would be false, see https://github.com/golang/go/issues/6977 --- in practice keep true until it's an actual issue in generated code
	dbgEmitEmptyFuncs                     = false
)

func (_ *irAst) codeGenCommaIf(w io.Writer, i int) {
	if i > 0 {
		fmt.Fprint(w, ", ")
	}
}

func (_ *irAst) codeGenComments(w io.Writer, singlelineprefix string, comments ...*udevps.CoreComment) {
	for _, c := range comments {
		if c.BlockComment != "" {
			fmt.Fprintf(w, "/*%s*/", c.BlockComment)
		} else if c.LineComment != "" {
			fmt.Fprintf(w, "%s//%s\n", singlelineprefix, c.LineComment)
		}
	}
}

func (me *irAst) codeGenAst(w io.Writer, indent int, ast irA) {
	if ast == nil {
		return
	}
	tabs := ""
	if indent > 0 {
		tabs = strings.Repeat("\t", indent)
	}
	switch a := ast.(type) {
	case *irALitStr:
		fmt.Fprintf(w, "%q", a.LitStr)
	case *irALitBool:
		fmt.Fprintf(w, "%t", a.LitBool)
	case *irALitNum:
		s := fmt.Sprintf("%f", a.LitNum)
		for strings.HasSuffix(s, "0") {
			s = s[:len(s)-1]
		}
		fmt.Fprint(w, s)
	case *irALitInt:
		fmt.Fprintf(w, "%d", a.LitInt)
	case *irALitArr:
		me.codeGenTypeRef(w, &a.irGoNamedTypeRef, indent)
		fmt.Fprint(w, "{")
		for i, expr := range a.ArrVals {
			me.codeGenCommaIf(w, i)
			me.codeGenAst(w, indent, expr)
		}
		fmt.Fprint(w, "}")
	case *irALitObj:
		me.codeGenTypeRef(w, &a.irGoNamedTypeRef, -1)
		fmt.Fprint(w, "{")
		for i, namevaluepair := range a.ObjFields {
			me.codeGenCommaIf(w, i)
			if namevaluepair.NameGo != "" {
				fmt.Fprintf(w, "%s: ", namevaluepair.NameGo)
			}
			me.codeGenAst(w, indent, namevaluepair.FieldVal)
		}
		fmt.Fprint(w, "}")
	case *irAConst:
		fmt.Fprintf(w, "%sconst %s ", tabs, a.NameGo)
		me.codeGenTypeRef(w, a.ExprType(), -1)
		fmt.Fprint(w, " = ")
		me.codeGenAst(w, indent, a.ConstVal)
		fmt.Fprint(w, "\n")
	case *irASym:
		fmt.Fprint(w, a.NameGo)
	case *irALet:
		switch ato := a.LetVal.(type) {
		case *irAToType:
			fmt.Fprint(w, tabs)
			if a.typeConv.okname == "" {
				fmt.Fprint(w, a.NameGo)
			} else {
				if a.typeConv.vused {
					fmt.Fprint(w, a.NameGo)
				} else {
					fmt.Fprint(w, "_")
				}
				fmt.Fprint(w, ", "+a.typeConv.okname)
			}
			fmt.Fprint(w, " := ")
			me.codeGenAst(w, indent, ato)
		default:
			if at := a.ExprType(); at.Ref.F != nil && a.LetVal != nil {
				fmt.Fprintf(w, "%s%s := ", tabs, a.NameGo)
				me.codeGenAst(w, indent, a.LetVal)
			} else {
				fmt.Fprintf(w, "%svar %s ", tabs, a.NameGo)
				me.codeGenTypeRef(w, at, -1)
				if a.LetVal != nil {
					fmt.Fprint(w, " = ")
					me.codeGenAst(w, indent, a.LetVal)
				}
				if a.isTopLevel() {
					fmt.Fprint(w, "\n")
				}
			}
		}
		fmt.Fprint(w, "\n")
	case *irABlock:
		if dbgEmitEmptyFuncs && a != nil && a.parent != nil {
			me.codeGenAst(w, indent, ªRet(nil))
		} else if a == nil || len(a.Body) == 0 {
			fmt.Fprint(w, "{}")
			// } else if len(a.Body) == 1 {
			// 	fmt.Fprint(w, "{ ")
			// 	me.codeGenAst(w, -1, a.Body[0])
			// 	fmt.Fprint(w, " }")
		} else {
			fmt.Fprint(w, "{\n")
			indent++
			for _, expr := range a.Body {
				me.codeGenAst(w, indent, expr)
			}
			fmt.Fprintf(w, "%s}", tabs)
			indent-- // ineffectual; keep around in case we later switch things around
		}
	case *irAIf:
		fmt.Fprintf(w, "%sif ", tabs)
		me.codeGenAst(w, indent, a.If)
		fmt.Fprint(w, " ")
		me.codeGenAst(w, indent, a.Then)
		if a.Else != nil {
			fmt.Fprint(w, " else ")
			me.codeGenAst(w, indent, a.Else)
		}
		fmt.Fprint(w, "\n")
	case *irACall:
		me.codeGenAst(w, indent, a.Callee)
		fmt.Fprint(w, "(")
		for i, expr := range a.CallArgs {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			me.codeGenAst(w, indent, expr)
		}
		fmt.Fprint(w, ")")
	case *irAFunc:
		me.codeGenTypeRef(w, &a.irGoNamedTypeRef, indent)
		fmt.Fprint(w, " ")
		me.codeGenAst(w, indent, a.FuncImpl)
	case *irAComments:
		me.codeGenComments(w, tabs, a.Comments...)
	case *irARet:
		if a.RetArg == nil {
			fmt.Fprintf(w, "%sreturn", tabs)
		} else {
			fmt.Fprintf(w, "%sreturn ", tabs)
			me.codeGenAst(w, indent, a.RetArg)
		}
		if indent >= 0 {
			fmt.Fprint(w, "\n")
		}
	case *irAPanic:
		fmt.Fprintf(w, "%spanic(", tabs)
		me.codeGenAst(w, indent, a.PanicArg)
		fmt.Fprint(w, ")\n")
	case *irADot:
		me.codeGenAst(w, indent, a.DotLeft)
		fmt.Fprint(w, ".")
		me.codeGenAst(w, indent, a.DotRight)
	case *irAIndex:
		me.codeGenAst(w, indent, a.IdxLeft)
		fmt.Fprint(w, "[")
		me.codeGenAst(w, indent, a.IdxRight)
		fmt.Fprint(w, "]")
	case *irAIsType:
		fmt.Fprint(w, "ː"+a.names.v+"ᐧ"+a.names.t)
		// fmt.Fprint(w, typeNameWithPkgName(me.resolveGoTypeRefFromQName(a.TypeToTest)))
	case *irAToType:
		me.codeGenAst(w, indent, a.ExprToConv)
		fmt.Fprintf(w, ".(%s)", typeNameWithPkgName(me.resolveGoTypeRefFromQName(ustr.PrefixWithSep(a.TypePkg, ".", a.TypeName))))
	case *irAPkgSym:
		if a.PkgName != "" {
			if pkgimp := me.irM.ensureImp(a.PkgName, "", ""); pkgimp != nil {
				pkgimp.emitted = true
			}
			fmt.Fprintf(w, "%s.", a.PkgName)
		}
		fmt.Fprint(w, a.Symbol)
	case *irASet:
		fmt.Fprint(w, tabs)
		me.codeGenAst(w, indent, a.SetLeft)
		if a.isInVarGroup {
			fmt.Fprint(w, " ")
			me.codeGenTypeRef(w, &a.irGoNamedTypeRef, indent)
		}
		fmt.Fprint(w, " = ")
		me.codeGenAst(w, indent, a.ToRight)
		fmt.Fprint(w, "\n")
	case *irAOp1:
		po1, po2 := a.parentOp()
		parens := po2 != nil || po1 != nil
		if parens {
			fmt.Fprint(w, "(")
		}
		fmt.Fprint(w, a.Op1)
		me.codeGenAst(w, indent, a.Of)
		if parens {
			fmt.Fprint(w, ")")
		}
	case *irAOp2:
		po1, po2 := a.parentOp()
		parens := po1 != nil || (po2 != nil && (po2.Op2 != a.Op2 || (a.Op2 != "+" && a.Op2 != "*" && a.Op2 != "&&" && a.Op2 != "&" && a.Op2 != "||" && a.Op2 != "|")))
		if parens {
			fmt.Fprint(w, "(")
		}
		me.codeGenAst(w, indent, a.Left)
		fmt.Fprintf(w, " %s ", a.Op2)
		me.codeGenAst(w, indent, a.Right)
		if parens {
			fmt.Fprint(w, ")")
		}
	case *irANil:
		fmt.Fprint(w, "nil")
	case *irAFor:
		if a.ForRange != nil {
			fmt.Fprintf(w, "%sfor _, %s := range ", tabs, a.ForRange.NameGo)
			me.codeGenAst(w, indent, a.ForRange.LetVal)
			me.codeGenAst(w, indent, a.ForDo)
		} else if len(a.ForInit) > 0 || len(a.ForStep) > 0 {
			fmt.Fprint(w, "for ")

			for i, finit := range a.ForInit {
				me.codeGenCommaIf(w, i)
				fmt.Fprint(w, finit.NameGo)
			}
			fmt.Fprint(w, " := ")
			for i, finit := range a.ForInit {
				me.codeGenCommaIf(w, i)
				me.codeGenAst(w, indent, finit.LetVal)
			}
			fmt.Fprint(w, "; ")

			me.codeGenAst(w, indent, a.ForCond)
			fmt.Fprint(w, "; ")

			for i, fstep := range a.ForStep {
				me.codeGenCommaIf(w, i)
				me.codeGenAst(w, indent, fstep.SetLeft)
			}
			fmt.Fprint(w, " = ")
			for i, fstep := range a.ForStep {
				me.codeGenCommaIf(w, i)
				me.codeGenAst(w, indent, fstep.ToRight)
			}
			me.codeGenAst(w, indent, a.ForDo)
		} else {
			fmt.Fprintf(w, "%sfor ", tabs)
			me.codeGenAst(w, indent, a.ForCond)
			fmt.Fprint(w, " ")
			me.codeGenAst(w, indent, a.ForDo)
		}
		fmt.Fprint(w, "\n")
	default:
		b, _ := json.Marshal(&ast)
		fmt.Fprintf(w, "/*****%v*****/", string(b))
	}
}

func (me *irAst) codeGenGroupedVals(w io.Writer, consts bool, asts []irA) {
	if l := len(asts); l == 1 {
		me.codeGenAst(w, 0, asts[0])
	} else if l > 1 {
		if consts {
			fmt.Fprint(w, "const (\n")
		} else {
			fmt.Fprint(w, "var (\n")
		}
		valˇnameˇtype := func(a irA) (val irA, name string, typeref *irGoNamedTypeRef) {
			if ac, _ := a.(*irAConst); ac != nil && consts {
				val, name, typeref = ac.ConstVal, ac.NameGo, ac.ExprType()
			} else if av, _ := a.(*irALet); av != nil {
				val, name, typeref = av.LetVal, av.NameGo, &av.irGoNamedTypeRef
			}
			return
		}
		for i, a := range asts {
			val, name, typeref := valˇnameˇtype(a)
			me.codeGenAst(w, 1, ªsetVarInGroup(name, val, typeref))
			if i < (len(asts) - 1) {
				if _, ok := asts[i+1].(*irAComments); ok {
					fmt.Fprint(w, "\n")
				}
			}
		}
		fmt.Fprint(w, ")\n\n")
	}
}

// func (_ *irAst) codeGenEnumConsts(w io.Writer, enumconstnames []string, enumconsttype string) {
// 	fmt.Fprint(w, "const (\n")
// 	fmt.Fprintf(w, "\t_ %v= iota\n", strings.Repeat(" ", len(enumconsttype)+len(enumconstnames[0])))
// 	for i, enumconstname := range enumconstnames {
// 		fmt.Fprintf(w, "\t%s", enumconstname)
// 		if i == 0 {
// 			fmt.Fprintf(w, " %s = iota", enumconsttype)
// 		}
// 		fmt.Fprint(w, "\n")
// 	}
// 	fmt.Fprint(w, ")\n\n")
// }

func (me *irAst) codeGenFuncArgs(w io.Writer, indent int, methodargs irGoNamedTypeRefs, isretargs bool, withnames bool) {
	if dbgEmitEmptyFuncs && isretargs && withnames {
		methodargs[0].NameGo = "ret"
	}
	parens := (!isretargs) || len(methodargs) > 1 || (len(methodargs) == 1 && len(methodargs[0].NameGo) > 0)
	if parens {
		fmt.Fprint(w, "(")
	}
	if len(methodargs) > 0 {
		for i, arg := range methodargs {
			me.codeGenCommaIf(w, i)
			if withnames && arg.NameGo != "" {
				fmt.Fprintf(w, "%s ", arg.NameGo)
			}
			me.codeGenTypeRef(w, arg, indent+1)
		}
	}
	if parens {
		fmt.Fprint(w, ")")
	}
	if !isretargs {
		fmt.Fprint(w, " ")
	}
}

func (me *irAst) codeGenModImps(w io.Writer) (err error) {
	if len(me.irM.Imports) > 0 {
		modimps := make(irMPkgRefs, 0, len(me.irM.Imports))
		for _, modimp := range me.irM.Imports {
			if modimp.emitted {
				modimps = append(modimps, modimp)
			}
		}
		if len(modimps) > 0 {
			sort.Sort(modimps)
			if _, err = fmt.Fprint(w, "import (\n"); err == nil {
				wasuriform := modimps[0].isUriForm()
				for _, modimp := range modimps {
					if modimp.isUriForm() != wasuriform {
						wasuriform = !wasuriform
						_, err = fmt.Fprint(w, "\n")
					}
					if modimp.GoName == modimp.ImpPath || /*for the time being*/ true {
						_, err = fmt.Fprintf(w, "\t%q\n", modimp.ImpPath)
					} else {
						_, err = fmt.Fprintf(w, "\t%s %q\n", modimp.GoName, modimp.ImpPath)
					}
					if err != nil {
						break
					}
				}
				if err == nil {
					fmt.Fprint(w, ")\n\n")
				}
			}
		}
	}
	return
}

func (me *irAst) codeGenPkgDecl(w io.Writer) (err error) {
	_, err = fmt.Fprintf(w, "package %s\n\n", me.mod.pName)
	return
}

func (me *irAst) codeGenStructMethods(w io.Writer, tr *irGoNamedTypeRef) {
	if tr.Ref.S != nil && len(tr.Ref.S.Methods) > 0 {
		for _, method := range tr.Ref.S.Methods {
			mthis := "_"
			if tr.Ref.S.PassByPtr {
				fmt.Fprintf(w, "func (%s *%s) %s", mthis, tr.NameGo, method.NameGo)
			} else {
				fmt.Fprintf(w, "func (%s %s) %s", mthis, tr.NameGo, method.NameGo)
			}
			me.codeGenFuncArgs(w, -1, method.Ref.F.Args, false, true)
			me.codeGenFuncArgs(w, -1, method.Ref.F.Rets, true, true)
			fmt.Fprint(w, " ")
			me.codeGenAst(w, 0, method.Ref.F.impl)
			fmt.Fprint(w, "\n")
		}
		fmt.Fprint(w, "\n")
	}
}

func (me *irAst) codeGenTypeDef(w io.Writer, gtd *irGoNamedTypeRef) {
	fmt.Fprintf(w, "type %s ", gtd.NameGo)
	me.codeGenTypeRef(w, gtd, 0)
	fmt.Fprint(w, "\n\n")
}

func (me *irAst) codeGenTypeRef(w io.Writer, gtd *irGoNamedTypeRef, indlevel int) {
	if gtd == nil {
		fmt.Fprint(w, "interface{/*NIL*/}")
		return
	}
	fmtembeds := "\t%s\n"
	isfuncwithbodynotjustsig := gtd.Ref.F != nil && gtd.Ref.F.impl != nil
	if gtd.Ref.Q != nil {
		me.codeGenAst(w, -1, ªPkgSym(me.resolveGoTypeRefFromQName(gtd.Ref.Q.Q)))
	} else if gtd.Ref.A != nil {
		fmt.Fprint(w, "[]")
		me.codeGenTypeRef(w, gtd.Ref.A.Of, -1)
	} else if gtd.Ref.P != nil {
		fmt.Fprint(w, "*")
		me.codeGenTypeRef(w, gtd.Ref.P.Of, -1)
	} else if gtd.Ref.I != nil {
		if len(gtd.Ref.I.Embeds) == 0 && len(gtd.Ref.I.Methods) == 0 {
			if gtd.Ref.I.isTypeVar {
				fmt.Fprint(w, "𝒈.𝑻")
				me.irM.ensureImp("", "github.com/gonadz/-", "").emitted = true
			} else {
				fmt.Fprint(w, "interface{}")
			}
		} else {
			var tabind string
			if indlevel > 0 {
				tabind = strings.Repeat("\t", indlevel)
			}
			fmt.Fprint(w, "interface {\n")
			if areOverlappingInterfacesSupportedByGo {
				for _, ifembed := range gtd.Ref.I.Embeds {
					fmt.Fprint(w, tabind+"\t")
					me.codeGenAst(w, -1, ªPkgSym(me.resolveGoTypeRefFromQName(ifembed)))
					fmt.Fprint(w, "\n")
				}
			}
			var buf bytes.Buffer
			for _, ifmethod := range gtd.Ref.I.Methods {
				fmt.Fprint(&buf, ifmethod.NameGo)
				if ifmethod.Ref.F == nil {
					panic(notImplErr("interface-method (not a func)", ifmethod.NamePs, gtd.NamePs))
				} else {
					me.codeGenFuncArgs(&buf, indlevel, ifmethod.Ref.F.Args, false, false)
					me.codeGenFuncArgs(&buf, indlevel, ifmethod.Ref.F.Rets, true, false)
				}
				fmt.Fprint(w, tabind)
				fmt.Fprintf(w, fmtembeds, buf.String())
				buf.Reset()
			}
			fmt.Fprintf(w, "%s}", tabind)
		}
	} else if gtd.Ref.S != nil {
		var tabind string
		if indlevel > 0 {
			tabind = strings.Repeat("\t", indlevel)
		}
		if len(gtd.Ref.S.Embeds) == 0 && len(gtd.Ref.S.Fields) == 0 {
			fmt.Fprint(w, "struct{}")
		} else {
			fmt.Fprint(w, "struct {\n")
			for _, structembed := range gtd.Ref.S.Embeds {
				fmt.Fprint(w, tabind)
				fmt.Fprintf(w, fmtembeds, structembed)
			}
			fnlen := 0
			for _, structfield := range gtd.Ref.S.Fields {
				if l := len(structfield.NameGo); l > fnlen {
					fnlen = l
				}
			}
			var buf bytes.Buffer
			for _, structfield := range gtd.Ref.S.Fields {
				me.codeGenTypeRef(&buf, structfield, indlevel+1)
				fmt.Fprint(w, tabind)
				fmt.Fprintf(w, fmtembeds, ustr.PadRight(structfield.NameGo, fnlen)+" "+buf.String())
				buf.Reset()
			}
			fmt.Fprintf(w, "%s}", tabind)
		}
	} else if gtd.Ref.F != nil {
		fmt.Fprint(w, "func")
		if isfuncwithbodynotjustsig && gtd.NameGo != "" {
			fmt.Fprintf(w, " %s", gtd.NameGo)
		}
		me.codeGenFuncArgs(w, indlevel, gtd.Ref.F.Args, false, isfuncwithbodynotjustsig)
		me.codeGenFuncArgs(w, indlevel, gtd.Ref.F.Rets, true, isfuncwithbodynotjustsig)
	} else {
		fmt.Fprint(w, "interface{/*EMPTY*/}")
	}
}
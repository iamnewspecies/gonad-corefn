package main

import (
	"fmt"
	"strings"

	"github.com/metaleap/go-util/slice"
)

type irGoNamedTypeRefs []*irGoNamedTypeRef

func (me irGoNamedTypeRefs) Len() int { return len(me) }
func (me irGoNamedTypeRefs) Less(i, j int) bool {
	if me[i].sortIndex != me[j].sortIndex {
		return me[i].sortIndex < me[j].sortIndex
	}
	return strings.ToLower(me[i].NameGo) < strings.ToLower(me[j].NameGo)
}
func (me irGoNamedTypeRefs) Swap(i, j int) { me[i], me[j] = me[j], me[i] }

func (me irGoNamedTypeRefs) byPsName(psname string) *irGoNamedTypeRef {
	for _, gntr := range me {
		if gntr.NamePs == psname {
			return gntr
		}
	}
	return nil
}

func (me irGoNamedTypeRefs) equiv(cmp irGoNamedTypeRefs) bool {
	if l := len(me); l == len(cmp) {
		for i := 0; i < l; i++ {
			if !me[i].equiv(cmp[i]) {
				return false
			}
		}
		return true
	}
	return false
}

type irGoNamedTypeRef struct {
	NamePs string      `json:",omitempty"`
	NameGo string      `json:",omitempty"`
	Export bool        `json:",omitempty"`
	Ref    irGoTypeRef `json:",omitempty"`

	sortIndex int
}

type irGoTypeRef struct {
	A *irGoTypeRefArray     `json:",omitempty"`
	F *irGoTypeRefFunc      `json:",omitempty"`
	I *irGoTypeRefInterface `json:",omitempty"`
	P *irGoTypeRefPtr       `json:",omitempty"`
	Q *irGoTypeRefAlias     `json:",omitempty"`
	S *irGoTypeRefStruct    `json:",omitempty"`
}

func (me *irGoNamedTypeRef) clearTypeInfo() {
	me.Ref.Q, me.Ref.I, me.Ref.F, me.Ref.S, me.Ref.A, me.Ref.P = nil, nil, nil, nil, nil, nil
}

func (me *irGoNamedTypeRef) copyFrom(from *irGoNamedTypeRef, names bool, trefs bool, export bool) {
	if names {
		me.NameGo, me.NamePs = from.NameGo, from.NamePs
	}
	if trefs {
		me.Ref.Q, me.Ref.I, me.Ref.F, me.Ref.S, me.Ref.A, me.Ref.P = from.Ref.Q, from.Ref.I, from.Ref.F, from.Ref.S, from.Ref.A, from.Ref.P
	}
	if export {
		me.Export = from.Export
	}
}

func (me *irGoNamedTypeRef) copyTypeInfoFrom(from *irGoNamedTypeRef) {
	me.copyFrom(from, false, true, false)
}

func (me *irGoNamedTypeRef) nameless() (copy *irGoNamedTypeRef) {
	copy = &irGoNamedTypeRef{}
	copy.copyTypeInfoFrom(me)
	return
}

func (me *irGoNamedTypeRef) equiv(cmp *irGoNamedTypeRef) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.Ref.Q.equiv(cmp.Ref.Q) && me.Ref.I.equiv(cmp.Ref.I) && me.Ref.F.equiv(cmp.Ref.F) && me.Ref.S.equiv(cmp.Ref.S) && me.Ref.A.equiv(cmp.Ref.A) && me.Ref.P.equiv(cmp.Ref.P))
}

func (me *irGoNamedTypeRef) hasName() bool {
	return me.NamePs != ""
}

func (me *irGoNamedTypeRef) hasTypeInfoBeyondEmptyIface() (welltyped bool) {
	if welltyped = me.hasTypeInfo(); welltyped && me.Ref.I != nil {
		welltyped = len(me.Ref.I.Embeds) > 0 || len(me.Ref.I.Methods) > 0
	}
	return
}

func (me *irGoNamedTypeRef) hasTypeInfo() bool {
	return me != nil && (me.Ref.Q != nil || me.Ref.A != nil || me.Ref.F != nil || me.Ref.I != nil || me.Ref.P != nil || me.Ref.S != nil)
}

func (me *irGoNamedTypeRef) setBothNamesFromPsName(psname string) {
	me.NamePs = psname
	me.NameGo = sanitizeSymbolForGo(psname, me.Export)
}

func (me *irGoTypeRef) setFrom(tref interface{}) {
	me.A, me.F, me.I, me.P, me.Q, me.S = nil, nil, nil, nil, nil, nil
	switch tr := tref.(type) {
	case *irGoTypeRef:
		me.Q = tr.Q
		me.A = tr.A
		me.F = tr.F
		me.I = tr.I
		me.P = tr.P
		me.S = tr.S
	case *irGoTypeRefInterface:
		me.I = tr
	case *irGoTypeRefFunc:
		me.F = tr
	case *irGoTypeRefStruct:
		me.S = tr
	case *irGoTypeRefArray:
		me.A = tr
	case *irGoTypeRefPtr:
		me.P = tr
	case *irGoTypeRefAlias:
		me.Q = tr
	case nil:
	default:
		panicWithType("setFrom", tref, "tref")
	}
}

func (me *irGoNamedTypeRef) turnRefIntoRefPtr() {
	refptr := &irGoTypeRefPtr{Of: &irGoNamedTypeRef{}}
	refptr.Of.copyTypeInfoFrom(me)
	me.Ref.Q, me.Ref.A, me.Ref.F, me.Ref.I, me.Ref.P, me.Ref.S = nil, nil, nil, nil, refptr, nil
}

type irGoTypeRefAlias struct {
	Q string
}

func (me *irGoTypeRefAlias) equiv(cmp *irGoTypeRefAlias) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.Q == cmp.Q)
}

type irGoTypeRefArray struct {
	Of *irGoNamedTypeRef
}

func (me *irGoTypeRefArray) equiv(cmp *irGoTypeRefArray) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.Of.equiv(cmp.Of))
}

type irGoTypeRefPtr struct {
	Of *irGoNamedTypeRef
}

func (me *irGoTypeRefPtr) equiv(cmp *irGoTypeRefPtr) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.Of.equiv(cmp.Of))
}

type irGoTypeRefInterface struct {
	Embeds  []string          `json:",omitempty"`
	Methods irGoNamedTypeRefs `json:",omitempty"`

	isTypeVar        bool
	xtc              *irPsTypeClass
	xtd              *irPsTypeDataDef
	inheritedMethods irGoNamedTypeRefs
}

func (me *irGoTypeRefInterface) equiv(cmp *irGoTypeRefInterface) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.isTypeVar == cmp.isTypeVar && uslice.StrEq(me.Embeds, cmp.Embeds) && me.Methods.equiv(cmp.Methods))
}

type irGoTypeRefFunc struct {
	Args irGoNamedTypeRefs `json:",omitempty"`
	Rets irGoNamedTypeRefs `json:",omitempty"`

	impl *irABlock
}

func (me *irGoTypeRefFunc) copyArgTypesOnlyFrom(namesIfMeNil bool, from *irGoTypeRefFunc) {
	copyargs := func(meargs irGoNamedTypeRefs, fromargs irGoNamedTypeRefs) irGoNamedTypeRefs {
		if numargsme := len(meargs); numargsme == 0 {
			for _, arg := range fromargs {
				mearg := &irGoNamedTypeRef{}
				mearg.copyFrom(arg, namesIfMeNil, true, false)
				meargs = append(meargs, mearg)
			}
		} else if numargsfrom := len(fromargs); numargsme != numargsfrom {
			panic(notImplErr("args-num mismatch", fmt.Sprintf("%v vs %v", numargsme, numargsfrom), "copyArgTypesFrom"))
		} else {
			for i := 0; i < numargsme; i++ {
				meargs[i].copyTypeInfoFrom(fromargs[i])
			}
		}
		return meargs
	}
	if from != nil {
		me.Args = copyargs(me.Args, from.Args)
		me.Rets = copyargs(me.Rets, from.Rets)
	}
}

func (me *irGoTypeRefFunc) equiv(cmp *irGoTypeRefFunc) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.Args.equiv(cmp.Args) && me.Rets.equiv(cmp.Rets))
}

func (me *irGoTypeRefFunc) forEachArgAndRet(on func(*irGoNamedTypeRef)) {
	for _, a := range me.Args {
		on(a)
	}
	for _, r := range me.Rets {
		on(r)
	}
}

func (me *irGoTypeRefFunc) haveAllArgsTypeInfo() bool {
	for _, arg := range me.Args {
		if !arg.hasTypeInfo() {
			return false
		}
	}
	for _, ret := range me.Rets {
		if !ret.hasTypeInfo() {
			return false
		}
	}
	return true
}

func (me *irGoTypeRefFunc) haveAnyArgsTypeInfo() bool {
	for _, arg := range me.Args {
		if arg.hasTypeInfo() {
			return true
		}
	}
	for _, ret := range me.Rets {
		if ret.hasTypeInfo() {
			return true
		}
	}
	return false
}

func (me *irGoTypeRefFunc) toSig(forceretarg bool) (rf *irGoTypeRefFunc) {
	rf = &irGoTypeRefFunc{}
	for _, arg := range me.Args {
		rf.Args = append(rf.Args, arg.nameless())
	}
	if len(me.Rets) == 0 && forceretarg {
		rf.Rets = append(rf.Rets, &irGoNamedTypeRef{})
	} else {
		for _, ret := range me.Rets {
			rf.Rets = append(rf.Rets, ret.nameless())
		}
	}
	return
}

type irGoTypeRefStruct struct {
	Embeds    []string          `json:",omitempty"`
	Fields    irGoNamedTypeRefs `json:",omitempty"`
	PassByPtr bool              `json:",omitempty"`
	Methods   irGoNamedTypeRefs `json:",omitempty"`
}

func (me *irGoTypeRefStruct) equiv(cmp *irGoTypeRefStruct) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && uslice.StrEq(me.Embeds, cmp.Embeds) && me.Fields.equiv(cmp.Fields))
}

func (me *irGoTypeRefStruct) memberByPsName(nameps string) (mem *irGoNamedTypeRef) {
	if mem = me.Fields.byPsName(nameps); mem == nil {
		mem = me.Methods.byPsName(nameps)
	}
	return
}

func (me *irMeta) goTypeDefByGoName(goname string) *irGoNamedTypeRef {
	for _, gtd := range me.GoTypeDefs {
		if gtd.NameGo == goname {
			return gtd
		}
	}
	return nil
}

func (me *irMeta) goTypeDefByPsName(psname string) *irGoNamedTypeRef {
	var gtdi *irGoNamedTypeRef
	for _, gtd := range me.GoTypeDefs {
		if gtd.NamePs == psname {
			if gtd.Ref.I != nil {
				gtdi = gtd
			} else {
				return gtd
			}
		}
	}
	return gtdi
}

func (me *irAst) resolveGoTypeRefFromQName(tref string) (pname string, tname string) {
	var mod *modPkg
	wasprim := false
	i := strings.LastIndex(tref, ".")
	if tname = tref[i+1:]; i > 0 {
		pname = tref[:i]
		if pname == me.mod.qName {
			pname = ""
			mod = me.mod
		} else if wasprim = (pname == "Prim"); wasprim {
			pname = ""
			switch tname {
			case "Char":
				tname = "rune"
			case "String":
				tname = "string"
			case "Boolean":
				tname = "bool"
			case "Number":
				tname = "float64"
			case "Int":
				tname = "int"
			default:
				tname = "interface{/*Prim." + tname + "*/}"
				println(me.mod.srcFilePath + "\t" + tref + "\t" + tname)
			}
		} else {
			qn, foundimport, isffi := pname, false, strings.HasPrefix(pname, prefixDefaultFfiPkgNs)
			if !isffi {
				if mod = findModuleByQName(qn); mod == nil {
					if mod = findModuleByPName(qn); mod == nil {
						panic(notImplErr("module qname", qn, me.mod.srcFilePath))
					}
				}
				pname = mod.pName
			}
			for _, imp := range me.irM.Imports {
				if imp.PsModQName == qn {
					foundimport = true
					break
				}
			}
			if !foundimport {
				var imp *irMPkgRef
				if isffi {
					imp = &irMPkgRef{ImpPath: prefixDefaultFfiPkgImpPath + strReplDot2Slash.Replace(qn)}
				} else {
					imp = mod.newModImp()
				}
				me.irM.imports, me.irM.Imports = append(me.irM.imports, mod), append(me.irM.Imports, imp)
			}
		}
	} else {
		mod = me.mod
	}
	if (!wasprim) && mod != nil {
		if gtd := mod.irMeta.goTypeDefByPsName(tname); gtd != nil {
			tname = gtd.NameGo
		}
	}
	return
}
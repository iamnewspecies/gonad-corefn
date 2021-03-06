package main

func (me *irAst) prepFromCore() {
	me.Block.Comments = me.mod.coreFn.Comments

	for i := range me.mod.coreFn.Decls {
		me.intoFromˇDecl(&me.Block, &me.mod.coreFn.Decls[i])
	}

	me.prepInitialFixups()
}

func (me *irAst) prepInitialFixups() {
	me.walk(func(subast irA) irA {
		switch a := subast.(type) {
		case *irALet:
			if ProjCfg.CodeGen.VarsAsConstsWherePossible && a.isConstable() {
				//	turn var=literal's into consts
				c := ªConst(&a.irGoNamedTypeRef, a.LetVal)
				c.copyTypeInfoFrom(a.ExprType())
				c.parent = a.parent
				return c
			}
		}
		return subast
	})
}

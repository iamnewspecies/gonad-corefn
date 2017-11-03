package main

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/metaleap/go-util/dev/bower"
	"github.com/metaleap/go-util/dev/go"
	"github.com/metaleap/go-util/fs"
)

type psBowerFile struct {
	udevbower.BowerFile

	Gonad struct { // all settings in here apply to all Deps equally as they do to the main Proj --- ie. the former get a copy of the latter, ignoring their own Gonad field even if present
		In struct {
			CoreFilesDirPath string // dir path containing Some.Module.QName/corefn.json files
		}
		Out struct {
			DumpAst         bool   // dumps an additional gonad.ast.json next to gonad.json
			MainDepLevel    int    // temporary option
			GoDirSrcPath    string // defaults to the first `GOPATH` found that has a `src` sub-directory
			GoNamespaceProj string
			GoNamespaceDeps string
		}
		CodeGen struct {
			TypeAliasesForNewtypes         bool
			TypeAliasesForSingletonStructs bool
			DataTypeAssertionMethods       bool
			PtrStructMinFieldCount         int
			Fmt                            struct {
				Reserved_Keywords    string
				Reserved_Identifiers string
				StructName_InstImpl  string
				StructName_DataCtor  string
				FieldName_DataCtor   string
				IfaceName_TypeClass  string
				Method_ThisName      string
			}
		}

		loadedFromJson bool
	}
}

type psBowerProject struct {
	BowerJsonFile     psBowerFile
	BowerJsonFilePath string
	DepsDirPath       string
	SrcDirPath        string
	Modules           []*modPkg
	GoOut             struct {
		PkgDirPath string
	}
}

func (me *psBowerProject) ensureOutDirs() (err error) {
	dirpath := filepath.Join(Proj.BowerJsonFile.Gonad.Out.GoDirSrcPath, me.GoOut.PkgDirPath)
	if err = ufs.EnsureDirExists(dirpath); err == nil {
		for _, depmod := range me.Modules {
			if err = ufs.EnsureDirExists(filepath.Join(dirpath, depmod.goOutDirPath)); err != nil {
				break
			}
		}
	}
	return
}

func (me *psBowerProject) moduleByQName(qname string) *modPkg {
	if qname != "" {
		for _, m := range me.Modules {
			if m.qName == qname {
				return m
			}
		}
	}
	return nil
}

func (me *psBowerProject) moduleByPName(pname string) *modPkg {
	if pname != "" {
		pᛌname := strReplUnderscore2ꓸ.Replace(pname)
		for _, m := range me.Modules {
			if m.pName == pᛌname || m.pName == pname {
				return m
			}
		}
	}
	return nil
}

func (me *psBowerProject) populateCfgDefaults() {
	cfg := &Proj.BowerJsonFile.Gonad
	if cfg.In.CoreFilesDirPath == "" {
		cfg.In.CoreFilesDirPath = "output"
	}
	if cfg.Out.GoNamespaceProj == "" {
		panic("missing in bower.json: `Gonad{Out{GoNamespaceProj=\"...\"}}` setting (the directory path relative to either your GOPATH or the specified `Gonad{Out{GoDirSrcPath=\"...\"}}`)")
	}
	if cfg.Out.GoDirSrcPath == "" {
		for _, gopath := range udevgo.AllGoPaths() {
			if cfg.Out.GoDirSrcPath = filepath.Join(gopath, "src"); ufs.DirExists(cfg.Out.GoDirSrcPath) {
				break
			}
		}
	}
	if cfg.CodeGen.PtrStructMinFieldCount == 0 {
		cfg.CodeGen.PtrStructMinFieldCount = 2
	}
	if cfg.CodeGen.TypeAliasesForSingletonStructs {
		cfg.CodeGen.DataTypeAssertionMethods = true
	}

	fmts := &cfg.CodeGen.Fmt
	if fmts.StructName_InstImpl == "" {
		fmts.StructName_InstImpl = "ᛌ%s"
	}
	if fmts.IfaceName_TypeClass == "" {
		fmts.IfaceName_TypeClass = "%sᛌ"
	}
	if fmts.StructName_DataCtor == "" {
		fmts.StructName_DataCtor = "{D}۰{C}"
	}
	if fmts.FieldName_DataCtor == "" {
		fmts.FieldName_DataCtor = "{C}ˈ{I}"
	}
	if fmts.Reserved_Keywords == "" {
		fmts.Reserved_Keywords = "%sʾ"
	}
	if fmts.Reserved_Identifiers == "" {
		fmts.Reserved_Identifiers = "ʾ%s"
	}
	if fmts.Method_ThisName == "" {
		fmts.Method_ThisName = "me"
	}
}

func (me *psBowerProject) loadFromJsonFile() (err error) {
	if err = udevbower.LoadFromFile(me.BowerJsonFilePath, &me.BowerJsonFile); err == nil {
		// populate defaults for Gonad sub-fields
		cfg, isdep := &me.BowerJsonFile.Gonad, me != &Proj
		if isdep {
			cfg = &Proj.BowerJsonFile.Gonad
		} else {
			me.populateCfgDefaults()
			err = ufs.EnsureDirExists(cfg.Out.GoDirSrcPath)
			cfg.loadedFromJson = true
		}
		if err == nil {
			// proceed
			me.GoOut.PkgDirPath = cfg.Out.GoNamespaceProj
			if isdep && cfg.Out.GoNamespaceDeps != "" {
				me.GoOut.PkgDirPath = cfg.Out.GoNamespaceDeps
				if repourl := me.BowerJsonFile.RepositoryURLParsed(); repourl != nil && repourl.Path != "" {
					if i := strings.LastIndex(repourl.Path, "."); i > 0 {
						me.GoOut.PkgDirPath = filepath.Join(cfg.Out.GoNamespaceDeps, repourl.Path[:i])
					} else {
						me.GoOut.PkgDirPath = filepath.Join(cfg.Out.GoNamespaceDeps, repourl.Path)
					}
				}
				if me.GoOut.PkgDirPath = strings.Trim(me.GoOut.PkgDirPath, "/\\"); !strings.HasSuffix(me.GoOut.PkgDirPath, me.BowerJsonFile.Name) {
					me.GoOut.PkgDirPath = filepath.Join(me.GoOut.PkgDirPath, me.BowerJsonFile.Name)
				}
				if me.BowerJsonFile.Version != "" {
					me.GoOut.PkgDirPath = filepath.Join(me.GoOut.PkgDirPath, me.BowerJsonFile.Version)
				}
			}
			gopkgdir := filepath.Join(cfg.Out.GoDirSrcPath, me.GoOut.PkgDirPath)
			ufs.WalkAllFiles(me.SrcDirPath, func(relpath string) bool {
				if relpath = strings.TrimLeft(relpath[len(me.SrcDirPath):], "\\/"); strings.HasSuffix(relpath, ".purs") {
					me.addModPkgFromPsSrcFileIfCoreFiles(relpath, gopkgdir)
				}
				return true
			})
		}
	}
	if err != nil {
		err = errors.New(me.BowerJsonFilePath + ": " + err.Error())
	}
	return
}

func (me *psBowerProject) addModPkgFromPsSrcFileIfCoreFiles(relpath string, gopkgdir string) {
	i, l, opt := strings.LastIndexAny(relpath, "/\\"), len(relpath)-5, Proj.BowerJsonFile.Gonad
	modinfo := &modPkg{
		proj: me, srcFilePath: filepath.Join(me.SrcDirPath, relpath),
		qName: strReplFsSlash2Dot.Replace(relpath[:l]), lName: relpath[i+1 : l],
	}
	if modinfo.impFilePath = filepath.Join(opt.In.CoreFilesDirPath, modinfo.qName, "coreimp.json"); ufs.FileExists(modinfo.impFilePath) {
		if modinfo.cfnFilePath = filepath.Join(opt.In.CoreFilesDirPath, modinfo.qName, "corefn.json"); ufs.FileExists(modinfo.cfnFilePath) {
			modinfo.pName = strReplDot2ꓸ.Replace(modinfo.qName)
			modinfo.extFilePath = filepath.Join(opt.In.CoreFilesDirPath, modinfo.qName, "externs.json")
			modinfo.irMetaFilePath = filepath.Join(opt.In.CoreFilesDirPath, modinfo.qName, "gonad.json")
			modinfo.goOutDirPath = relpath[:l]
			modinfo.goOutFilePath = filepath.Join(modinfo.goOutDirPath, modinfo.qName) + ".go"
			modinfo.gopkgfilepath = filepath.Join(gopkgdir, modinfo.goOutFilePath)
			if ufs.FileExists(modinfo.irMetaFilePath) && ufs.FileExists(modinfo.gopkgfilepath) {
				stalemetaˇimp, _ := ufs.IsNewerThan(modinfo.impFilePath, modinfo.irMetaFilePath)
				stalepkgˇimp, _ := ufs.IsNewerThan(modinfo.impFilePath, modinfo.gopkgfilepath)
				stalemetaˇcfn, _ := ufs.IsNewerThan(modinfo.cfnFilePath, modinfo.irMetaFilePath)
				stalepkgˇcfn, _ := ufs.IsNewerThan(modinfo.cfnFilePath, modinfo.gopkgfilepath)
				stalemetaˇext, _ := ufs.IsNewerThan(modinfo.extFilePath, modinfo.irMetaFilePath)
				stalepkgˇext, _ := ufs.IsNewerThan(modinfo.extFilePath, modinfo.gopkgfilepath)
				modinfo.reGenIr = stalemetaˇimp || stalepkgˇimp || stalemetaˇcfn || stalepkgˇcfn || stalemetaˇext || stalepkgˇext
			} else {
				modinfo.reGenIr = true
			}
			me.Modules = append(me.Modules, modinfo)
		}
	}
}

func (me *psBowerProject) forAll(op func(*sync.WaitGroup, *modPkg)) {
	var wg sync.WaitGroup
	for _, modinfo := range me.Modules {
		wg.Add(1)
		go op(&wg, modinfo)
	}
	wg.Wait()
}

func (me *psBowerProject) ensureModPkgIrMetas() {
	me.forAll(func(wg *sync.WaitGroup, modinfo *modPkg) {
		defer wg.Done()
		var err error
		if modinfo.reGenIr || Flag.ForceAll {
			err = modinfo.reGenPkgIrMeta()
		} else if err = modinfo.loadPkgIrMeta(); err != nil {
			modinfo.reGenIr = true // we capture this so the .go file later also gets re-gen'd from the re-gen'd IRs
			println(modinfo.qName + ": regenerating due to error when loading " + modinfo.irMetaFilePath + ": " + err.Error())
			err = modinfo.reGenPkgIrMeta()
		}
		if err != nil {
			panic(err)
		}
	})
}

func (me *psBowerProject) populateModPkgIrMetas() {
	me.forAll(func(wg *sync.WaitGroup, modinfo *modPkg) {
		defer wg.Done()
		modinfo.populatePkgIrMeta()
	})
}

func (me *psBowerProject) prepModPkirAsts() {
	me.forAll(func(wg *sync.WaitGroup, modinfo *modPkg) {
		defer wg.Done()
		if modinfo.reGenIr || Flag.ForceAll {
			modinfo.prepIrAst()
		}
	})
}

func (me *psBowerProject) reGenModPkirAsts() {
	me.forAll(func(wg *sync.WaitGroup, modinfo *modPkg) {
		defer wg.Done()
		if modinfo.reGenIr || Flag.ForceAll {
			modinfo.reGenPkgIrAst()
		}
	})
}

func (me *psBowerProject) writeOutFiles() {
	me.forAll(func(wg *sync.WaitGroup, m *modPkg) {
		defer wg.Done()
		if m.irMeta.isDirty || m.reGenIr || Flag.ForceAll {
			//	maybe gonad.json
			err := m.writeIrMetaFile()
			if err == nil && (m.reGenIr || Flag.ForceAll) {
				//	maybe gonad.ast.json
				if Proj.BowerJsonFile.Gonad.Out.DumpAst {
					err = m.writeIrAstFile()
				}
				//	maybe .go file
				if err == nil {
					err = m.writeGoFile()
				}
			}
			if err != nil {
				panic(err)
			}
		}
	})
	return
}

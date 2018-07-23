package generator

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"github.com/go-courier/loaderx"
	"github.com/go-courier/reflectx/typesutil"
	"golang.org/x/tools/go/loader"

	"github.com/go-courier/statuserror"
)

func NewStatusErrorScanner(program *loader.Program) *StatusErrorScanner {
	return &StatusErrorScanner{
		program: program,
	}
}

type StatusErrorScanner struct {
	program      *loader.Program
	StatusErrors map[*types.TypeName][]*statuserror.StatusErr
}

func sortedStatusErrList(list []*statuserror.StatusErr) []*statuserror.StatusErr {
	sort.Slice(list, func(i, j int) bool {
		return list[i].Code < list[j].Code
	})
	return list
}

func (scanner *StatusErrorScanner) StatusError(typeName *types.TypeName) []*statuserror.StatusErr {
	if typeName == nil {
		return nil
	}

	if statusErrs, ok := scanner.StatusErrors[typeName]; ok {
		return sortedStatusErrList(statusErrs)
	}

	if !strings.Contains(typeName.Type().Underlying().String(), "int") {
		panic(fmt.Errorf("status error type underlying must be an int or uint, but got %s", typeName.String()))
	}

	prog := loaderx.NewProgram(scanner.program)

	pkgInfo := prog.Package(typeName.Pkg().Path())
	if pkgInfo == nil {
		return nil
	}

	serviceCode := 0

	method, ok := typesutil.FromTType(typeName.Type()).MethodByName("ServiceCode")
	if ok {
		results, n := loaderx.NewProgram(scanner.program).FuncResultsOf(method.(*typesutil.TMethod).Func)
		if n == 1 {
			ret := results[0][0]
			if ret.IsValue() {
				if i, err := strconv.ParseInt(ret.Value.String(), 10, 64); err == nil {
					serviceCode = int(i)
				}
			}
		}
	}

	for ident, def := range pkgInfo.Defs {
		typeConst, ok := def.(*types.Const)
		if !ok {
			continue
		}
		if typeConst.Type() != typeName.Type() {
			continue
		}

		key := typeConst.Name()
		code, _ := strconv.ParseInt(typeConst.Val().String(), 10, 64)

		msg, canBeTalkError := ParseStatusErrMsg(ident.Obj.Decl.(*ast.ValueSpec).Doc.Text())

		scanner.addStatusError(typeName, key, int(code)+serviceCode, msg, canBeTalkError)
	}

	return sortedStatusErrList(scanner.StatusErrors[typeName])
}

func ParseStatusErrMsg(s string) (string, bool) {
	firstLine := strings.Split(strings.TrimSpace(s), "\n")[0]

	prefix := "@errTalk "
	if strings.HasPrefix(firstLine, prefix) {
		return firstLine[len(prefix):], true
	}
	return firstLine, false
}

func (scanner *StatusErrorScanner) addStatusError(
	typeName *types.TypeName,
	key string, code int, msg string, canBeTalkError bool,
) {
	if scanner.StatusErrors == nil {
		scanner.StatusErrors = map[*types.TypeName][]*statuserror.StatusErr{}
	}

	statusErr := statuserror.NewStatusErr(key, code, msg)
	if canBeTalkError {
		statusErr = statusErr.EnableErrTalk()
	}
	scanner.StatusErrors[typeName] = append(scanner.StatusErrors[typeName], statusErr)
}

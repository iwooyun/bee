// @Time : 2019/9/11 18:29
// @Author : duanqiangwen
// @File : g_validation
// @Software: GoLand
package validation

import (
	"errors"
	"fmt"
	beeLogger "github.com/iwooyun/bee/logger"
	"github.com/iwooyun/bee/logger/colors"
	bu "github.com/iwooyun/bee/utils"
	"github.com/astaxie/beego/swagger"
	"github.com/astaxie/beego/utils"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

const (
	astTypeArray               = "array"
	validatorControllerMapName = "commentsValidator_routers"
	validatorSuffix            = "Validator"
)

var (
	controllerList map[string]map[string]*swagger.Item
	importList     map[string]string
	moduleList     map[string]map[string]string
	funcList       map[string]map[string]string
	rootApi        swagger.Swagger
)

// refer to builtin.go
var basicTypes = map[string]string{
	"bool":       "bool",
	"uint":       "uint",
	"uint8":      "uint8",
	"uint16":     "uint16",
	"uint32":     "uint32",
	"uint64":     "uint64",
	"int":        "int",
	"int8":       "int8",
	"int16":      "int16",
	"int32":      "int32",
	"int64":      "int64",
	"uintptr":    "int64",
	"float32":    "float32",
	"float64":    "float64",
	"string":     "string",
	"complex64":  "float",
	"complex128": "double",
	"byte":       "byte",
	"rune":       "byte",
	// builtin golang objects
	"time.Time":       "string",
	"json.RawMessage": "object",

	"integer": "int",
	"boolean": "bool",
	"number":  "int",
	"float":   "float32",
	"double":  "float64",
}

func init() {
	importList = make(map[string]string)
	controllerList = make(map[string]map[string]*swagger.Item)
	moduleList = make(map[string]map[string]string)
	funcList = make(map[string]map[string]string)
}

func GenerateValidation(currentPath string) {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, filepath.Join(currentPath, "routers", "router.go"), nil, parser.ParseComments)
	if err != nil {
		beeLogger.Log.Fatalf("Error while parsing router.go: %s", err)
	}

	// Analyse controller package
	for _, im := range f.Imports {
		localName := ""
		if im.Name != nil {
			localName = im.Name.Name
		}
		analyseControllerPkg(path.Join(currentPath, "vendor"), localName, im.Path.Value)
	}

	for _, d := range f.Decls {
		switch specDecl := d.(type) {
		case *ast.FuncDecl:
			for _, l := range specDecl.Body.List {
				switch stmt := l.(type) {
				case *ast.AssignStmt:
					for _, l := range stmt.Rhs {
						if v, ok := l.(*ast.CallExpr); ok {
							// Analyze NewNamespace, it will return version and the subfunction
							selExpr, selOK := v.Fun.(*ast.SelectorExpr)
							if !selOK || selExpr.Sel.Name != "NewNamespace" {
								continue
							}
							version, params := analyseNewNamespace(v)
							if rootApi.BasePath == "" && version != "" {
								rootApi.BasePath = version
							}
							for _, p := range params {
								switch pp := p.(type) {
								case *ast.CallExpr:
									if selname := pp.Fun.(*ast.SelectorExpr).Sel.String(); selname == "NSNamespace" {
										s, params := analyseNewNamespace(pp)
										for _, sp := range params {
											switch pp := sp.(type) {
											case *ast.CallExpr:
												if pp.Fun.(*ast.SelectorExpr).Sel.String() == "NSInclude" {
													analyseNSInclude(s, pp)
												}
											}
										}
									} else if selname == "NSInclude" {
										analyseNSInclude("", pp)
									}
								}
							}
						}

					}
				}
			}
		}
	}

	validPath := path.Join(currentPath, "controllers", "validator")
	_ = os.Mkdir(validPath, 0777)
	var (
		mapLines   []string
		constLines []string
		validList  map[string][]map[string][]swagger.Parameter
	)
	validList = make(map[string][]map[string][]swagger.Parameter)
	for rt, item := range rootApi.Paths {
		moduleObj := moduleList[rt]
		module := moduleObj["module"]
		if _, ok := validList[module]; !ok {
			constLines = append(constLines, fmt.Sprintf("%s = \"%s\"", module+validatorSuffix, module+validatorSuffix))
		}
		mapLines = append(mapLines, fmt.Sprintf(`GlobalControllerValidator["%s"] = ValidComments{
				Validator:%s,
				Method:"%s",
			}`, rootApi.BasePath+rt, module+validatorSuffix, moduleObj["funcName"]))
		var parameters []swagger.Parameter
		if item.Get != nil {
			parameters = item.Get.Parameters
		} else if item.Post != nil {
			parameters = item.Post.Parameters
		}
		validList[module] = append(validList[module], map[string][]swagger.Parameter{
			rt: parameters,
		})
	}
	writeMapFile(validPath, mapLines, constLines)

	for validator, rtSlice := range validList {
		var funcSlice []string
		for _, rtMap := range rtSlice {
			for rt, parameters := range rtMap {
				funcSlice = append(funcSlice, genMethodCode(validator, moduleList[rt]["funcName"], parameters))
			}
		}
		writeFile(validPath, validator, funcSlice)
	}
}

func writeFile(validPath string, module string, funcSlice []string) {
	w := colors.NewColorWriter(os.Stdout)
	filename := bu.SnakeString(module) + "_valid"
	fPath := path.Join(validPath, filename+".go")
	f, err := openFile(fPath)
	if err != nil {
		return
	}

	fileStr := strings.Replace(VALIDTPL, "{{moduleName}}", module, -1)
	fileStr = strings.Replace(fileStr, "{{methodList}}", strings.Join(funcSlice, "\n"), -1)

	if _, err := f.WriteString(fileStr); err != nil {
		beeLogger.Log.Errorf("Could not write model file to '%s': %s", fPath, err)
		return
	}

	bu.CloseFile(f)
	fmt.Fprintf(w, "\t%s%screate%s\t %s%s\n", "\x1b[32m", "\x1b[1m", "\x1b[21m", fPath, "\x1b[0m")
	bu.FormatSourceCode(fPath)
}

func writeMapFile(validPath string, mapLines []string, constLines []string) {
	w := colors.NewColorWriter(os.Stdout)
	fPath := path.Join(validPath, validatorControllerMapName+".go")
	f, err := openFile(fPath)
	if err != nil {
		return
	}

	fileStr := strings.Replace(MAPPERTPL, "{{mapList}}", strings.Join(mapLines, "\n"), -1)
	fileStr = strings.Replace(fileStr, "{{constList}}", strings.Join(constLines, "\n"), -1)

	if _, err := f.WriteString(fileStr); err != nil {
		beeLogger.Log.Errorf("Could not write model file to '%s': %s", fPath, err)
		return
	}

	bu.CloseFile(f)
	fmt.Fprintf(w, "\t%s%screate%s\t %s%s\n", "\x1b[32m", "\x1b[1m", "\x1b[21m", fPath, "\x1b[0m")
	bu.FormatSourceCode(fPath)
}

func genMethodCode(module string, funcName string, parameters []swagger.Parameter) string {
	var (
		methodList     string
		parameterRules string
	)
	parameterRules = ""
	for _, parameter := range parameters {
		param := strings.ToLower(bu.CamelCase(parameter.Name)[0:1]) + bu.CamelCase(parameter.Name)[1:]
		switch parameter.Type {
		case "int":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetInt", parameter.Name)
		case "int64":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetInt64", parameter.Name)
		case "int32":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetInt32", parameter.Name)
		case "int16":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetInt16", parameter.Name)
		case "int8":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetInt8", parameter.Name)
		case "uint":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetInt", parameter.Name)
		case "uint64":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetUint64", parameter.Name)
		case "uint32":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetUint32", parameter.Name)
		case "uint16":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetUint16", parameter.Name)
		case "uint8":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetUint8", parameter.Name)
		case "bool":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetBool", parameter.Name)
		case "float64":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetFloat", parameter.Name)
		case "float32":
			parameterRules += fmt.Sprintf("%s, _ := v.%s(\"%s\")\n", param, "GetFloat", parameter.Name)
		case "string":
			parameterRules += fmt.Sprintf("%s := v.%s(\"%s\")\n", param, "GetString", parameter.Name)
		default:
			fmt.Println(parameter.Type + ":" + param)
		}
		parameterRules += fmt.Sprintf("valid.Required(%s, \"%s\")\n", param, parameter.Name)
	}

	methodList += fmt.Sprintf(`
            func (v %sValid) %s(input *context.BeegoInput) {
                valid := validation.Validation{}
                v.Input = input
                %s
                v.ErrorHandle(valid)
            }`, module, funcName, parameterRules)

	return methodList
}

func openFile(fpath string) (f *os.File, err error) {
	if bu.IsExist(fpath) {
		beeLogger.Log.Warnf("'%s' already exists. Do you want to overwrite it? [Yes|No] ", fpath)
		if bu.AskForConfirmation() {
			f, err = os.OpenFile(fpath, os.O_RDWR|os.O_TRUNC, 0666)
			if err != nil {
				beeLogger.Log.Warnf("%s", err)
				return f, err
			}
		} else {
			beeLogger.Log.Warnf("Skipped create file '%s'", fpath)
			return f, errors.New(fmt.Sprintf("Skipped create file '%s'", fpath))
		}
	} else {
		f, err = os.OpenFile(fpath, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			beeLogger.Log.Warnf("%s", err)
			return f, err
		}
	}
	return f, err
}

// analyseNewNamespace returns version and the others params
func analyseNewNamespace(ce *ast.CallExpr) (first string, others []ast.Expr) {
	for i, p := range ce.Args {
		if i == 0 {
			switch pp := p.(type) {
			case *ast.BasicLit:
				first = strings.Trim(pp.Value, `"`)
			}
			continue
		}
		others = append(others, p)
	}
	return
}

func analyseNSInclude(baseurl string, ce *ast.CallExpr) string {
	cname := ""
	for _, p := range ce.Args {
		var x *ast.SelectorExpr
		var p1 interface{} = p
		if ident, ok := p1.(*ast.Ident); ok {
			if assign, ok := ident.Obj.Decl.(*ast.AssignStmt); ok {
				if len(assign.Rhs) > 0 {
					p1 = assign.Rhs[0].(*ast.UnaryExpr)
				}
			}
		}
		if _, ok := p1.(*ast.UnaryExpr); ok {
			x = p1.(*ast.UnaryExpr).X.(*ast.CompositeLit).Type.(*ast.SelectorExpr)
		} else {
			beeLogger.Log.Warnf("Couldn't determine type\n")
			continue
		}
		if v, ok := importList[fmt.Sprint(x.X)]; ok {
			cname = v + x.Sel.Name
		}
		if apis, ok := controllerList[cname]; ok {
			for rt, item := range apis {
				funcName := funcList[cname][rt]
				if baseurl != "" {
					rt = baseurl + rt
				}
				if len(rootApi.Paths) == 0 {
					rootApi.Paths = make(map[string]*swagger.Item)
				}
				rt = urlReplace(rt)
				rootApi.Paths[rt] = item
				moduleName := bu.CamelCase(strings.TrimSuffix(x.Sel.Name, "Controller"))
				if _, ok := moduleList[rt]; !ok {
					moduleList[rt] = make(map[string]string)
				}
				moduleList[rt]["module"] = moduleName
				moduleList[rt]["funcName"] = funcName
			}
		}
	}
	return cname
}

func analyseControllerPkg(vendorPath, localName, pkgpath string) {
	pkgpath = strings.Trim(pkgpath, "\"")
	if isSystemPackage(pkgpath) {
		return
	}
	if pkgpath == "github.com/astaxie/beego" {
		return
	}
	if localName != "" {
		importList[localName] = pkgpath
	} else {
		pps := strings.Split(pkgpath, "/")
		importList[pps[len(pps)-1]] = pkgpath
	}
	goPaths := bu.GetGOPATHs()
	if len(goPaths) == 0 {
		beeLogger.Log.Fatal("GOPATH environment variable is not set or empty")
	}
	pkgRealpath := ""

	wg, _ := filepath.EvalSymlinks(filepath.Join(vendorPath, pkgpath))
	if utils.FileExists(wg) {
		pkgRealpath = wg
	} else {
		wGoPath := goPaths
		for _, wg := range wGoPath {
			wg, _ = filepath.EvalSymlinks(filepath.Join(wg, "src", pkgpath))
			if utils.FileExists(wg) {
				pkgRealpath = wg
				break
			}
		}
	}

	fileSet := token.NewFileSet()
	astPkgs, err := parser.ParseDir(fileSet, pkgRealpath, func(info os.FileInfo) bool {
		name := info.Name()
		return !info.IsDir() && !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".go")
	}, parser.ParseComments)
	if err != nil {
		beeLogger.Log.Fatalf("Error while parsing dir at '%s': %s", pkgpath, err)
	}
	for _, pkg := range astPkgs {
		for _, fl := range pkg.Files {
			for _, d := range fl.Decls {
				switch specDecl := d.(type) {
				case *ast.FuncDecl:
					if specDecl.Recv != nil && len(specDecl.Recv.List) > 0 {
						if t, ok := specDecl.Recv.List[0].Type.(*ast.StarExpr); ok {
							// Parse controller method
							_ = parserComments(specDecl, fmt.Sprint(t.X), pkgpath)
						}
					}
				}
			}
		}
	}

}

func isSystemPackage(pkgpath string) bool {
	goRoot := os.Getenv("GOROOT")
	if goRoot == "" {
		goRoot = runtime.GOROOT()
	}
	if goRoot == "" {
		beeLogger.Log.Fatalf("GOROOT environment variable is not set or empty")
	}

	wg, _ := filepath.EvalSymlinks(filepath.Join(goRoot, "src", "pkg", pkgpath))
	if utils.FileExists(wg) {
		return true
	}

	// TODO(zh):support go1.4
	wg, _ = filepath.EvalSymlinks(filepath.Join(goRoot, "src", pkgpath))
	return utils.FileExists(wg)
}

// parse the func comments
func parserComments(f *ast.FuncDecl, controllerName, pkgpath string) error {
	var routerPath string
	var HTTPMethod string
	opts := swagger.Operation{
		Responses: make(map[string]swagger.Response),
	}
	funcName := f.Name.String()
	comments := f.Doc
	funcParamMap := buildParamMap(f.Type.Params)
	// TODO: resultMap := buildParamMap(f.Type.Results)
	if comments != nil && comments.List != nil {
		for _, c := range comments.List {
			t := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if strings.HasPrefix(t, "@router") {
				elements := strings.TrimSpace(t[len("@router"):])
				e1 := strings.SplitN(elements, " ", 2)
				if len(e1) < 1 {
					return errors.New("you should has router infomation")
				}
				routerPath = e1[0]
				if len(e1) == 2 && e1[1] != "" {
					e1 = strings.SplitN(e1[1], " ", 2)
					HTTPMethod = strings.ToUpper(strings.Trim(e1[0], "[]"))
				} else {
					HTTPMethod = "GET"
				}
			} else if strings.HasPrefix(t, "@Param") {
				para := swagger.Parameter{}
				p := getparams(strings.TrimSpace(t[len("@Param "):]))
				if len(p) < 4 {
					beeLogger.Log.Fatal(controllerName + "_" + funcName + "'s comments @Param should have at least 4 params")
				}
				paramNames := strings.SplitN(p[0], "=>", 2)
				para.Name = paramNames[0]
				funcParamName := para.Name
				if len(paramNames) > 1 {
					funcParamName = paramNames[1]
				}
				paramType, ok := funcParamMap[funcParamName]
				if ok {
					delete(funcParamMap, funcParamName)
				}

				para.In = p[1]
				pp := strings.Split(p[2], ".")
				typ := pp[len(pp)-1]
				if len(pp) >= 2 {
					if p[1] == "body" && strings.HasPrefix(p[2], "[]") {
						p[2] = p[2][2:]
					}
				} else {
					if typ == "auto" {
						typ = paramType
					}
					setParamType(&para, typ, pkgpath, controllerName)
				}
				switch len(p) {
				case 5:
					para.Required, _ = strconv.ParseBool(p[3])
					para.Description = strings.Trim(p[4], `" `)
				case 6:
					para.Default = str2RealType(p[3], para.Type)
					para.Required, _ = strconv.ParseBool(p[4])
					para.Description = strings.Trim(p[5], `" `)
				default:
					para.Description = strings.Trim(p[3], `" `)
				}
				opts.Parameters = append(opts.Parameters, para)
			}
		}
	}

	if routerPath != "" {
		// Go over function parameters which were not mapped and create swagger params for them
		for name, typ := range funcParamMap {
			para := swagger.Parameter{}
			para.Name = name
			setParamType(&para, typ, pkgpath, controllerName)
			if paramInPath(name, routerPath) {
				para.In = "path"
			} else {
				para.In = "query"
			}
			opts.Parameters = append(opts.Parameters, para)
		}

		var item *swagger.Item
		if itemList, ok := controllerList[pkgpath+controllerName]; ok {
			if it, ok := itemList[routerPath]; !ok {
				item = &swagger.Item{}
			} else {
				item = it
			}
		} else {
			controllerList[pkgpath+controllerName] = make(map[string]*swagger.Item)
			item = &swagger.Item{}
		}
		for _, hm := range strings.Split(HTTPMethod, ",") {
			switch hm {
			case "GET":
				item.Get = &opts
			case "POST":
				item.Post = &opts
			case "PUT":
				item.Put = &opts
			case "PATCH":
				item.Patch = &opts
			case "DELETE":
				item.Delete = &opts
			case "HEAD":
				item.Head = &opts
			case "OPTIONS":
				item.Options = &opts
			}
		}
		controllerList[pkgpath+controllerName][routerPath] = item

		if _, ok := funcList[pkgpath+controllerName]; !ok {
			funcList[pkgpath+controllerName] = make(map[string]string)
		}
		funcList[pkgpath+controllerName][routerPath] = funcName
	}

	return nil
}

func paramInPath(name, route string) bool {
	return strings.HasSuffix(route, ":"+name) ||
		strings.Contains(route, ":"+name+"/")
}

func setParamType(para *swagger.Parameter, typ string, pkgpath, controllerName string) {
	isArray := false
	paraType := ""

	if strings.HasPrefix(typ, "[]") {
		typ = typ[2:]
		isArray = true
	}

	if sType, ok := basicTypes[typ]; ok {
		paraType = sType
	}

	if isArray {
		para.Type = astTypeArray
	} else {
		para.Type = paraType
	}

}

func getFunctionParamType(t ast.Expr) string {
	switch paramType := t.(type) {
	case *ast.Ident:
		return paramType.Name
	// case *ast.Ellipsis:
	// 	result := getFunctionParamType(paramType.Elt)
	// 	result.array = true
	// 	return result
	case *ast.ArrayType:
		return "[]" + getFunctionParamType(paramType.Elt)
	case *ast.StarExpr:
		return getFunctionParamType(paramType.X)
	case *ast.SelectorExpr:
		return getFunctionParamType(paramType.X) + "." + paramType.Sel.Name
	default:
		return ""

	}
}

func buildParamMap(list *ast.FieldList) map[string]string {
	i := 0
	result := map[string]string{}
	if list != nil {
		funcParams := list.List
		for _, fparam := range funcParams {
			param := getFunctionParamType(fparam.Type)
			var paramName string
			if len(fparam.Names) > 0 {
				paramName = fparam.Names[0].Name
			} else {
				paramName = fmt.Sprint(i)
				i++
			}
			result[paramName] = param
		}
	}
	return result
}

// analisys params return []string
// @Param	query		form	 string	true		"The email for login"
// [query form string true "The email for login"]
func getparams(str string) []string {
	var s []rune
	var j int
	var start bool
	var r []string
	var quoted int8
	for _, c := range str {
		if unicode.IsSpace(c) && quoted == 0 {
			if !start {
				continue
			} else {
				start = false
				j++
				r = append(r, string(s))
				s = make([]rune, 0)
				continue
			}
		}

		start = true
		if c == '"' {
			quoted ^= 1
			continue
		}
		s = append(s, c)
	}
	if len(s) > 0 {
		r = append(r, string(s))
	}
	return r
}

func urlReplace(src string) string {
	pt := strings.Split(src, "/")
	for i, p := range pt {
		if len(p) > 0 {
			if p[0] == ':' {
				pt[i] = "{" + p[1:] + "}"
			} else if p[0] == '?' && p[1] == ':' {
				pt[i] = "{" + p[2:] + "}"
			}

			if pt[i][0] == '{' && strings.Contains(pt[i], ":") {
				pt[i] = pt[i][:strings.Index(pt[i], ":")] + "}"
			} else if pt[i][0] == '{' && strings.Contains(pt[i], "(") {
				pt[i] = pt[i][:strings.Index(pt[i], "(")] + "}"
			}
		}
	}
	return strings.Join(pt, "/")
}

func str2RealType(s string, typ string) interface{} {
	var err error
	var ret interface{}

	switch typ {
	case "int", "int64", "int32", "int16", "int8":
		ret, err = strconv.Atoi(s)
	case "uint", "uint64", "uint32", "uint16", "uint8":
		ret, err = strconv.ParseUint(s, 10, 0)
	case "bool":
		ret, err = strconv.ParseBool(s)
	case "float64":
		ret, err = strconv.ParseFloat(s, 64)
	case "float32":
		ret, err = strconv.ParseFloat(s, 32)
	default:
		return s
	}

	if err != nil {
		beeLogger.Log.Warnf("Invalid default value type '%s': %s", typ, s)
		return s
	}

	return ret
}

const VALIDTPL = `
package validator

import (
    "github.com/astaxie/beego/context"
    "github.com/astaxie/beego/validation"
)

type {{moduleName}}Valid struct {
    Validator
}

func New{{moduleName}}Valid() IValidator {
    return &{{moduleName}}Valid{}
}

{{methodList}}

func init() {
    Register({{moduleName}}Validator, New{{moduleName}}Valid)
}
`

const MAPPERTPL = `
package validator

const (
    {{constList}}
)

func init() {
    {{mapList}}
}
`

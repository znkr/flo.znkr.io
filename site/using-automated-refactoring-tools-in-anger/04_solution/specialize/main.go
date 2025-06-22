package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"slices"

	"golang.org/x/tools/go/ast/astutil"
)

func main() {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sort.go", nil, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing input: %v", err)
		os.Exit(1)
	}

	file = astutil.Apply(file, func(c *astutil.Cursor) bool {
		// Declaration of Sort[T any]
		fd, ok := c.Node().(*ast.FuncDecl)
		if !ok || fd.Name.Name != "Sort" {
			return true
		}

		// Rename to SortInt and remove type parameters.
		fd.Name.Name = "SortInt"
		fd.Type.TypeParams = nil

		// Remove `less` from function parameters (SortInt shouldn't have a less parametr)
		fd.Type.Params.List = slices.DeleteFunc(fd.Type.Params.List, func(f *ast.Field) bool {
			if len(f.Names) != 1 {
				return false
			}
			return f.Names[0].Name == "less"
		})

		// Specialize all type parameters in the parameter list.
		for _, param := range fd.Type.Params.List {
			param.Type = astutil.Apply(param.Type, func(c *astutil.Cursor) bool {
				if ident, ok := c.Node().(*ast.Ident); ok && ident.Name == "T" {
					ident.Name = "int"
					return false
				}
				return true
			}, nil).(ast.Expr)
		}

		// Specialize body, by replacing Sort invocation with SortInt and less invocations with `<`.
		fd.Body = astutil.Apply(fd.Body, func(c *astutil.Cursor) bool {
			call, ok := c.Node().(*ast.CallExpr)
			if !ok {
				return true
			}
			fun, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			switch fun.Name {
			case "Sort":
				fun.Name = "SortInt"
				call.Args = slices.DeleteFunc(call.Args, func(arg ast.Expr) bool {
					name, ok := arg.(*ast.Ident)
					return ok && name.Name == "less"
				})
			case "less":
				if len(call.Args) != 2 {
					return true
				}
				c.Replace(&ast.BinaryExpr{
					X:  call.Args[0],
					Op: token.LSS,
					Y:  call.Args[1],
				})
			}
			return true
		}, nil).(*ast.BlockStmt)
		return false
	}, nil).(*ast.File)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		fmt.Fprintf(os.Stderr, "error formatting result: %v", err)
		os.Exit(1)
	}
	if err := os.WriteFile("gen_sort_int.go", buf.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing result: %v", err)
		os.Exit(1)
	}
}

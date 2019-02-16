package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var (
	srcDir    string
	outDir    string
	prefix    string
	tmplFile  string
	printTmpl bool
)

// Flash card output template that works for Quizlet.
var quizletTmpl = `What is pkg {{.Name}}?,{{firstSentence .Doc}};
{{range .Funcs}}{{if isExported .Decl}}What does function {{.Name}} do and what is its declaration?,{{firstSentence .Doc}}

{{funcDeclString .Decl}};
{{end}}{{end}}
{{range .Types}}{{if isExported .Decl}}What is type {{.Name}}?,{{firstSentence .Doc}};
{{range .Methods}}{{if .Decl.Name.IsExported}}What does method {{.Name}} do and what is its declaration?,{{firstSentence .Doc}}

{{funcDeclString .Decl}};
{{end}}{{end}}{{end}}{{end}}
`

func main() {
	flag.StringVar(&srcDir, "src", "", "Path to Go source code")
	flag.StringVar(&outDir, "out", "", "Path to output directory")
	flag.StringVar(&prefix, "prefix", "", "Prefix for output files")
	flag.StringVar(&tmplFile, "tmpl", "", "Path to card template file")
	flag.BoolVar(&printTmpl, "deftmpl", false, "Print default template to stdout and exit")
	flag.Parse()

	// If caller requested the default template, print it and exit.
	if printTmpl {
		fmt.Printf("%s\n", quizletTmpl)
		os.Exit(0)
	}

	// Parse the Go code in srcDir.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, srcDir, filterTests, parser.ParseComments)
	check(err)

	// Create a function that the Go template executor can use to get a
	// string representation from a function declaration.
	funcDeclString := func(f *ast.FuncDecl) (string, error) {
		buf := &bytes.Buffer{}
		if err := printer.Fprint(buf, fset, f); err != nil {
			return "", err
		}
		return buf.String(), nil
	}

	// Helper functions that users can call in templates to extract
	// data from docs and AST.
	funcMap := template.FuncMap{
		"firstSentence":  firstSentence,
		"funcDeclString": funcDeclString,
		"isExported":     isExported,
	}

	// Parse the template that specifies the flash card output format.
	tmpl := template.New("cards").Funcs(funcMap)

	if tmplFile != "" {
		tmpl, err = template.ParseFiles(tmplFile)
	} else {
		tmpl, err = tmpl.Parse(quizletTmpl)
	}
	check(err)

	// If user didn't specify an outDir, create one in /tmp.
	if outDir == "" {
		dir, err := ioutil.TempDir("", "gopkgcards")
		check(err)
		outDir = dir
	}

	// Make sure the outDir exists.
	err = os.MkdirAll(outDir, 0777)
	check(err)

	// Tell the user what's happening.
	fmt.Printf("input: %s\n", srcDir)
	fmt.Printf("output %s\n", outDir)
	fmt.Println("generating...")

	// Iterate through packages the parser found and generate flash
	// cards for each using the output template.
	for name, pkg := range pkgs {
		if pkg.Name == "main" {
			continue
		}
		err := writePkgCards(name, pkg, fset, tmpl, prefix, outDir)
		check(err)
	}

	fmt.Println("done")
}

func writePkgCards(name string, pkg *ast.Package, fset *token.FileSet, tmpl *template.Template, prefix, dir string) error {
	// Each package will have its cards generated in a separate file named
	// <prefix><package-name>.
	fname := fmt.Sprintf("%s%s", prefix, name)
	fname = filepath.Join(dir, fname)

	// Create the output file for this package.
	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	// Get the docs for this package from the AST.
	p := doc.New(pkg, "", doc.AllDecls|doc.AllMethods)

	// Execute the template to write flash cards for this package to
	// the output file.
	if err := tmpl.Execute(f, p); err != nil {
		return err
	}

	return nil
}

func firstSentence(paragraph string) string {
	sentences := strings.Split(paragraph, ".")
	sentence := trim(sentences[0]) + "."
	return sentence
}

func isExported(n ast.Node) bool {
	switch t := n.(type) {
	case *ast.GenDecl:
		switch t.Tok {
		case token.TYPE:
			if len(t.Specs) < 1 {
				return false
			}
			ts, ok := t.Specs[0].(*ast.TypeSpec)
			if !ok {
				return false
			}
			return ts.Name.IsExported()
		default:
			return false
		}
	case *ast.FuncDecl:
		return t.Name.IsExported()
	default:
		return false
	}
}

func filterTests(fi os.FileInfo) bool {
	return !strings.Contains(fi.Name(), "_test")
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Replace(s, "\n", " ", -1)
	return strings.Replace(s, "\t", "", -1)
}

func check(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

package arch_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainNoIOImports(t *testing.T) {
	root := filepath.Join("..", "..", "domain")
	fset := token.NewFileSet()
	forbidden := []string{"os", "net", "io/fs", "syscall"}
	var bad []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, fbd := range forbidden {
				if p == fbd || strings.HasPrefix(p, fbd+"/") {
					bad = append(bad, path+": "+p)
				}
			}
		}
		return nil
	})
	if len(bad) > 0 {
		t.Fatal(strings.Join(bad, "\n"))
	}
}

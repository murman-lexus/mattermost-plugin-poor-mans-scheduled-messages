package logging

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Check every production log call, including paths not reached by runtime tests.
func TestProductionLogsExcludePayloads(t *testing.T) {
	for _, dir := range []string{"server", "adapters"} {
		err := filepath.WalkDir(filepath.Join("..", "..", dir), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_mock.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || !isLogCall(call) {
					return true
				}
				location := fset.Position(call.Pos()).String()
				_, literal := call.Args[0].(*ast.BasicLit)
				require.True(t, literal, "%s: log messages must be static", location)
				for i := 1; i+1 < len(call.Args); i += 2 {
					key, ok := call.Args[i].(*ast.BasicLit)
					require.True(t, ok, "%s: log field names must be static", location)
					name, err := strconv.Unquote(key.Value)
					require.NoError(t, err)
					switch name {
					case "command", "command_text", "text", "original_text", "message", "response_text", "context", "url", "action", "reason":
						t.Errorf("%s: raw payload field %q", location, name)
					case "error", "post_error", "dm_error", "original_post_error":
						t.Errorf("%s: raw error field %q", location, name)
					}
					if strings.HasSuffix(name, "_reason") {
						reason, ok := call.Args[i+1].(*ast.CallExpr)
						require.True(t, ok, "%s: error reasons must use logging.ErrorReason", location)
						selector, ok := reason.Fun.(*ast.SelectorExpr)
						require.True(t, ok, location)
						require.Equal(t, "ErrorReason", selector.Sel.Name, location)
					}
				}
				return true
			})
			return nil
		})
		require.NoError(t, err)
	}
}

func isLogCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch selector.Sel.Name {
	case "Debug", "Info", "Warn", "Error", "LogDebug", "LogInfo", "LogWarn", "LogError":
	default:
		return false
	}
	switch receiver := selector.X.(type) {
	case *ast.Ident:
		return receiver.Name == "logger"
	case *ast.SelectorExpr:
		return receiver.Sel.Name == "logger" || receiver.Sel.Name == "API"
	}
	return false
}

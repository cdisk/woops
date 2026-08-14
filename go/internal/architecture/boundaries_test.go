package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/ops-bastion/ops/go"

var legacyPaths = []string{
	"internal/agent/modules",
	"internal/gateway/modules",
	"internal/guac",
	"internal/protocol/portmap",
	"internal/agent/core/netiface",
}

func TestArchitectureBoundaries(t *testing.T) {
	root := moduleRoot(t)
	internalRoot := filepath.Join(root, "internal")

	err := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		source := strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/")

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			checkImport(t, filepath.ToSlash(rel), source, importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoLegacyArchitecturePaths(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		livePath := filepath.ToSlash(rel)
		for _, legacy := range legacyPaths {
			if livePath == legacy || strings.HasPrefix(livePath, legacy+"/") {
				t.Errorf("legacy architecture path remains live: %s", livePath)
			}
		}
		gatewayLeaf := strings.TrimPrefix(livePath, "internal/gateway/")
		if gatewayLeaf != livePath && !strings.Contains(gatewayLeaf, "/") && filepath.Ext(livePath) == ".go" {
			t.Errorf("gateway core Go file remains at package root: %s", livePath)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func checkImport(t *testing.T, sourceFile string, source []string, importPath string) {
	t.Helper()
	internalImport := strings.TrimPrefix(importPath, modulePath+"/")
	for _, legacy := range legacyPaths {
		if internalImport == legacy || strings.HasPrefix(internalImport, legacy+"/") {
			t.Errorf("%s: imports legacy architecture path %s", sourceFile, importPath)
		}
	}

	target := strings.Split(internalImport, "/")
	if len(target) < 3 || target[0] != "internal" {
		return
	}

	if len(source) >= 3 && source[0] == "internal" && source[2] == "core" &&
		source[1] == target[1] && isFeatureLayer(target[2]) {
		t.Errorf("%s: %s/core must not import %s/%s", sourceFile, source[1], target[1], target[2])
	}

	if len(source) < 4 || len(target) < 4 ||
		source[0] != "internal" || source[1] != target[1] ||
		!isFeatureLayer(source[2]) || !isFeatureLayer(target[2]) {
		return
	}
	if source[3] != target[3] {
		t.Errorf("%s: feature %s/%s must not import feature implementation %s/%s",
			sourceFile, source[2], source[3], target[2], target[3])
	}
}

func isFeatureLayer(name string) bool {
	switch name {
	case "sessions", "services", "plugins":
		return true
	default:
		return false
	}
}

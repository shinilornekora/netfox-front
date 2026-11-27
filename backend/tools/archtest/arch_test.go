// Архитектурные правила Плёнки, проверяемые на каждом прогоне тестов:
//  1. сервис не импортирует код другого сервиса — только contracts и kit;
//  2. core (доменная модель) не знает про базу, HTTP, gRPC и Kafka;
//  3. kit не зависит ни от одного сервиса.
package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var services = []string{"identity", "catalog", "billing", "playback", "gateway"}

func imports(t *testing.T, dir string) map[string][]string {
	out := map[string][]string{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range f.Imports {
			out[p] = append(out[p], strings.Trim(i.Path.Value, `"`))
		}
		return nil
	})
	return out
}

func TestServicesAreIsolated(t *testing.T) {
	root := "../.."
	for _, svc := range services {
		for file, imps := range imports(t, filepath.Join(root, "services", svc)) {
			for _, imp := range imps {
				for _, other := range services {
					if other != svc && strings.HasPrefix(imp, "plenka/"+other) {
						t.Errorf("%s imports %s", file, imp)
					}
				}
				if strings.Contains(file, string(os.PathSeparator)+"core"+string(os.PathSeparator)) {
					for _, bad := range []string{"pgx", "net/http", "google.golang.org/grpc", "franz-go", "plenka/kit/pgkit", "plenka/kit/bus", "plenka/kit/web"} {
						if strings.Contains(imp, bad) {
							t.Errorf("domain core %s depends on infrastructure %s", file, imp)
						}
					}
				}
			}
		}
	}
	for file, imps := range imports(t, filepath.Join(root, "kit")) {
		for _, imp := range imps {
			for _, s := range services {
				if strings.HasPrefix(imp, "plenka/"+s) {
					t.Errorf("kit %s imports service %s", file, imp)
				}
			}
		}
	}
}

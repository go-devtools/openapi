package verify

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"testing"
)

// Reject both declared replacements and replacements selected by the actual module graph.
// 拒绝 go.mod 声明的替换与实际模块图选中的替换。
func TestNoLocalReplace(t *testing.T) {
	edit := exec.Command("go", "mod", "edit", "-json")
	edit.Dir = "../.."
	raw, err := edit.Output()
	if err != nil {
		t.Fatal(err)
	}
	var module struct{ Replace []json.RawMessage }
	if err = json.Unmarshal(raw, &module); err != nil {
		t.Fatal(err)
	}
	if len(module.Replace) != 0 {
		t.Fatal("published go.mod must not contain replacements")
	}
	graph := exec.Command("go", "list", "-m", "-json", "all")
	graph.Dir = "../.."
	raw, err = graph.Output()
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		var dependency struct {
			Path    string
			Replace *json.RawMessage
		}
		err := decoder.Decode(&dependency)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if dependency.Replace != nil {
			t.Fatalf("module graph contains a replacement: %s", dependency.Path)
		}
	}
}

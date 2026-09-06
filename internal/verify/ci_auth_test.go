package verify

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Restrict CI credentials to exact repository paths and avoid storing token values.
func TestCICredentialScope(t *testing.T) {
	helper := filepath.Join("..", "..", "scripts", "git-credential-ci.sh")
	for _, sample := range []struct {
		name, action, protocol, host, path, repository, own, core, want string
	}{
		{"own core", "get", "https", "github.com", "openapi-golang/openapi", "openapi-golang/openapi", "own-fake", "", "own-fake"},
		{"own adapter", "get", "https", "github.com", "openapi-golang/gin-swagger.git", "openapi-golang/gin-swagger", "own-fake", "core-fake", "own-fake"},
		{"private core", "get", "https", "github.com", "openapi-golang/openapi.git", "openapi-golang/gin-swagger", "own-fake", "core-fake", "core-fake"},
		{"no cross token", "get", "https", "github.com", "openapi-golang/openapi", "openapi-golang/gin-swagger", "own-fake", "", ""},
		{"other host", "get", "https", "github.com.evil.test", "openapi-golang/openapi", "openapi-golang/gin-swagger", "own-fake", "core-fake", ""},
		{"http", "get", "http", "github.com", "openapi-golang/openapi", "openapi-golang/gin-swagger", "own-fake", "core-fake", ""},
		{"path suffix", "get", "https", "github.com", "openapi-golang/openapi-other", "openapi-golang/gin-swagger", "own-fake", "core-fake", ""},
		{"nested path", "get", "https", "github.com", "openapi-golang/openapi/other", "openapi-golang/gin-swagger", "own-fake", "core-fake", ""},
		{"unrelated owner", "get", "https", "github.com", "other/openapi", "openapi-golang/gin-swagger", "own-fake", "core-fake", ""},
		{"reverse dependency", "get", "https", "github.com", "openapi-golang/gin-swagger", "openapi-golang/openapi", "own-fake", "core-fake", ""},
		{"store ignored", "store", "https", "github.com", "openapi-golang/openapi", "openapi-golang/gin-swagger", "own-fake", "core-fake", ""},
		{"erase ignored", "erase", "https", "github.com", "openapi-golang/openapi", "openapi-golang/gin-swagger", "own-fake", "core-fake", ""},
	} {
		t.Run(sample.name, func(t *testing.T) {
			cmd := exec.Command("bash", helper, sample.action)
			cmd.Env = append(os.Environ(), "CI_REPOSITORY="+sample.repository, "CI_REPOSITORY_TOKEN="+sample.own, "OPENAPI_READ_TOKEN="+sample.core)
			cmd.Stdin = strings.NewReader("protocol=" + sample.protocol + "\nhost=" + sample.host + "\npath=" + sample.path + "\n\n")
			raw, err := cmd.Output()
			if err != nil {
				t.Fatalf("credential helper failed: %v", err)
			}
			want := ""
			if sample.want != "" {
				want = "username=x-access-token\npassword=" + sample.want + "\n\n"
			}
			if string(raw) != want {
				t.Fatal("credential response exceeded or lost its expected scope")
			}
		})
	}
}

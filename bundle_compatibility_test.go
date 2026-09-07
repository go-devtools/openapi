package openapi_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/contracttest"
)

// Read authentic older writer output and preserve its independently validated wire contract.
func TestPublishedBundleCompatibility(t *testing.T) {
	for _, fixture := range []struct{ name, checksum string }{
		{"writer-72140a9.bundle.json", "9899cf64e97d2bcd4cc414cbda8bdc6514593605d2fd188aa0537a473bd8aba3"},
		{"writer-3654588.bundle.json", "06e40fb5bc47d90e0fc4f6d9bbe261abbdd4b709998ffeaee2b29a64535a9404"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "compatibility", fixture.name))
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%x", sha256.Sum256(raw)) != fixture.checksum {
				t.Fatal("archived writer output changed")
			}
			bundle, err := openapi.ParseBundle(raw)
			if err != nil {
				t.Fatal(err)
			}
			index := bundle.Index()
			if len(index) != 1 || index[0].Symbol != "example.test/bundle-compatibility/api.Create" {
				t.Fatalf("lost the source template: %+v", index)
			}
			document, err := openapi.Build(bundle, []openapi.Route{{Method: "POST", Path: "/users", OperationKey: index[0].Key}}, openapi.Config{Title: "Compatible reader", Version: "1"})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(document.JSON(), []byte("Create a user")) {
				t.Fatal("source summary was lost")
			}
			for _, sample := range []struct{ pointer, valid, invalid string }{
				{"/paths/~1users/post/requestBody/content/application~1json/schema", `{"Name":"alice"}`, `{"Name":"a"}`},
				{"/paths/~1users/post/responses/201/content/application~1json/schema", `{"ID":1024,"Name":"alice"}`, `{"ID":"incorrect","Name":"alice"}`},
			} {
				validator, err := contracttest.Compile(document.JSON(), sample.pointer, contracttest.Options{})
				if err != nil {
					t.Fatal(err)
				}
				if err = validator.JSON([]byte(sample.valid)); err != nil {
					t.Fatal(err)
				}
				if validator.JSON([]byte(sample.invalid)) == nil {
					t.Fatal("writer constraints were lost")
				}
			}
			// Reader acceptance depends on format and capabilities, not equality of informational writer strings.
			data := bundle.Snapshot()
			data.Profile.Generator = "other-compatible-writer"
			data.Profile.Frontend = "other-compatible-frontend"
			changed, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			alternate, err := openapi.ParseBundle(changed)
			if err != nil {
				t.Fatal(err)
			}
			rebuilt, err := openapi.Build(alternate, []openapi.Route{{Method: "POST", Path: "/users", OperationKey: index[0].Key}}, openapi.Config{Title: "Compatible reader", Version: "1"})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(document.JSON(), rebuilt.JSON()) {
				t.Fatal("informational writer identity changed the contract")
			}
		})
	}
}

// Fail closed for incompatible serialized formats, capabilities and unrecognized protocol fields.
func TestSerializedBundleCompatibilityRejection(t *testing.T) {
	raw, err := os.ReadFile("testdata/compatibility/writer-3654588.bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"future-format", "wrong-specification", "required-capability", "unknown-field"} {
		t.Run(name, func(t *testing.T) {
			var data map[string]json.RawMessage
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "future-format":
				data["formatVersion"] = json.RawMessage(`2`)
			case "wrong-specification":
				data["specVersion"] = json.RawMessage(`"3.1.0"`)
			case "required-capability":
				data["capabilities"] = json.RawMessage(`["oas32","future-required"]`)
			case "unknown-field":
				data["futureRequiredBehavior"] = json.RawMessage(`true`)
			}
			changed, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := openapi.ParseBundle(changed); err == nil {
				t.Fatal("incompatible serialized contract was accepted")
			}
			if err := openapi.GeneratedBundle(string(changed)).Validate(); err == nil {
				t.Fatal("generated factory hid incompatibility")
			}
		})
	}
}

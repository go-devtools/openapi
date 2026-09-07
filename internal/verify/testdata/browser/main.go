// Serve isolated local documentation for browser contract tests.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/swaggerui"
)

// Keep the schema-first fixture limited to browser verification of shared resources.
const documentJSON = `{
  "openapi": "3.2.0",
  "info": {
    "title": "Browser API",
    "version": "1"
  },
  "servers": [
    {
      "url": "/"
    }
  ],
  "security": [
    {
      "BearerAuth": []
    }
  ],
  "paths": {
    "/submit": {
      "post": {
        "tags": [
          "Browser"
        ],
        "summary": "Submit a role",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/Request_opaque"
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Accepted",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Request_opaque"
                }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "securitySchemes": {
      "BearerAuth": {
        "type": "http",
        "scheme": "bearer"
      }
    },
    "schemas": {
      "Request_opaque": {
        "title": "Request",
        "type": "object",
        "required": [
          "Role"
        ],
        "properties": {
          "Role": {
            "type": "string",
            "enum": [
              "admin",
              "editor"
            ],
            "x-enum-descriptions": [
              "Administrator",
              "Editor"
            ],
            "examples": [
              "admin"
            ]
          }
        }
      }
    }
  }
}`

// Keep native Example fields intact in the served document.
//
//go:embed native-examples.json
var nativeDocumentJSON string

// Start an owned ephemeral listener and stop it when the test process requests shutdown.
func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	if report := openapi.Check([]byte(documentJSON)); report.HasErrors() {
		panic(report)
	}
	if report := openapi.Check([]byte(nativeDocumentJSON)); report.HasErrors() {
		panic(report)
	}
	var guard sync.Mutex
	state := map[string]any{"requests": 0, "authorization": ""}
	mux := http.NewServeMux()
	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		guard.Lock()
		state["requests"] = state["requests"].(int) + 1
		state["authorization"] = r.Header.Get("Authorization")
		guard.Unlock()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Invalid JSON", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	// Echo raw bytes so tests can detect altered serialization and rounded numbers.
	mux.HandleFunc("/native-submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "Invalid body", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"body": string(body), "contentType": r.Header.Get("Content-Type")})
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		guard.Lock()
		defer guard.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
	})
	for _, mode := range []string{"safe", "enabled", "native", "native-enabled"} {
		cfg := swaggerui.Config{Title: "Browser contract", DocExpansion: "full", Definitions: []swaggerui.Definition{{Name: "All endpoints", URL: "./openapi.json"}, {Name: "Reference", URL: "./reference.json"}}}
		if strings.HasSuffix(mode, "enabled") {
			cfg.SubmitMethods = []string{"post"}
		}
		ui, err := swaggerui.New(cfg)
		if err != nil {
			panic(err)
		}
		prefix := "/" + mode + "/docs/"
		mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
			name := strings.TrimPrefix(r.URL.Path, prefix)
			if name == "openapi.json" || name == "reference.json" {
				data := documentJSON
				if strings.HasPrefix(mode, "native") {
					data = nativeDocumentJSON
				}
				if name == "reference.json" {
					data = strings.Replace(data, "Browser API", "Reference API", 1)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(data))
				return
			}
			if name == "" {
				name = "index.html"
			}
			resource, err := ui.Resource(name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			for key, value := range resource.Headers() {
				w.Header().Set(key, value)
			}
			_, _ = w.Write(resource.Bytes())
		})
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("{\"url\":\"http://%s\"}\n", listener.Addr())
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}

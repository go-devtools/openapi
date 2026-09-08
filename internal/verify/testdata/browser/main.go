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

// Keep QUERY, case-sensitive extension methods, and tag metadata intact for browser verification.
//
//go:embed native-methods.json
var methodsDocumentJSON string

// Keep whole-query parameters and stream item contracts native for browser verification.
//
//go:embed native-wire.json
var wireDocumentJSON string

// Keep native security metadata and device endpoints intact for browser verification.
//
//go:embed native-security.json
var securityDocumentJSON string

// Keep parameter, header, form and native encoding fixtures valid before browser rendering.
//
//go:embed native-projection.json
var projectionDocumentJSON string

// Keep schema metadata intact while verifying the shared viewer.
//
//go:embed native-schema-metadata.json
var metadataDocumentJSON string

// Keep the incremental stream contract valid before testing browser buffering.
//
//go:embed native-incremental.json
var incrementalDocumentJSON string

// Keep advanced relationship contracts valid before testing browser display.
//
//go:embed native-relationships.json
var relationshipsDocumentJSON string

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
	if report := openapi.Check([]byte(methodsDocumentJSON)); report.HasErrors() {
		panic(report)
	}
	if report := openapi.Check([]byte(wireDocumentJSON)); report.HasErrors() {
		panic(report)
	}
	if report := openapi.Check([]byte(securityDocumentJSON)); report.HasErrors() {
		panic(report)
	}
	if report := openapi.Check([]byte(projectionDocumentJSON)); report.HasErrors() {
		panic(report)
	}
	if report := openapi.Check([]byte(metadataDocumentJSON)); report.HasErrors() {
		panic(report)
	}
	if report := openapi.Check([]byte(incrementalDocumentJSON)); report.HasErrors() {
		panic(report)
	}
	if report := openapi.Check([]byte(relationshipsDocumentJSON)); report.HasErrors() {
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
	// Echo the received method and bytes without performing any business action.
	mux.HandleFunc("/query-submit", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "Invalid body", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"method": r.Method, "body": string(body)})
	})
	// Return the actual query bytes without deriving a value from parsed parameters.
	mux.HandleFunc("/querystring-submit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"query": r.URL.RawQuery})
	})
	// Echo an explicitly submitted finite stream without re-encoding its records.
	mux.HandleFunc("/stream-submit", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "Invalid body", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"body": string(body), "contentType": r.Header.Get("Content-Type")})
	})
	// Return finite NDJSON and SSE samples using their actual wire framing.
	mux.HandleFunc("/native-stream", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: update\ndata: {\"message\":\"first\"}\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"message\":\"first\"}\n{\"message\":\"second\"}\n"))
	})
	// Hold an actual response open after its first flushed item until the browser test releases it.
	var streamRelease chan struct{}
	mux.HandleFunc("/incremental-stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gate := make(chan struct{})
		guard.Lock()
		if streamRelease != nil {
			guard.Unlock()
			w.WriteHeader(http.StatusConflict)
			return
		}
		streamRelease = gate
		guard.Unlock()
		defer func() {
			guard.Lock()
			if streamRelease == gate {
				streamRelease = nil
			}
			guard.Unlock()
		}()
		first, second := "{\"message\":\"flushed-first\"}\n", "{\"message\":\"released-second\"}\n"
		media := "application/x-ndjson"
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			media = "text/event-stream"
			first, second = "event: first\ndata: flushed-first\n\n", "event: second\ndata: released-second\n\n"
		}
		w.Header().Set("Content-Type", media)
		_, _ = io.WriteString(w, first)
		w.(http.Flusher).Flush()
		select {
		case <-gate:
			_, _ = io.WriteString(w, second)
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
	})
	// Release only the fixture's pending stream; repeated or premature release has an observable failure.
	mux.HandleFunc("/incremental-release", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		guard.Lock()
		gate := streamRelease
		streamRelease = nil
		guard.Unlock()
		if gate == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		close(gate)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		guard.Lock()
		defer guard.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
	})
	for _, mode := range []string{"safe", "enabled", "native", "native-enabled", "methods", "methods-enabled", "wire", "wire-enabled", "security", "security-enabled"} {
		cfg := swaggerui.Config{Title: "Browser contract", DocExpansion: "full", Definitions: []swaggerui.Definition{{Name: "All endpoints", URL: "./openapi.json"}, {Name: "Reference", URL: "./reference.json"}}}
		if strings.HasSuffix(mode, "enabled") {
			cfg.SubmitMethods = []string{"post"}
		}
		if mode == "methods-enabled" {
			cfg.SubmitMethods = []string{"query"}
		}
		if mode == "wire-enabled" {
			cfg.SubmitMethods = []string{"get", "post"}
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
				if strings.HasPrefix(mode, "methods") {
					data = methodsDocumentJSON
				}
				if strings.HasPrefix(mode, "wire") {
					data = wireDocumentJSON
				}
				if strings.HasPrefix(mode, "security") {
					data = securityDocumentJSON
				}
				if name == "reference.json" {
					if strings.HasPrefix(mode, "methods") || strings.HasPrefix(mode, "security") {
						data = documentJSON
					}
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

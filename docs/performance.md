# Performance measurement

Run the source-generation, startup-linking, and document-read benchmarks from the module root with Go 1.27.1:

```sh
GOWORK=off go mod download
GOWORK=off go test ./compiler -run '^$' -bench '^BenchmarkScale$' -benchtime=500ms -count=3 -benchmem
```

Dependencies must already be available to the configured module cache. Benchmark fixture loading sets `GOWORK=off` and `GOPROXY=off`; it does not silently measure dependency downloads. Record the actual toolchain, platform, CPU, module revision, cache state, command, and all repeated samples with any published results.

| Stage | Work inside the measured loop |
| --- | --- |
| `Generate` | Load and analyze real Go source through `compiler.Compile`, project Schema, serialize the Bundle, format generated Go, and atomically write it through `Result.Write`. |
| `Build` | Link a precompiled Bundle and explicit neutral routes through `openapi.Build`, including document validation and serialization. |
| `Read` | Return a defensive copy of the cached JSON through `Document.JSON`. |

Each scale has 100 or 1000 distinct tag-free functions, a required string request field with a minimum length, and a typed JSON response. Setup checks both the template count and final path count. The benchmark frontend uses only the public compiler SDK and has no framework dependency. Temporary source construction, route construction, and baseline validation are outside the timed loops.

Generation measures the library pipeline. It excludes CLI process startup and compilation of the generated consumer application. `Build` measures startup work, with the default core runtime-profile policy; it does not execute application handlers. `Read` includes one owned snapshot copy. It does not time HTTP transport or the Swagger UI. Ordinary business requests are outside all of these benchmarks.

`B/op` and `allocs/op` are cumulative Go allocations in the benchmark process. They are not retained heap size, peak RSS, or memory allocated by the external Go tools that package loading may start. Repeated runs normally use warm build and filesystem caches. Cold-cache resource usage requires a separate experiment. These benchmarks do not establish a universal throughput guarantee or a zero-cost request-path claim.

The Gin adapter has paired core and adapter build benchmarks over the same actual route snapshot. Use that pair to investigate linking costs; do not subtract this neutral workload from an unrelated Gin workload.

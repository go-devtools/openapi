// Exercise parameterized summaries against actual neutral HTTP response behavior.
package helpersummary

import (
	"encoding/json"
	"net/http"
)

// Preserve field contracts through generic helpers and cached effects.
type Item struct {
	// A declared display-name constraint.
	// @openapi minLength=3
	Name string
}

// Wrap concrete generic data without adding DTO tags.
type Envelope[T any] struct{ Data T }

// Provide real response status, headers, and bytes independently of the compiler model.
type Channel struct {
	writer    http.ResponseWriter
	pending   int
	committed bool
	Method    string
}

// Initialize a neutral response carrier with the actual request method.
func NewChannel(writer http.ResponseWriter, method string) *Channel {
	return &Channel{writer: writer, pending: 200, Method: method}
}

// Replace or remove the current header value.
func (c *Channel) Header(name, value string) {
	if value == "" {
		c.writer.Header().Del(name)
	} else {
		c.writer.Header().Set(name, value)
	}
}

// Keep a pending status until the response is committed.
func (c *Channel) Status(code int) {
	if !c.committed {
		c.pending = code
	}
}

// Commit only the first final response status.
func (c *Channel) Commit(code int) {
	if !c.committed {
		c.committed = true
		c.pending = code
		c.writer.WriteHeader(code)
	}
}

// Write real JSON bytes after respecting the existing commit.
func (c *Channel) Send(code int, value any) {
	c.Header("Content-Type", "application/json")
	if code == -1 {
		code = c.pending
	}
	c.Commit(code)
	if err := json.NewEncoder(c.writer).Encode(value); err != nil {
		panic(err)
	}
}

// Provide an effect-free observation point for compiler invocation counts.
func (c *Channel) Trace() {}

// Provide an actual method-dependent branch for correlated frontend outcomes.
func (c *Channel) IsGet() bool { return c.Method == "GET" }

// Forward both status constants and concrete payload instantiations.
func deliver[T any](c *Channel, code int, value T) { c.Send(code, Envelope[T]{Data: value}) }

// Return a constant after a frontend observation call.
func choose(c *Channel, accepted bool) int {
	c.Trace()
	if accepted {
		return 201
	}
	return 202
}

// Retain committed and subsequently observed headers as distinct state.
func committed(c *Channel) error {
	c.Header("Content-Type", "application/json")
	c.Header("X-State", "before")
	c.Commit(409)
	c.Header("X-State", "after")
	return nil
}

// Use the current pending response state inside the helper.
func pending(c *Channel) { c.Send(-1, Item{Name: "Ada"}) }

// Preserve mutable caller cells through the ordinary source-analysis fallback.
func increment(c *Channel, code *int) { c.Trace(); *code = *code + 1 }

// Return independent mutable closures rather than cached capture identities.
func counter(c *Channel) func() int {
	code := 200
	return func() int { c.Trace(); code = code + 1; return code }
}

// Preserve request-condition and response correlation across a summary replay.
func conditional(c *Channel) {
	if c.IsGet() {
		c.Send(201, Item{Name: "Ada"})
	} else {
		c.Send(202, Item{Name: "Ada"})
	}
}

// Two equivalent branch contexts should share one complete effect summary.
func HReplay(c *Channel, flag bool) {
	if flag {
		deliver(c, 201, Item{Name: "Ada"})
	} else {
		deliver(c, 201, Item{Name: "Ada"})
	}
}

// Different type and constant parameters require different contexts.
func HArguments(c *Channel, flag bool) {
	if flag {
		deliver(c, 201, Item{Name: "Ada"})
	} else {
		deliver(c, 202, "ready")
	}
}

// Return facts from a cached helper remain available to its caller.
func HReturn(c *Channel, flag bool) {
	code := 200
	if flag {
		code = choose(c, true)
	} else {
		code = choose(c, true)
	}
	c.Send(code, Item{Name: "Ada"})
}

// A replay must restore the commit and observed-header transition together.
func HCommit(c *Channel, flag bool) {
	if flag {
		committed(c)
	} else {
		committed(c)
	}
	c.Send(201, Item{Name: "Ada"})
}

// Identical arguments under distinct response states cannot share a summary.
func HState(c *Channel, flag bool) {
	if flag {
		c.Status(202)
	} else {
		c.Status(203)
	}
	pending(c)
}

// Consecutive bodies must remain a diagnostic, even with identical helper arguments.
func HSequence(c *Channel, flag bool) {
	deliver(c, 201, Item{Name: "Ada"})
	deliver(c, 201, Item{Name: "Ada"})
}

// Caller pointer mutations must not be replaced by an immutable summary.
func HMutation(c *Channel, flag bool) {
	code := 200
	increment(c, &code)
	increment(c, &code)
	c.Send(code, Item{Name: "Ada"})
}

// Factory calls must allocate independent closure state.
func HFactory(c *Channel, flag bool) {
	first := counter(c)
	second := counter(c)
	c.Send(first()+second(), Item{Name: "Ada"})
}

// Cached alternatives must retain their finite request domains.
func HCondition(c *Channel, flag bool) {
	if flag {
		conditional(c)
	} else {
		conditional(c)
	}
}

// Record a nested helper's actual call and depth footprint.
func leaf(c *Channel) int { c.Trace(); return 201 }

// Provide a reusable two-frame summary.
func middle(c *Channel) int { return leaf(c) }

// Reach the same summary from a deeper call site.
func deeper(c *Channel) int { return middle(c) }

// A cached shallow invocation must not bypass a later deeper invocation's depth limit.
func HDepth(c *Channel, flag bool) {
	if flag {
		c.Send(middle(c), Item{Name: "Ada"})
	} else {
		c.Send(deeper(c), Item{Name: "Ada"})
	}
}

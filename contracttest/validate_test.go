package contracttest

import (
	"strings"
	"testing"
)

// Test precision, recursion, boolean Schemas, and offline reference boundaries independently.
func TestIndependentSchema(t *testing.T) {
	v, err := Compile([]byte(`{"$defs":{"node":{"type":"object","properties":{"next":{"anyOf":[{"$ref":"#/$defs/node"},{"type":"null"}]}},"required":["next"]}},"allOf":[{"$ref":"#/$defs/node"}],"properties":{"id":{"const":9007199254740993}},"required":["id"]}`), "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`{"id":9007199254740993,"next":null}`)); err != nil {
		t.Fatal(err)
	}
	if v.JSON([]byte(`{"id":9007199254740992,"next":null}`)) == nil {
		t.Fatal("large integer was rounded")
	}
	if _, err = Compile([]byte(`{"$ref":"file:///etc/passwd"}`), "", Options{}); err == nil {
		t.Fatal("external reference was not rejected")
	}
	deny, err := Compile([]byte(`false`), "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if deny.JSON([]byte(`null`)) == nil {
		t.Fatal("false did not reject the instance")
	}
	if v.JSON([]byte(`{} {}`)) == nil {
		t.Fatal("multiple JSON values were accepted")
	}
}

// Compile a Schema pointer within a complete document while preserving component references.
func TestDocumentPointer(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","components":{"schemas":{"Name":{"type":"string","minLength":3}}},"paths":{"/users":{"post":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Name"}}}}}}}}}`)
	v, err := Compile(raw, "/paths/~1users/post/responses/200/content/application~1json/schema", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"User A"`)); err != nil {
		t.Fatal(err)
	}
	if v.JSON([]byte(`"ab"`)) == nil {
		t.Fatal("Unicode character count was not checked")
	}
}

// Validate NDJSON items separately instead of treating the stream as one JSON array.
func TestNDJSON(t *testing.T) {
	v, err := Compile([]byte(`{"type":"integer"}`), "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.NDJSON(strings.NewReader("1\n2\r\n3"), Limits{}); err != nil {
		t.Fatal(err)
	}
	if v.NDJSON(strings.NewReader("1\n\"bad\"\n"), Limits{}) == nil {
		t.Fatal("invalid entry was omitted")
	}
	if v.NDJSON(strings.NewReader("1\n2\n"), Limits{MaxItems: 1}) == nil {
		t.Fatal("record count budget is missing")
	}
}

// Test SSE data joining, integer retry values, comments, and ignored fields.
func TestSSE(t *testing.T) {
	input := "\ufeff: comment\r\nevent: update\rdata: first\ndata: second\r\nid: 42\nretry: 1200\nunknown: ignored\n\ndata: {}\nretry: -1\nid: bad\x00id\n\ndata: unfinished"
	events, err := ParseSSE(strings.NewReader(input), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0]["data"] != "first\nsecond" || events[0]["event"] != "update" || events[0]["id"] != "42" {
		t.Fatalf("incorrect event: %#v", events)
	}
	if _, ok := events[1]["retry"]; ok {
		t.Fatal("invalid retry was not ignored")
	}
	if _, ok := events[1]["id"]; ok {
		t.Fatal("id containing NUL was not ignored")
	}
	v, err := Compile([]byte(`{"type":"object","required":["data"],"properties":{"data":{"type":"string"},"retry":{"type":"integer","minimum":0}},"additionalProperties":true}`), "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.SSE(strings.NewReader(input), Limits{}); err != nil {
		t.Fatal(err)
	}
	if _, err = ParseSSE(strings.NewReader("data: too long\n\n"), Limits{MaxBytes: 3}); err == nil {
		t.Fatal("byte budget is missing")
	}
}

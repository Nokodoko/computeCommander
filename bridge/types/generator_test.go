package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testGoSource = `package example

import "encoding/json"

// ExampleStruct is a test type.
// bridge:export
type ExampleStruct struct {
	Name    string          ` + "`" + `json:"name"` + "`" + `
	Count   int             ` + "`" + `json:"count"` + "`" + `
	Active  bool            ` + "`" + `json:"active"` + "`" + `
	Payload json.RawMessage ` + "`" + `json:"payload"` + "`" + `
	Tag     string          ` + "`" + `json:"tag,omitempty"` + "`" + `
	Items   []string        ` + "`" + `json:"items"` + "`" + `
}

// NotExported has no bridge:export marker.
type NotExported struct {
	Foo string
}
`

func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path
}

func TestParseFile_FindsExportedStructs(t *testing.T) {
	path := writeTestFile(t, testGoSource)

	structs, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(structs) != 1 {
		t.Fatalf("got %d structs, want 1", len(structs))
	}

	s := structs[0]
	if s.Name != "ExampleStruct" {
		t.Errorf("name = %q, want %q", s.Name, "ExampleStruct")
	}
	if len(s.Fields) != 6 {
		t.Fatalf("fields = %d, want 6", len(s.Fields))
	}
}

func TestParseFile_FieldTypes(t *testing.T) {
	path := writeTestFile(t, testGoSource)
	structs, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	fields := structs[0].Fields
	tests := []struct {
		jsonName string
		tsType   string
		optional bool
	}{
		{"name", "string", false},
		{"count", "number", false},
		{"active", "boolean", false},
		{"payload", "unknown", false},
		{"tag", "string", true},
		{"items", "string[]", false},
	}

	for i, tt := range tests {
		f := fields[i]
		if f.JSONName != tt.jsonName {
			t.Errorf("field[%d] jsonName = %q, want %q", i, f.JSONName, tt.jsonName)
		}
		if f.TSType != tt.tsType {
			t.Errorf("field[%d] tsType = %q, want %q", i, f.TSType, tt.tsType)
		}
		if f.Optional != tt.optional {
			t.Errorf("field[%d] optional = %v, want %v", i, f.Optional, tt.optional)
		}
	}
}

func TestParseFile_SkipsNonExported(t *testing.T) {
	src := `package example

type Internal struct {
	X int
}
`
	path := writeTestFile(t, src)
	structs, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(structs) != 0 {
		t.Errorf("got %d structs, want 0 (no bridge:export)", len(structs))
	}
}

func TestParseFile_PointerFields(t *testing.T) {
	src := `package example

// bridge:export
type WithPointer struct {
	Label *string ` + "`" + `json:"label"` + "`" + `
}
`
	path := writeTestFile(t, src)
	structs, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(structs) != 1 {
		t.Fatalf("got %d structs, want 1", len(structs))
	}

	f := structs[0].Fields[0]
	if !f.Optional {
		t.Error("pointer field should be optional")
	}
	if !strings.Contains(f.TSType, "undefined") {
		t.Errorf("pointer field TSType = %q, want to contain 'undefined'", f.TSType)
	}
}

func TestParseFile_MapFields(t *testing.T) {
	src := `package example

// bridge:export
type WithMap struct {
	Meta map[string]int ` + "`" + `json:"meta"` + "`" + `
}
`
	path := writeTestFile(t, src)
	structs, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	f := structs[0].Fields[0]
	if f.TSType != "Record<string, number>" {
		t.Errorf("map TSType = %q, want %q", f.TSType, "Record<string, number>")
	}
}

func TestParseFile_SkipsJSONDash(t *testing.T) {
	src := `package example

// bridge:export
type WithHidden struct {
	Visible string ` + "`" + `json:"visible"` + "`" + `
	Hidden  string ` + "`" + `json:"-"` + "`" + `
}
`
	path := writeTestFile(t, src)
	structs, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(structs[0].Fields) != 1 {
		t.Errorf("fields = %d, want 1 (json:\"-\" should be skipped)", len(structs[0].Fields))
	}
}

func TestGenerateTypeScript_Output(t *testing.T) {
	structs := []StructInfo{
		{
			Name: "TestType",
			Doc:  "A test type",
			Fields: []FieldInfo{
				{JSONName: "id", TSType: "number", Optional: false},
				{JSONName: "name", TSType: "string", Optional: true},
			},
		},
	}

	output := GenerateTypeScript(structs)

	if !strings.Contains(output, "DO NOT EDIT") {
		t.Error("missing generated header")
	}
	if !strings.Contains(output, "export interface TestType") {
		t.Error("missing interface declaration")
	}
	if !strings.Contains(output, "id: number;") {
		t.Error("missing id field")
	}
	if !strings.Contains(output, "name?: string;") {
		t.Error("missing optional name field")
	}
	if !strings.Contains(output, "/** A test type */") {
		t.Error("missing doc comment")
	}
}

func TestGenerateToFile_Integration(t *testing.T) {
	goPath := writeTestFile(t, testGoSource)
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "generated.d.ts")

	err := GenerateToFile([]string{goPath}, outPath)
	if err != nil {
		t.Fatalf("GenerateToFile: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "export interface ExampleStruct") {
		t.Error("missing ExampleStruct in output")
	}
}

func TestGenerateToFile_NoExportedStructs(t *testing.T) {
	src := `package example
type Internal struct { X int }
`
	goPath := writeTestFile(t, src)
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "generated.d.ts")

	err := GenerateToFile([]string{goPath}, outPath)
	if err == nil {
		t.Fatal("expected error for no exported structs")
	}
}

func TestParseFile_BadFile(t *testing.T) {
	_, err := ParseFile("/nonexistent/file.go")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

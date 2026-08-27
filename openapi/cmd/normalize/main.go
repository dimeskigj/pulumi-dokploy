package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

type Document struct {
	OpenAPI    string               `json:"openapi"`
	Info       map[string]any       `json:"info"`
	Servers    []any                `json:"servers,omitempty"`
	Paths      map[string]*PathItem `json:"paths"`
	Components *Components          `json:"components,omitempty"`
}
type Components struct {
	Schemas         map[string]json.RawMessage `json:"schemas,omitempty"`
	SecuritySchemes map[string]json.RawMessage `json:"securitySchemes,omitempty"`
}
type PathItem struct {
	Get  *Operation `json:"get,omitempty"`
	Post *Operation `json:"post,omitempty"`
}
type Operation struct {
	OperationID string         `json:"operationId"`
	Raw         map[string]any `json:"-"`
}

func (o *Operation) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &o.Raw); err != nil {
		return err
	}
	o.OperationID, _ = o.Raw["operationId"].(string)
	return nil
}
func (o Operation) MarshalJSON() ([]byte, error) {
	m := o.Raw
	if m == nil {
		m = map[string]any{}
	}
	m["operationId"] = o.OperationID
	return json.Marshal(m)
}
func (p *PathItem) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for k, v := range m {
		if k == "get" {
			var x Operation
			if err := json.Unmarshal(v, &x); err != nil {
				return err
			}
			p.Get = &x
		}
		if k == "post" {
			var x Operation
			if err := json.Unmarshal(v, &x); err != nil {
				return err
			}
			p.Post = &x
		}
	}
	return nil
}

type Corrections struct {
	Responses map[string]string          `json:"responses"`
	Schemas   map[string]json.RawMessage `json:"schemas"`
}

func operationFor(p *PathItem) *Operation {
	if p.Post != nil {
		return p.Post
	}
	return p.Get
}
func schemaRef(name string) map[string]any {
	if name == "boolean" || name == "string" || name == "integer" || name == "number" {
		return map[string]any{"type": name}
	}
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func normalize(in *Document, allow []string, c Corrections) (*Document, error) {
	out := &Document{OpenAPI: "3.1.0", Info: in.Info, Servers: in.Servers, Paths: map[string]*PathItem{}, Components: &Components{Schemas: map[string]json.RawMessage{}, SecuritySchemes: map[string]json.RawMessage{}}}
	for name, raw := range in.Components.SecuritySchemes {
		out.Components.SecuritySchemes[name] = raw
	}
	seen := map[string]string{}
	for _, name := range allow {
		path := "/" + name
		item, ok := in.Paths[path]
		if !ok {
			return nil, fmt.Errorf("allowed operation %s is absent", name)
		}
		op := operationFor(item)
		if op == nil || op.OperationID == "" {
			return nil, fmt.Errorf("allowed operation %s has no operation ID", name)
		}
		if prior, ok := seen[op.OperationID]; ok {
			return nil, fmt.Errorf("duplicate operation ID %s at %s and %s", op.OperationID, prior, path)
		}
		seen[op.OperationID] = path
		b := *item
		if item.Get != nil {
			x := *item.Get
			x.Raw = cloneMap(x.Raw)
			b.Get = &x
		}
		if item.Post != nil {
			x := *item.Post
			x.Raw = cloneMap(x.Raw)
			b.Post = &x
		}
		if schema, ok := c.Responses[name]; ok {
			target := b.Post
			if target == nil {
				target = b.Get
			}
			responses, ok := target.Raw["responses"].(map[string]any)
			if !ok {
				responses = map[string]any{}
				target.Raw["responses"] = responses
			}
			response, ok := responses["200"].(map[string]any)
			if !ok {
				response = map[string]any{}
				responses["200"] = response
			}
			content := map[string]any{"application/json": map[string]any{"schema": schemaRef(schema)}}
			response["content"] = content
			responses["200"] = response
		}
		out.Paths[path] = &b
		if sec, ok := op.Raw["security"].([]any); ok {
			for _, entry := range sec {
				if names, ok := entry.(map[string]any); ok {
					for scheme := range names {
						if raw, exists := in.Components.SecuritySchemes[scheme]; exists {
							out.Components.SecuritySchemes[scheme] = raw
						}
					}
				}
			}
		}
	}
	for n, b := range c.Schemas {
		out.Components.Schemas[n] = b
	}
	refs := map[string]bool{}
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if r, ok := x["$ref"].(string); ok && len(r) > 21 && r[:21] == "#/components/schemas/" {
				refs[r[21:]] = true
			}
			for _, z := range x {
				visit(z)
			}
		case []any:
			for _, z := range x {
				visit(z)
			}
		}
	}
	for _, p := range out.Paths {
		if p.Get != nil {
			visit(p.Get.Raw)
		}
		if p.Post != nil {
			visit(p.Post.Raw)
		}
	}
	for {
		changed := false
		for n := range refs {
			if _, ok := out.Components.Schemas[n]; !ok {
				if b, ok := in.Components.Schemas[n]; ok {
					out.Components.Schemas[n] = b
					changed = true
					visitBytes(b, refs)
				}
			}
		}
		if !changed {
			break
		}
	}
	for n := range out.Components.Schemas {
		if !refs[n] {
			delete(out.Components.Schemas, n)
		}
	}
	return out, nil
}
func visitBytes(b json.RawMessage, refs map[string]bool) {
	var v any
	if json.Unmarshal(b, &v) == nil {
		var f func(any)
		f = func(x any) {
			switch y := x.(type) {
			case map[string]any:
				if r, ok := y["$ref"].(string); ok && len(r) > 21 && r[:21] == "#/components/schemas/" {
					refs[r[21:]] = true
				}
				for _, z := range y {
					f(z)
				}
			case []any:
				for _, z := range y {
					f(z)
				}
			}
		}
		f(v)
	}
}
func cloneMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var x map[string]any
	_ = json.Unmarshal(b, &x)
	return x
}

func main() {
	inPath := flag.String("in", "", "input")
	outPath := flag.String("out", "", "output")
	flag.Parse()
	if *inPath == "" || *outPath == "" {
		panic("-in and -out are required")
	}
	ib, err := os.ReadFile(*inPath)
	if err != nil {
		panic(err)
	}
	var in Document
	if err = json.Unmarshal(ib, &in); err != nil {
		panic(err)
	}
	ab, err := os.ReadFile("openapi/operations.txt")
	if err != nil {
		panic(err)
	}
	var allow []string
	for _, line := range splitLines(string(ab)) {
		if line != "" {
			allow = append(allow, line)
		}
	}
	cb, err := os.ReadFile("openapi/corrections.json")
	if err != nil {
		panic(err)
	}
	var c Corrections
	if err = json.Unmarshal(cb, &c); err != nil {
		panic(err)
	}
	out, err := normalize(&in, allow, c)
	if err != nil {
		panic(err)
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	b = append(b, '\n')
	if err = os.WriteFile(*outPath, b, 0644); err != nil {
		panic(err)
	}
}
func splitLines(s string) []string {
	var r []string
	for len(s) > 0 {
		i := 0
		for i < len(s) && s[i] != '\n' {
			i++
		}
		r = append(r, s[:i])
		if i == len(s) {
			break
		}
		s = s[i+1:]
	}
	return r
}

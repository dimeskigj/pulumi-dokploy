package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
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
	Get     *Operation                 `json:"get,omitempty"`
	Post    *Operation                 `json:"post,omitempty"`
	Methods map[string]*Operation      `json:"-"`
	Raw     map[string]json.RawMessage `json:"-"`
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
	p.Methods = map[string]*Operation{}
	p.Raw = map[string]json.RawMessage{}
	for k, v := range m {
		if isHTTPMethod(k) {
			var x Operation
			if err := json.Unmarshal(v, &x); err != nil {
				return err
			}
			p.Methods[k] = &x
			if k == "get" {
				p.Get = &x
			}
			if k == "post" {
				p.Post = &x
			}
		} else {
			p.Raw[k] = v
		}
	}
	return nil
}
func (p PathItem) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range p.Raw {
		m[k] = v
	}
	methods := p.Methods
	if methods == nil {
		methods = map[string]*Operation{}
	}
	if p.Get != nil {
		methods["get"] = p.Get
	}
	if p.Post != nil {
		methods["post"] = p.Post
	}
	for k, v := range methods {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		m[k] = b
	}
	return json.Marshal(m)
}
func isHTTPMethod(s string) bool {
	switch strings.ToLower(s) {
	case "get", "put", "post", "delete", "options", "head", "patch", "trace":
		return true
	}
	return false
}

type Corrections struct {
	Responses map[string]string          `json:"responses"`
	Schemas   map[string]json.RawMessage `json:"schemas"`
}

func allOperations(p *PathItem) map[string]*Operation {
	r := map[string]*Operation{}
	for method, op := range p.Methods {
		r[method] = op
	}
	if p.Get != nil {
		r["get"] = p.Get
	}
	if p.Post != nil {
		r["post"] = p.Post
	}
	return r
}
func operationFor(p *PathItem, name string) *Operation {
	expected := strings.Replace(name, ".", "-", 1)
	for _, op := range allOperations(p) {
		if op.OperationID == expected {
			return op
		}
	}
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
		op := operationFor(item, name)
		if op == nil || op.OperationID == "" {
			return nil, fmt.Errorf("allowed operation %s has no operation ID", name)
		}
		for method, candidate := range allOperations(item) {
			if candidate.OperationID == "" {
				return nil, fmt.Errorf("allowed operation %s method %s has no operation ID", name, method)
			}
			if prior, ok := seen[candidate.OperationID]; ok {
				return nil, fmt.Errorf("duplicate operation ID %s at %s and %s", candidate.OperationID, prior, path)
			}
			seen[candidate.OperationID] = path
		}
		b := *item
		b.Raw = cloneRawMap(item.Raw)
		b.Methods = map[string]*Operation{}
		for method, candidate := range allOperations(item) {
			x := *candidate
			x.Raw = cloneMap(candidate.Raw)
			b.Methods[method] = &x
			if method == "get" {
				b.Get = &x
			}
			if method == "post" {
				b.Post = &x
			}
		}
		if op = operationFor(&b, name); op == nil {
			return nil, fmt.Errorf("allowed operation %s has no operation", name)
		}
		if schema, ok := c.Responses[name]; ok {
			target := operationFor(&b, name)
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
					if _, exists := names["Authorization"]; exists {
						delete(names, "Authorization")
						names["apiKey"] = []any{}
					}
				}
			}
			bOp := operationFor(&b, name)
			bOp.Raw["security"] = sec
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
	refs := map[string]bool{}
	for n, b := range c.Schemas {
		out.Components.Schemas[n] = b
		refs[n] = true
		visitBytes(b, refs)
	}
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
		for _, op := range allOperations(p) {
			visit(op.Raw)
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
func cloneRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	r := map[string]json.RawMessage{}
	for k, v := range m {
		r[k] = append(json.RawMessage(nil), v...)
	}
	return r
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

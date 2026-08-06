// Command speccheck parses the OpenAPI document and reports any $ref that does
// not resolve, which YAML parsing alone would happily accept.
package main

import (
	"fmt"
	"os"
	"sort"

	yaml "go.yaml.in/yaml/v3"
)

func collectRefs(node any, out map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			if key == "$ref" {
				if s, ok := val.(string); ok {
					out[s] = true
					continue
				}
			}
			collectRefs(val, out)
		}
	case []any:
		for _, item := range v {
			collectRefs(item, out)
		}
	}
}

func keysOf(doc map[string]any, section string) map[string]bool {
	set := map[string]bool{}
	components, _ := doc["components"].(map[string]any)
	group, _ := components[section].(map[string]any)
	for name := range group {
		set[name] = true
	}
	return set
}

func main() {
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("read:", err)
		os.Exit(1)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		fmt.Println("PARSE FAILED:", err)
		os.Exit(1)
	}

	refs := map[string]bool{}
	collectRefs(doc, refs)

	defined := map[string]map[string]bool{
		"schemas":    keysOf(doc, "schemas"),
		"responses":  keysOf(doc, "responses"),
		"parameters": keysOf(doc, "parameters"),
	}

	var dangling []string
	for ref := range refs {
		for section, names := range defined {
			prefix := "#/components/" + section + "/"
			if len(ref) > len(prefix) && ref[:len(prefix)] == prefix {
				if !names[ref[len(prefix):]] {
					dangling = append(dangling, ref)
				}
			}
		}
	}
	sort.Strings(dangling)

	paths, _ := doc["paths"].(map[string]any)
	fmt.Printf("parsed OK: %d paths, %d schemas, %d refs\n",
		len(paths), len(defined["schemas"]), len(refs))
	if len(dangling) == 0 {
		fmt.Println("all $refs resolve")
		return
	}
	fmt.Println("DANGLING REFS:")
	for _, ref := range dangling {
		fmt.Println("  ", ref)
	}
	os.Exit(1)
}

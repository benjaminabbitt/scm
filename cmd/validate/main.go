// validate checks that all fragment YAML files conform to the JSON schema.
// Run before build to catch schema violations early.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "validation failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load the schema
	schemaPath := "resources/context-fragments/standards/fragment-schema.json"
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("fragment.json", strings.NewReader(string(schemaData))); err != nil {
		return fmt.Errorf("failed to add schema resource: %w", err)
	}

	schema, err := compiler.Compile("fragment.json")
	if err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	// Walk resources/context-fragments and validate each YAML file
	fragmentsDir := "resources/context-fragments"
	var errors []string
	var validated int

	err = filepath.WalkDir(fragmentsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Skip non-YAML files
		name := d.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}

		// Skip the schema itself
		if strings.Contains(path, "standards/") {
			return nil
		}

		// Validate this file
		if err := validateFile(schema, path); err != nil {
			errors = append(errors, fmt.Sprintf("  %s: %v", path, err))
		} else {
			validated++
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk fragments: %w", err)
	}

	// Also validate prompts
	promptsDir := "resources/prompts"
	if _, err := os.Stat(promptsDir); err == nil {
		err = filepath.WalkDir(promptsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			name := d.Name()
			if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
				return nil
			}

			if err := validateFile(schema, path); err != nil {
				errors = append(errors, fmt.Sprintf("  %s: %v", path, err))
			} else {
				validated++
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to walk prompts: %w", err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("schema validation errors:\n%s", strings.Join(errors, "\n"))
	}

	fmt.Printf("Validated %d files against schema\n", validated)
	return nil
}

func validateFile(schema *jsonschema.Schema, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	// Parse YAML
	var yamlData interface{}
	if err := yaml.Unmarshal(data, &yamlData); err != nil {
		return fmt.Errorf("YAML parse error: %w", err)
	}

	// Convert to JSON-compatible structure
	jsonData := convertToJSON(yamlData)

	// Validate against schema
	if err := schema.Validate(jsonData); err != nil {
		return err
	}

	return nil
}

// convertToJSON converts YAML-parsed data to JSON-compatible types.
// YAML uses map[string]interface{} but JSON schema expects map[string]interface{}.
// YAML also uses []interface{} for arrays which is already compatible.
func convertToJSON(v interface{}) interface{} {
	switch v := v.(type) {
	case map[string]interface{}:
		m := make(map[string]interface{})
		for k, val := range v {
			m[k] = convertToJSON(val)
		}
		return m
	case []interface{}:
		arr := make([]interface{}, len(v))
		for i, val := range v {
			arr[i] = convertToJSON(val)
		}
		return arr
	default:
		return v
	}
}

package template

import (
	"fmt"
	"sort"
	"strings"

	"github.com/1morshed1/prompt-hub/server/internal/model"
)

// ValidationError aggregates every problem found in one pass so the caller sees
// all of them at once ("role is required; count must be a number") instead of
// fixing one, re-submitting, and hitting the next.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "variable validation failed: " + strings.Join(e.Problems, "; ")
}

// BuildContext validates caller-supplied input against a version's declared
// variable schema and returns a clean context ready for rendering. It:
//
//   - fills in declared defaults for omitted optional variables,
//   - errors if a required variable is missing,
//   - type-checks each provided value against its declared type,
//   - passes through undeclared extras (templates may reference ad-hoc data; the
//     schema is a contract for the declared ones, not an allowlist).
//
// Running this before eval turns confusing mid-render failures into one upfront,
// actionable error — the difference between a tool engineers trust and one they
// don't.
func BuildContext(vars []model.Variable, input map[string]any) (map[string]any, error) {
	ctx := make(map[string]any, len(input))
	for k, v := range input {
		ctx[k] = v
	}

	var problems []string
	for _, decl := range vars {
		val, present := input[decl.Name]
		if !present {
			if decl.Required {
				problems = append(problems, fmt.Sprintf("%q is required", decl.Name))
				continue
			}
			if decl.Default != nil {
				ctx[decl.Name] = decl.Default
			}
			continue
		}
		if err := checkType(decl, val); err != nil {
			problems = append(problems, err.Error())
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, &ValidationError{Problems: problems}
	}
	return ctx, nil
}

// checkType verifies one value matches its declared type. An empty/unknown type
// declaration means "don't check", so authors can leave a variable untyped.
func checkType(decl model.Variable, val any) error {
	switch strings.ToLower(decl.Type) {
	case "", "any":
		return nil
	case "string":
		if _, ok := val.(string); !ok {
			return typeErr(decl.Name, "string", val)
		}
	case "number", "int", "integer", "float":
		if _, ok := toFloat(val); !ok {
			return typeErr(decl.Name, "number", val)
		}
	case "boolean", "bool":
		if _, ok := val.(bool); !ok {
			return typeErr(decl.Name, "boolean", val)
		}
	case "list", "array":
		if _, ok := val.([]any); !ok {
			return typeErr(decl.Name, "list", val)
		}
	case "object", "map":
		if _, ok := val.(map[string]any); !ok {
			return typeErr(decl.Name, "object", val)
		}
	default:
		// Unknown declared type: don't block rendering, but it's worth surfacing.
		return fmt.Errorf("%q has unknown declared type %q", decl.Name, decl.Type)
	}
	return nil
}

func typeErr(name, want string, got any) error {
	return fmt.Errorf("%q must be a %s, got %T", name, want, got)
}

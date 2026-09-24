package template

import (
	"fmt"
	"strconv"
)

// This file holds the runtime value semantics: how the engine decides truthiness,
// equality, ordering, and string form. JSON-decoded data arrives as a small set
// of Go types (bool, float64, string, []any, map[string]any, nil), plus we accept
// native ints/floats when callers build contexts in Go. Everything here normalizes
// across those.

// truthy defines what counts as "true" in an {% if %}. The rules mirror common
// template languages: nil is false; false is false; 0 is false; "" is false;
// empty list/map is false; everything else is true.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		if f, ok := toFloat(v); ok {
			return f != 0
		}
		return true
	}
}

// valuesEqual compares two values for ==/!=. Numbers compare numerically across
// int/float; strings and bools compare directly; otherwise fall back to Go ==
// where the types are comparable, else format-and-compare as a last resort.
func valuesEqual(a, b any) bool {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			return af == bf
		}
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return as == bs
	}
	ab, aok := a.(bool)
	bb, bok := b.(bool)
	if aok && bok {
		return ab == bb
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// compareNumbers handles < > <= >= which only make sense for numbers.
func compareNumbers(op string, a, b any) (any, error) {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if !aok || !bok {
		return nil, fmt.Errorf("operator %q needs numbers, got %T and %T", op, a, b)
	}
	switch op {
	case "<":
		return af < bf, nil
	case ">":
		return af > bf, nil
	case "<=":
		return af <= bf, nil
	case ">=":
		return af >= bf, nil
	}
	return nil, fmt.Errorf("unknown comparison %q", op)
}

// toFloat coerces the numeric types we might see into float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

// stringify renders a value for {{ output }}. Whole-number floats print without a
// trailing ".0" (JSON numbers decode as float64, so 3 arrives as 3.0 and users
// expect "3", not "3.0").
func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		if f, ok := toFloat(v); ok {
			return stringify(f)
		}
		return fmt.Sprint(v)
	}
}

// asList coerces a value into a slice for {% for %}. Only real lists iterate;
// anything else is an error so a typo (looping over a string) fails loudly.
func asList(v any) ([]any, error) {
	switch x := v.(type) {
	case []any:
		return x, nil
	case nil:
		return nil, nil // looping over an absent/empty value yields zero iterations
	default:
		return nil, fmt.Errorf("cannot iterate over %T (expected a list)", v)
	}
}

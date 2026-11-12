package ami_tag

import (
	"bytes"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"text/template"
	"unicode/utf8"

	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// missingPathRe extracts the full expression path from a
// text/template error produced by Option("missingkey=error").
// For example, `at <.AMITags.Test>: map has no entry for key "Test"`
// captures "AMITags.Test" as the path.
var missingPathRe = regexp.MustCompile(
	`at <\.([^>]+)>.*?map has no entry for key "([^"]+)"`,
)

// missingKeyRe is the fallback: it extracts just the leaf key
// name when the expression path is not available.
var missingKeyRe = regexp.MustCompile(
	`map has no entry for key "([^"]+)"`,
)

// normalizeData accepts map[string]interface{} or
// map[interface{}]interface{}; else empty map. Keys are
// converted with fmt.Sprint. Nested map[interface{}]interface{}
// values are recursed.
func normalizeData(raw interface{}) map[string]interface{} {
	switch m := raw.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			out[k] = normalizeValue(v)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			out[fmt.Sprint(k)] = normalizeValue(v)
		}
		return out
	default:
		return make(map[string]interface{})
	}
}

// normalizeValue recurses into nested map[interface{}]interface{}
// values; all other values pass through unchanged.
func normalizeValue(v interface{}) interface{} {
	if nested, ok := v.(map[interface{}]interface{}); ok {
		out := make(map[string]interface{}, len(nested))
		for nk, nv := range nested {
			out[fmt.Sprint(nk)] = normalizeValue(nv)
		}
		return out
	}
	return v
}

// interpolateTags renders each tag value once against a copy
// of base with Data replaced by data. Tags are processed in
// sorted key order. Missing keys produce errors like:
//
//	tag key "K": missing variable "V"
//
// All errors are collected; both the partial result map and
// the error slice are returned.
func interpolateTags(
	tags map[string]string,
	base interpolate.Context,
	data map[string]interface{},
) (map[string]string, []error) {
	ctx := base
	ctx.Data = data

	result := make(map[string]string, len(tags))
	var errs []error

	for _, k := range slices.Sorted(maps.Keys(tags)) {
		v := tags[k]
		tpl, err := template.New(k).
			Funcs(interpolate.Funcs(&ctx)).
			Option("missingkey=error").
			Parse(v)
		if err != nil {
			errs = append(errs,
				fmt.Errorf("tag key %q: %w", k, err))
			continue
		}

		var buf bytes.Buffer
		if err := tpl.Execute(&buf, ctx.Data); err != nil {
			if m := missingPathRe.FindStringSubmatch(
				err.Error()); m != nil {
				errs = append(errs, fmt.Errorf(
					"tag key %q: missing variable %q",
					k, m[1]))
			} else if m := missingKeyRe.FindStringSubmatch(
				err.Error()); m != nil {
				errs = append(errs, fmt.Errorf(
					"tag key %q: missing variable %q",
					k, m[1]))
			} else {
				errs = append(errs,
					fmt.Errorf("tag key %q: %w", k, err))
			}
			continue
		}
		result[k] = buf.String()
	}
	return result, errs
}

// validateRenderedTags checks that every key and value in the
// rendered tag map respects the AWS tag-length limits
// (maxKeyLen=128 runes, maxValueLen=256 runes). One error is
// returned per violation, naming the offending key and its
// rune length.
func validateRenderedTags(tags map[string]string) []error {
	var errs []error
	for k, v := range tags {
		if n := utf8.RuneCountInString(k); n > maxKeyLen {
			errs = append(errs, fmt.Errorf(
				"tag key %q length %d exceeds maximum %d",
				k, n, maxKeyLen))
		}
		if n := utf8.RuneCountInString(v); n > maxValueLen {
			errs = append(errs, fmt.Errorf(
				"tag key %q: value length %d exceeds maximum %d",
				k, n, maxValueLen))
		}
	}
	return errs
}

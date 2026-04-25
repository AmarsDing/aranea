// Package output centralises every "print result to the user" call so
// global flags like --output and --quiet behave consistently across all
// sub-commands. It supports three formats today: text, json, and table.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
)

var (
	currentFormat = "text"
	currentQuiet  = false
	colorEnabled  = true
)

// Configure is called once during PersistentPreRunE so the package can
// remember the user's preferences for the lifetime of the process.
func Configure(format string, quiet, noColor bool) {
	if format != "" {
		currentFormat = strings.ToLower(format)
	}
	currentQuiet = quiet
	if noColor {
		colorEnabled = false
	} else {
		colorEnabled = isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == ""
	}
}

// Format returns the active output format.
func Format() string { return currentFormat }

// Quiet returns true when the user requested minimal output.
func Quiet() bool { return currentQuiet }

// Color returns true when ANSI colors should be emitted.
func Color() bool { return colorEnabled }

// Render is the workhorse: pick the right encoder for the active format
// and write the value to w. Tables are produced by reflecting over the
// fields of slices of structs; if reflection fails we fall back to the
// JSON encoder so the user always sees something.
func Render(w io.Writer, value any) {
	switch currentFormat {
	case "json":
		renderJSON(w, value)
	case "table":
		if !renderTable(w, value) {
			renderJSON(w, value)
		}
	default:
		if !renderText(w, value) {
			renderJSON(w, value)
		}
	}
}

func renderJSON(w io.Writer, value any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func renderText(w io.Writer, value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		_, _ = fmt.Fprintln(w, v.String())
		return true
	case reflect.Slice, reflect.Array:
		// For slices fall back to a table; renderTable already prints to w.
		return renderTable(w, value)
	case reflect.Struct:
		// List response envelopes ({Items, Total, Limit, Offset}) read better
		// as a table than as a key/value dump, so detect that shape first.
		if items := v.FieldByName("Items"); items.IsValid() && items.Kind() == reflect.Slice {
			return renderSliceTable(w, items)
		}
		printStruct(w, v, "")
		return true
	}
	return false
}

func renderTable(w io.Writer, value any) bool {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	// Many list endpoints return {Items: [...], Total: N}. Detect and unwrap.
	if v.Kind() == reflect.Struct {
		if items := v.FieldByName("Items"); items.IsValid() && items.Kind() == reflect.Slice {
			return renderSliceTable(w, items)
		}
		return false
	}
	if v.Kind() != reflect.Slice {
		return false
	}
	return renderSliceTable(w, v)
}

func renderSliceTable(w io.Writer, slice reflect.Value) bool {
	if slice.Len() == 0 {
		if !currentQuiet {
			_, _ = fmt.Fprintln(w, "(empty)")
		}
		return true
	}
	first := slice.Index(0)
	for first.Kind() == reflect.Ptr {
		first = first.Elem()
	}
	if first.Kind() != reflect.Struct {
		return false
	}
	headers := pickColumns(first.Type())
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if !currentQuiet {
		_, _ = fmt.Fprintln(tw, strings.Join(highlightHeaders(headers), "\t"))
	}
	for i := 0; i < slice.Len(); i++ {
		elem := slice.Index(i)
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		row := make([]string, len(headers))
		for j, name := range headers {
			row[j] = stringify(elem.FieldByName(structFieldFromHeader(elem.Type(), name)))
		}
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush() == nil
}

// pickColumns returns at most six descriptive columns for a struct.
// Preference order: ID/Key/Name first, then a couple of human-readable
// fields, then a status-like field. The goal is never to dump every
// column but to give a useful at-a-glance summary.
func pickColumns(t reflect.Type) []string {
	preferred := []string{
		"ID", "Key", "Slug", "AgentKey", "ToolKey",
		"DisplayName", "Name", "Title",
		"Status", "Enabled", "RiskLevel",
		"Provider", "Model",
		"UpdatedAt", "CreatedAt",
	}
	have := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.IsExported() && !f.Anonymous {
			have[f.Name] = true
		}
	}
	chosen := []string{}
	for _, name := range preferred {
		if have[name] && len(chosen) < 6 {
			chosen = append(chosen, jsonHeader(t, name))
		}
	}
	if len(chosen) == 0 {
		// Fall back: enumerate the first six exported scalar fields.
		for i := 0; i < t.NumField() && len(chosen) < 6; i++ {
			f := t.Field(i)
			if !f.IsExported() || f.Anonymous {
				continue
			}
			switch f.Type.Kind() {
			case reflect.String, reflect.Bool,
				reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Float32, reflect.Float64:
				chosen = append(chosen, jsonHeader(t, f.Name))
			}
		}
	}
	sort.SliceStable(chosen, func(i, j int) bool { return false })
	return chosen
}

// jsonHeader returns the json tag (without options) for a field if any.
func jsonHeader(t reflect.Type, fieldName string) string {
	if f, ok := t.FieldByName(fieldName); ok {
		tag := f.Tag.Get("json")
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		if tag != "" && tag != "-" {
			return tag
		}
	}
	return strings.ToLower(fieldName)
}

// structFieldFromHeader is the inverse of jsonHeader.
func structFieldFromHeader(t reflect.Type, header string) string {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		if tag == header {
			return f.Name
		}
	}
	return header
}

func stringify(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return ""
		}
		return stringify(v.Elem())
	case reflect.String:
		return v.String()
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", v.Float())
	case reflect.Slice, reflect.Array:
		return fmt.Sprintf("[%d]", v.Len())
	case reflect.Struct:
		return "{...}"
	}
	return fmt.Sprintf("%v", v.Interface())
}

func printStruct(w io.Writer, v reflect.Value, indent string) {
	t := v.Type()
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || f.Anonymous {
			continue
		}
		header := jsonHeader(t, f.Name)
		_, _ = fmt.Fprintf(tw, "%s%s:\t%s\n", indent, dim(header), stringify(v.Field(i)))
	}
	_ = tw.Flush()
}

// Success prints a friendly success line (skipped in --quiet mode).
func Success(w io.Writer, message string) {
	if currentQuiet {
		return
	}
	if colorEnabled {
		_, _ = fmt.Fprintf(w, "\x1b[32mok\x1b[0m %s\n", message)
		return
	}
	_, _ = fmt.Fprintf(w, "ok %s\n", message)
}

// Warn prints a warning line on stderr, even in --quiet mode.
func Warn(w io.Writer, message string) {
	if colorEnabled {
		_, _ = fmt.Fprintf(w, "\x1b[33mwarn\x1b[0m %s\n", message)
		return
	}
	_, _ = fmt.Fprintf(w, "warn %s\n", message)
}

// Error prints a failure line on stderr.
func Error(w io.Writer, message string) {
	if colorEnabled {
		_, _ = fmt.Fprintf(w, "\x1b[31merror\x1b[0m %s\n", message)
		return
	}
	_, _ = fmt.Fprintf(w, "error %s\n", message)
}

func dim(s string) string {
	if !colorEnabled {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}

func highlightHeaders(headers []string) []string {
	if !colorEnabled {
		return headers
	}
	out := make([]string, len(headers))
	for i, h := range headers {
		out[i] = "\x1b[1m" + h + "\x1b[0m"
	}
	return out
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

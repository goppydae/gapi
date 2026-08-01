package cgroups

import (
	"errors"
	"math"
	"strconv"
	"testing"
)

// TestParseResourceSpecCPU covers the CPU formats and, more to the
// point, the shapes that used to convert to a silent zero.
func TestParseResourceSpecCPU(t *testing.T) {
	tests := []struct {
		name  string
		cpu   string
		want  float64
		isErr bool
	}{
		{name: "empty means no limit", cpu: "", want: 0},
		{name: "decimal", cpu: "0.5", want: 0.5},
		{name: "integer", cpu: "2", want: 2},
		{name: "fractional above one", cpu: "1.5", want: 1.5},
		{name: "millicpu", cpu: "500m", want: 0.5},
		{name: "millicpu one", cpu: "1m", want: 0.001},
		{name: "whitespace tolerated", cpu: "  0.25  ", want: 0.25},

		// The reason this package exists: each of these used to leave
		// spec.CPU at zero, and Create writes nothing for a zero field.
		{name: "fractional millicpu rejected", cpu: "0.5m", isErr: true},
		{name: "millicpu with exponent rejected", cpu: "1e3m", isErr: true},
		{name: "bare unit rejected", cpu: "m", isErr: true},
		{name: "not a number", cpu: "foo", isErr: true},
		{name: "zero", cpu: "0", isErr: true},
		{name: "negative", cpu: "-1", isErr: true},
		{name: "negative millicpu", cpu: "-500m", isErr: true},
		{name: "nan", cpu: "NaN", isErr: true},
		{name: "inf", cpu: "Inf", isErr: true},
		{name: "plus inf", cpu: "+Inf", isErr: true},
		{name: "float out of range", cpu: "1e400", isErr: true},
		{name: "quota would not fit an int", cpu: "1e300", isErr: true},
		{name: "millicpu out of int64", cpu: "99999999999999999999m", isErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseResourceSpec(tt.cpu, "")
			if tt.isErr {
				if err == nil {
					t.Fatalf("ParseResourceSpec(%q, \"\") = %+v, want error", tt.cpu, spec)
				}
				var le *LimitError
				if !errors.As(err, &le) {
					t.Fatalf("error for %q is %T, want *LimitError", tt.cpu, err)
				}
				if le.Field != "cpu" {
					t.Errorf("LimitError.Field = %q, want \"cpu\"", le.Field)
				}
				if spec != (ResourceSpec{}) {
					t.Errorf("rejected %q but returned a non-zero spec %+v", tt.cpu, spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseResourceSpec(%q, \"\") returned %v", tt.cpu, err)
			}
			if spec.CPU != tt.want {
				t.Errorf("CPU = %v, want %v", spec.CPU, tt.want)
			}
		})
	}
}

// TestParseResourceSpecMemory covers the unit table, the cutset defect
// and the overflow guard.
func TestParseResourceSpecMemory(t *testing.T) {
	tests := []struct {
		name  string
		mem   string
		want  int64
		isErr bool
	}{
		{name: "empty means no limit", mem: "", want: 0},
		{name: "bytes B", mem: "1B", want: 1},
		{name: "bytes B larger", mem: "1000B", want: 1000},
		{name: "K", mem: "10K", want: 10 << 10},
		{name: "KB", mem: "10KB", want: 10 << 10},
		{name: "M", mem: "512M", want: 512 << 20},
		{name: "MB", mem: "10MB", want: 10 << 20},
		{name: "G", mem: "1G", want: 1 << 30},
		{name: "GB", mem: "1GB", want: 1 << 30},
		{name: "lower case", mem: "100mb", want: 100 << 20},
		{name: "whitespace tolerated", mem: " 1kb ", want: 1 << 10},
		{name: "signed positive", mem: "+5MB", want: 5 << 20},

		// A bare count is rejected: the unit is what makes a manifest
		// reviewable, and an operator who means bytes writes "1024B".
		// parseLimits used to read this as 1024 bytes while the
		// validator rejected it - the two disagreed.
		{name: "bare number rejected", mem: "1024", isErr: true},

		// The cutset defect. strings.TrimRight(upper, "KB") strips every
		// trailing K and B, so both of these used to convert to 1024
		// bytes - a limit the operator never wrote - while the validator
		// rejected them. Suffix trimming makes them errors on both
		// sides.
		{name: "1KBB rejected", mem: "1KBB", isErr: true},
		{name: "1KK rejected", mem: "1KK", isErr: true},
		{name: "1MBB rejected", mem: "1MBB", isErr: true},

		{name: "unit with no number", mem: "MB", isErr: true},
		{name: "not a number", mem: "bar", isErr: true},
		{name: "fractional rejected", mem: "1.5MB", isErr: true},
		{name: "zero", mem: "0MB", isErr: true},
		{name: "negative", mem: "-100MB", isErr: true},
		{name: "past int64 as a count", mem: "9223372036854775808MB", isErr: true},
		{name: "gigabytes overflow bytes", mem: "9223372036854775807GB", isErr: true},
		{name: "kilobytes overflow bytes", mem: "9007199254740993KB", isErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseResourceSpec("", tt.mem)
			if tt.isErr {
				if err == nil {
					t.Fatalf("ParseResourceSpec(\"\", %q) = %+v, want error", tt.mem, spec)
				}
				var le *LimitError
				if !errors.As(err, &le) {
					t.Fatalf("error for %q is %T, want *LimitError", tt.mem, err)
				}
				if le.Field != "memory" {
					t.Errorf("LimitError.Field = %q, want \"memory\"", le.Field)
				}
				if spec != (ResourceSpec{}) {
					t.Errorf("rejected %q but returned a non-zero spec %+v", tt.mem, spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseResourceSpec(\"\", %q) returned %v", tt.mem, err)
			}
			if spec.Memory != tt.want {
				t.Errorf("Memory = %d, want %d", spec.Memory, tt.want)
			}
		})
	}
}

// TestParseResourceSpecNoSilentZero is the defect stated as a property:
// a non-empty limit either produces a positive field or an error. There
// is no third outcome, because Create writes a limit only when the
// field is positive - a nil error with a zero field is an agent running
// with no containment and nothing logged (GAPI-DIV-049).
func TestParseResourceSpecNoSilentZero(t *testing.T) {
	cpus := []string{"", "0.5", "500m", "0.5m", "1B", "0", "-1", "NaN", "Inf", "foo", "1e400", "m", "1e300"}
	mems := []string{"", "1B", "1000B", "1KB", "1KBB", "1KK", "1024", "100MB", "0MB", "-1MB", "bar", "9223372036854775807GB"}

	for _, cpu := range cpus {
		for _, mem := range mems {
			spec, err := ParseResourceSpec(cpu, mem)
			if err != nil {
				continue
			}
			if cpu != "" && spec.CPU <= 0 {
				t.Errorf("ParseResourceSpec(%q, %q) accepted the cpu limit but yielded CPU %v", cpu, mem, spec.CPU)
			}
			if mem != "" && spec.Memory <= 0 {
				t.Errorf("ParseResourceSpec(%q, %q) accepted the memory limit but yielded Memory %d", cpu, mem, spec.Memory)
			}
		}
	}
}

// TestParseResourceSpecMemoryBoundary pins the near side of the
// overflow guard, not only its far side: the largest representable byte
// count for each unit is accepted and multiplies out exactly.
func TestParseResourceSpecMemoryBoundary(t *testing.T) {
	for _, u := range memUnits {
		maxCount := math.MaxInt64 / u.mult
		limit := strconv.FormatInt(maxCount, 10) + u.suffix

		spec, err := ParseResourceSpec("", limit)
		if err != nil {
			t.Fatalf("rejected the largest representable %s limit %q: %v", u.suffix, limit, err)
		}
		if want := maxCount * u.mult; spec.Memory != want {
			t.Errorf("%q -> %d bytes, want %d", limit, spec.Memory, want)
		}

		// "B" has no over-side: its largest count IS MaxInt64, and one
		// more does not exist as an int64 to format.
		if maxCount == math.MaxInt64 {
			continue
		}
		over := strconv.FormatInt(maxCount+1, 10) + u.suffix
		if _, err := ParseResourceSpec("", over); err == nil {
			t.Errorf("accepted %q, which overflows int64 bytes", over)
		}
	}
}

// TestLimitErrorNamesTheField checks that the error is usable as data:
// a caller can tell which of the two limits was bad without matching on
// the message.
func TestLimitErrorNamesTheField(t *testing.T) {
	_, err := ParseResourceSpec("1", "nope")
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("error is %T, want *LimitError", err)
	}
	if le.Field != "memory" || le.Value != "nope" {
		t.Errorf("LimitError = %+v, want Field=memory Value=nope", le)
	}
}

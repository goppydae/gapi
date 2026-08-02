package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Every config key must be reachable from the environment.
//
// viper's AutomaticEnv plus Unmarshal only resolves keys viper already
// knows, and it learns them from a config file, a default, or an explicit
// bind. Keys that had no SetDefault were therefore dropped in silence:
// the whole supervisor section, security.verifyKey, and the transport TLS
// paths. GAPI_SUPERVISOR_PRODUCTIONMODE=true produced a daemon with
// signature enforcement OFF and no error (GAPI-DIV-038).
//
// This test walks the Config struct by reflection rather than listing
// keys, because the gap was exactly the keys nobody thought to list. A
// field added later is covered because it exists.

type leaf struct {
	path  string // dotted config path, e.g. "supervisor.pid1Mode"
	index []int  // field index chain into Config
	kind  reflect.Kind
}

// leaves enumerates every scalar field in the config tree.
func leaves(t reflect.Type, prefix string, idx []int) []leaf {
	var out []leaf
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		chain := append(append([]int{}, idx...), i)

		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		switch ft.Kind() {
		case reflect.Struct:
			out = append(out, leaves(ft, path, chain)...)
		case reflect.Map, reflect.Slice:
			// No single-variable spelling; config file only.
		default:
			out = append(out, leaf{path: path, index: chain, kind: ft.Kind()})
		}
	}
	return out
}

// probeValue returns a value distinguishable from both the zero value and
// the registered default, plus its string spelling for the environment.
func probeValue(k reflect.Kind, seq int) (any, string, bool) {
	switch k {
	case reflect.String:
		s := "probe-" + strconv.Itoa(seq)
		return s, s, true
	case reflect.Bool:
		// true differs from every bool default except
		// transport.insecureSkipVerify, which is handled by probing false
		// there; see the caller.
		return true, "true", true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(9000 + seq)
		return n, strconv.FormatInt(n, 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := uint64(9000 + seq)
		return n, strconv.FormatUint(n, 10), true
	case reflect.Float32, reflect.Float64:
		f := float64(9000 + seq)
		return f, strconv.FormatFloat(f, 'f', -1, 64), true
	default:
		return nil, "", false
	}
}

// fieldValue walks an index chain and returns the leaf as a comparable.
func fieldValue(cfg *Config, index []int) reflect.Value {
	v := reflect.ValueOf(cfg).Elem()
	for _, i := range index {
		for v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}

func TestEveryConfigKeyIsReachableFromTheEnvironment(t *testing.T) {
	all := leaves(reflect.TypeOf(Config{}), "", nil)
	if len(all) < 20 {
		t.Fatalf("found only %d config leaves; the walk is not reaching the tree", len(all))
	}

	// Point the loader at an empty file so no real /etc/gapi config can
	// mask the environment.
	empty := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(empty, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Timeouts must stay parseable: Load fails if a duration does not
	// parse, and this test is about reachability, not validation.
	durations := map[string]string{
		"timeouts.quicStream":         "11s",
		"timeouts.quicIdle":           "12s",
		"timeouts.clientPending":      "13s",
		"timeouts.clientTerminal":     "14s",
		"timeouts.supervisorStart":    "15s",
		"timeouts.supervisorShutdown": "16s",
	}

	for seq, l := range all {
		want, envVal, ok := probeValue(l.kind, seq)
		if !ok {
			t.Fatalf("%s: unhandled kind %s; extend probeValue rather than skipping", l.path, l.kind)
		}
		if d, isDuration := durations[l.path]; isDuration {
			want, envVal = d, d
		}
		// insecureSkipVerify defaults to true, so true would pass even if
		// the override did nothing. Probe the value that cannot.
		if l.path == "transport.insecureSkipVerify" {
			want, envVal = false, "false"
		}
		// file.compress also defaults to true.
		if l.path == "logging.file.compress" {
			want, envVal = false, "false"
		}

		t.Run(l.path, func(t *testing.T) {
			t.Setenv("GAPI_CONFIG", empty)
			t.Setenv(EnvKeyFor(l.path), envVal)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			got := fieldValue(cfg, l.index).Interface()
			wantV := reflect.ValueOf(want).Convert(fieldValue(cfg, l.index).Type()).Interface()
			if got != wantV {
				t.Fatalf("%s = %v via %s=%s, want %v; this key is not reachable from the environment",
					l.path, got, EnvKeyFor(l.path), envVal, wantV)
			}
		})
	}
}

// The keys whose absence was security-relevant, asserted by name so the
// intent survives a refactor of the reflection walk.
func TestSecurityRelevantOverridesApply(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(empty, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GAPI_CONFIG", empty)
	t.Setenv("GAPI_SUPERVISOR_PRODUCTIONMODE", "true")
	t.Setenv("GAPI_SUPERVISOR_PID1MODE", "true")
	t.Setenv("GAPI_SECURITY_VERIFYKEY", "/probe/agent.pub.hex")
	t.Setenv("GAPI_TRANSPORT_TLSCERT", "/probe/server.crt")
	t.Setenv("GAPI_TRANSPORT_TLSKEY", "/probe/server.key")
	t.Setenv("GAPI_TRANSPORT_INSECURESKIPVERIFY", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Supervisor.ProductionMode {
		t.Error("productionMode did not apply; a daemon told to run in production mode would not be")
	}
	if !cfg.Supervisor.Pid1Mode {
		t.Error("pid1Mode did not apply")
	}
	if cfg.Security.VerifyKey != "/probe/agent.pub.hex" {
		t.Errorf("verifyKey = %q, want /probe/agent.pub.hex", cfg.Security.VerifyKey)
	}
	if cfg.Transport.TLSCert != "/probe/server.crt" {
		t.Errorf("tlsCert = %q, want /probe/server.crt", cfg.Transport.TLSCert)
	}
	if cfg.Transport.TLSKey != "/probe/server.key" {
		t.Errorf("tlsKey = %q, want /probe/server.key", cfg.Transport.TLSKey)
	}
	if cfg.Transport.InsecureSkipVerify {
		t.Error("insecureSkipVerify did not apply; peer verification would stay off")
	}
}

// A config file still wins over nothing, and the environment still wins
// over the file. Binding every key must not have inverted precedence.
func TestPrecedenceEnvOverFileOverDefault(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	body := "logging:\n  level: warn\nsupervisor:\n  productionMode: true\n"
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("default", func(t *testing.T) {
		empty := filepath.Join(t.TempDir(), "empty.yaml")
		if err := os.WriteFile(empty, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GAPI_CONFIG", empty)
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Logging.Level != "info" {
			t.Errorf("level = %q, want the default info", cfg.Logging.Level)
		}
		if cfg.Supervisor.ProductionMode {
			t.Error("productionMode defaulted to true")
		}
	})

	t.Run("file beats default", func(t *testing.T) {
		t.Setenv("GAPI_CONFIG", file)
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Logging.Level != "warn" {
			t.Errorf("level = %q, want warn from the file", cfg.Logging.Level)
		}
		if !cfg.Supervisor.ProductionMode {
			t.Error("productionMode from the file did not apply")
		}
	})

	t.Run("env beats file", func(t *testing.T) {
		t.Setenv("GAPI_CONFIG", file)
		t.Setenv("GAPI_LOGGING_LEVEL", "error")
		t.Setenv("GAPI_SUPERVISOR_PRODUCTIONMODE", "false")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Logging.Level != "error" {
			t.Errorf("level = %q, want error from the environment", cfg.Logging.Level)
		}
		if cfg.Supervisor.ProductionMode {
			t.Error("the environment did not override productionMode from the file")
		}
	})
}

// Load must not leak state between calls. It used viper's package-level
// singleton, so bindings and defaults accumulated across every call in a
// process.
func TestLoadDoesNotLeakBetweenCalls(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(empty, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GAPI_CONFIG", empty)
	t.Setenv("GAPI_SUPERVISOR_PRODUCTIONMODE", "true")
	first, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !first.Supervisor.ProductionMode {
		t.Fatal("productionMode did not apply on the first load")
	}

	// t.Setenv above registers the restore; unsetting here is what the
	// second load must observe.
	if err := os.Unsetenv("GAPI_SUPERVISOR_PRODUCTIONMODE"); err != nil {
		t.Fatal(err)
	}
	second, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if second.Supervisor.ProductionMode {
		t.Error("productionMode survived into a load with the variable unset")
	}
}

func TestEnvKeyForSpelling(t *testing.T) {
	for path, want := range map[string]string{
		"supervisor.pid1Mode":             "GAPI_SUPERVISOR_PID1MODE",
		"transport.tlsCert":               "GAPI_TRANSPORT_TLSCERT",
		"logging.file.maxBackups":         "GAPI_LOGGING_FILE_MAXBACKUPS",
		"security.verifyKey":              "GAPI_SECURITY_VERIFYKEY",
		"supervisor.watchdog.interval":    "GAPI_SUPERVISOR_WATCHDOG_INTERVAL",
		"supervisor.shutdown.gracePeriod": "GAPI_SUPERVISOR_SHUTDOWN_GRACEPERIOD",
	} {
		if got := EnvKeyFor(path); got != want {
			t.Errorf("EnvKeyFor(%q) = %q, want %q", path, got, want)
		}
	}
}

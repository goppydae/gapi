// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/goppydae/gapi/core/product"
)

// The runtime address file: where a daemon publishes the control-plane
// address it ACTUALLY BOUND, so its control binary can find it without
// sharing an environment (GAPI-DIV-070).
//
// THE ADDRESS MUST BE THE RESOLVED ONE, NOT THE CONFIGURED ONE. That is
// the whole point: a config of ":0" or a hostname that resolves
// differently leaves the daemon as the only party that knows where it
// ended up, which is the defect this file exists to remove. Publishing
// the configured value would reintroduce it one layer up.
//
// WHY /run AND XDG_RUNTIME_DIR RATHER THAN /var/lib. A listen address is
// VOLATILE state - it is meaningless once the process is gone - and
// these directories are cleared on boot, so a crash cannot leave a file
// that outlives its daemon across a reboot. /var/lib is for state that
// must survive one. This mirrors the tiering already in agent_paths.go,
// where /run/<p>/agents is documented as "transient, generated at
// runtime" and the user tier is XDG_RUNTIME_DIR.
//
// The tier list is ordered rather than scoped, which is what lets the
// client find the daemon WITHOUT being told which kind of daemon it is:
// a developer's unprivileged daemon writes the user tier and a systemd
// unit (which has no XDG_RUNTIME_DIR) writes /run, and the reader probes
// the same list in the same order.

// controlAddrBase is the file name inside each tier.
const controlAddrBase = "control.addr"

// ControlAddrFiles returns the candidate address files, highest priority
// first. A system daemon under systemd has no XDG_RUNTIME_DIR and gets
// exactly one entry; that is normal and not a degraded case.
func ControlAddrFiles() []string {
	p := product.Name()
	var paths []string
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		paths = append(paths, filepath.Join(run, p, controlAddrBase))
	}
	return append(paths, filepath.Join("/run", p, controlAddrBase))
}

// WriteControlAddr publishes addr to the highest tier it can write and
// returns the file it used.
//
// Falling through to the next tier on failure is deliberate: an
// unprivileged daemon cannot create /run/<p> and must not be fatal for
// it, while a systemd unit with RuntimeDirectory= can. Only exhausting
// every tier is an error, and it names them all - a daemon that could
// not publish its address is reachable only with an explicit flag, and
// the operator needs to know that before the first control call fails.
func WriteControlAddr(addr string) (string, error) {
	var attempts []string
	for _, path := range ControlAddrFiles() {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			attempts = append(attempts, fmt.Sprintf("%s (%v)", path, err))
			continue
		}
		// 0600/0750, owner-only. An earlier draft used 0644 reasoning
		// that "the address is not a secret", which is true and is not
		// the question: nothing in this repository requires a control
		// binary to run as a different user than the daemon, so the
		// wider mode bought a speculative case at a real cost. Where a
		// daemon and its operator genuinely differ, group ownership on
		// the runtime directory is the mechanism for it - not a
		// world-readable file naming an open port.
		if err := os.WriteFile(path, []byte(addr+"\n"), 0o600); err != nil {
			attempts = append(attempts, fmt.Sprintf("%s (%v)", path, err))
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("publish control address %s: no writable runtime directory, tried: %s",
		addr, strings.Join(attempts, "; "))
}

// ReadControlAddr returns the published address and THE FILE IT CAME
// FROM, or empty strings when no daemon has published one.
//
// The source is returned rather than discarded because the caller cannot
// otherwise report it, and "a bare timeout that names neither address"
// is the failure this entry was filed for. An address file can also be
// stale - a daemon that died without cleaning up - and only a dial can
// discover that, so naming the source is what lets the dialler say "the
// file says X and nothing is there" instead.
//
// A missing file is NOT an error: no daemon has run, which is the
// ordinary state of a clean host.
func ReadControlAddr() (addr string, from string, err error) {
	for _, path := range ControlAddrFiles() {
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			if errors.Is(rerr, fs.ErrNotExist) {
				continue
			}
			return "", "", fmt.Errorf("read control address %s: %w", path, rerr)
		}
		got := strings.TrimSpace(string(raw))
		if got == "" {
			continue // a truncated write is not an address
		}
		return got, path, nil
	}
	return "", "", nil
}

// RemoveControlAddr deletes the published address on shutdown.
//
// Absence is success: shutdown runs on paths where the write never
// happened (a daemon that failed before binding), and a cleanup that
// errors there would turn an orderly stop into a noisy one.
func RemoveControlAddr() error {
	for _, path := range ControlAddrFiles() {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove control address %s: %w", path, err)
		}
	}
	return nil
}

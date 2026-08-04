// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"testing"
	"time"
)

func TestParseSchedule_Systemd(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		wantErr  bool
	}{
		{"OnUnitActiveSec", "OnUnitActiveSec=5s", false},
		{"OnBootSec", "OnBootSec=1m", false},
		{"OnStartupSec", "OnStartupSec=30s", false},
		{"Raw duration", "10s", false},
		{"Invalid duration", "OnUnitActiveSec=invalid", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := ParseSchedule(tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSchedule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && sched == nil {
				t.Error("ParseSchedule() returned nil schedule")
			}
		})
	}
}

func TestParseSchedule_Cron(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		wantErr  bool
	}{
		{"Every 5 minutes", "*/5 * * * *", false},
		{"Hourly", "0 * * * *", false},
		{"Daily at midnight", "0 0 * * *", false},
		{"Weekdays at 9am", "0 9 * * 1-5", false},
		{"Named: hourly", "@hourly", false},
		{"Named: daily", "@daily", false},
		{"Named: weekly", "@weekly", false},
		{"Named: monthly", "@monthly", false},
		{"Invalid cron", "invalid cron", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := ParseSchedule(tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSchedule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && sched == nil {
				t.Error("ParseSchedule() returned nil schedule")
			}
		})
	}
}

func TestIntervalSchedule_Next(t *testing.T) {
	sched := &IntervalSchedule{interval: 5 * time.Second}
	now := time.Now()
	next := sched.Next(now)

	expected := now.Add(5 * time.Second)
	if !next.Equal(expected) {
		t.Errorf("Next() = %v, want %v", next, expected)
	}
}

func TestCronSchedule_Next(t *testing.T) {
	// Test @hourly schedule
	sched, err := ParseSchedule("@hourly")
	if err != nil {
		t.Fatalf("ParseSchedule() error = %v", err)
	}

	now := time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC)
	next := sched.Next(now)

	// Should be next hour at :00
	expected := time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("Next() = %v, want %v", next, expected)
	}
}

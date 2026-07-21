package termux

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// @description    Exercises ApplyLocalTimezone across TZ and getprop resolution outcomes.
//
// TestApplyLocalTimezone fakes getprop through PATH and confirms a valid TZ wins, an absent or
// invalid TZ falls back to the getprop-reported device zone, and getprop failures leave
// time.Local unchanged.
//
// @param           t  "test handle used for table-driven timezone assertions"
func TestApplyLocalTimezone(t *testing.T) {
	orig := time.Local
	defer func() { time.Local = orig }()

	tmp := t.TempDir()
	writeFake := func(dir, body string) string {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "getprop")
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	binDir := writeFake(filepath.Join(tmp, "bin"), "#!/bin/sh\necho '  Asia/Shanghai  '\n")
	failDir := writeFake(filepath.Join(tmp, "failbin"), "#!/bin/sh\nexit 1\n")
	emptyDir := filepath.Join(tmp, "emptybin")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		tz   string
		path string
		want string
	}{
		{name: "TZ wins over getprop", tz: "America/New_York", path: binDir, want: "America/New_York"},
		{name: "getprop used when TZ empty", tz: "", path: binDir, want: "Asia/Shanghai"},
		{name: "getprop used when TZ invalid", tz: "Not/AZone", path: binDir, want: "Asia/Shanghai"},
		{name: "getprop failure keeps UTC", tz: "", path: failDir, want: "UTC"},
		{name: "missing getprop keeps UTC", tz: "", path: emptyDir, want: "UTC"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			time.Local = time.UTC
			t.Setenv("TZ", tc.tz)
			t.Setenv("PATH", tc.path)
			ApplyLocalTimezone()
			if time.Local.String() != tc.want {
				t.Errorf("time.Local = %v, want %v", time.Local, tc.want)
			}
		})
	}
}

// @description    Exercises applyLocalTimezone across valid, unknown, and empty zone names.
//
// @param           t  "test handle used for table-driven zone assertions"
func Test_applyLocalTimezone(t *testing.T) {
	orig := time.Local
	defer func() { time.Local = orig }()

	cases := []struct {
		name      string
		zone      string
		wantApply bool
	}{
		{name: "valid zone applied", zone: "Asia/Shanghai", wantApply: true},
		{name: "colon-prefixed zone applied", zone: ":Asia/Shanghai", wantApply: true},
		{name: "unknown zone rejected", zone: "Not/AZone", wantApply: false},
		{name: "empty zone rejected", zone: "", wantApply: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			time.Local = time.UTC
			got := applyLocalTimezone(tc.zone)
			if got != tc.wantApply {
				t.Errorf("applyLocalTimezone(%q) = %v, want %v", tc.zone, got, tc.wantApply)
			}
			if tc.wantApply {
				if time.Local.String() != "Asia/Shanghai" {
					t.Errorf("time.Local = %v, want Asia/Shanghai", time.Local)
				}
				if hour := time.Unix(0, 0).In(time.Local).Hour(); hour != 8 {
					t.Errorf("epoch hour in time.Local = %d, want 8 (UTC+8)", hour)
				}
			} else if time.Local != time.UTC {
				t.Errorf("applyLocalTimezone(%q) changed time.Local to %v, want UTC unchanged", tc.zone, time.Local)
			}
		})
	}
}

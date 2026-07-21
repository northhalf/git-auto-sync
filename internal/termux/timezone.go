package termux

import (
	"os"
	"strings"
	"time"
)

// @description    Adopts the Android device timezone as the process local timezone.
//
// ApplyLocalTimezone works around Go's android builds pinning time.Local to UTC: the runtime's
// initLocal never consults TZ or /etc/localtime on android (src/time/zoneinfo_android.go leaves a
// TODO for getprop persist.sys.timezone), so without this every formatted timestamp and log line
// reads UTC while the device clock shows the local zone. It is intended for Android call sites,
// which gate on the platform themselves (a runtime check or the android build tag).
//
// A valid TZ value wins; otherwise the device zone comes from getprop persist.sys.timezone,
// resolved through PATH because its install location varies (a Termux package may shadow
// /system/bin/getprop). Any failure leaves time.Local unchanged.
func ApplyLocalTimezone() {
	if applyLocalTimezone(os.Getenv("TZ")) {
		return
	}
	out, err := Command("getprop", "persist.sys.timezone").Output()
	if err != nil {
		return
	}
	applyLocalTimezone(strings.TrimSpace(string(out)))
}

// @description    Assigns a named IANA location to time.Local.
//
// applyLocalTimezone strips a POSIX-style leading colon from name and loads the location through
// time.LoadLocation, which on android reads the system tzdata bundle. Unknown or empty names
// leave time.Local unchanged.
//
// @param           name  "IANA zone name such as Asia/Shanghai, optionally colon-prefixed"
//
// @return          bool  "true when time.Local was set to the named location"
func applyLocalTimezone(name string) bool {
	name = strings.TrimPrefix(name, ":")
	if name == "" {
		return false
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return false
	}
	time.Local = loc
	return true
}

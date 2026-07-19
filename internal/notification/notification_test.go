package notification

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type fakeNotifier struct {
	title   string
	content string
	warned  bool
}

func (f *fakeNotifier) Alert(title, content string) error {
	f.title = title
	f.content = content
	return nil
}

func (f *fakeNotifier) WarnIfUnavailable(*slog.Logger) { f.warned = true }

// @description    Verifies an unavailable Termux notification command is reported once per notifier.
//
// @param           t  "test handle used for assertions"
func TestTermuxNotifierWarnsOnlyOnceWhenUnavailable(t *testing.T) {
	var stderr bytes.Buffer
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	n := newTermuxNotifier(
		func() (string, error) { return "", errors.New("not found") },
		func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		&stderr,
		10*time.Second,
	)

	n.WarnIfUnavailable(logger)
	n.WarnIfUnavailable(logger)

	const warning = "Android notifications are unavailable"
	if got := strings.Count(stderr.String(), warning); got != 1 {
		t.Fatalf("stderr warning count = %d, want 1; output: %q", got, stderr.String())
	}
	if got := strings.Count(logs.String(), warning); got != 1 {
		t.Fatalf("log warning count = %d, want 1; output: %q", got, logs.String())
	}
}

// @description    Verifies Termux notifications pass title and content as separate command arguments.
//
// @param           t  "test handle used for assertions"
func TestTermuxNotifierAlertPassesArgumentsWithoutShell(t *testing.T) {
	var gotPath string
	var gotArgs []string
	n := newTermuxNotifier(
		func() (string, error) { return "/prefix/bin/termux-notification", nil },
		func(_ context.Context, path string, args ...string) ([]byte, error) {
			gotPath = path
			gotArgs = append([]string(nil), args...)
			return nil, nil
		},
		&bytes.Buffer{},
		10*time.Second,
	)

	if err := n.Alert("paused 'repo'", "path; rm -rf / is text"); err != nil {
		t.Fatalf("Alert returned error %v, want nil", err)
	}

	if gotPath != "/prefix/bin/termux-notification" {
		t.Fatalf("command path = %q, want Termux notification path", gotPath)
	}
	want := []string{"--title", "paused 'repo'", "--content", "path; rm -rf / is text"}
	if len(gotArgs) != len(want) {
		t.Fatalf("command args = %q, want %q", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Fatalf("command arg %d = %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

// @description    Verifies a missing Termux notification command returns the availability sentinel.
//
// @param           t  "test handle used for assertions"
func TestTermuxNotifierAlertReturnsUnavailableSentinel(t *testing.T) {
	n := newTermuxNotifier(
		func() (string, error) { return "", errors.New("not found") },
		func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		&bytes.Buffer{},
		10*time.Second,
	)

	err := n.Alert("title", "content")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Alert error = %v, want ErrUnavailable", err)
	}
}

// @description    Verifies Termux executable discovery prefers the command under PREFIX.
//
// @param           t  "test handle used for assertions"
func TestFindTermuxNotificationPrefersPrefix(t *testing.T) {
	var lookedUp []string
	path, err := findTermuxNotification(
		func(key string) string {
			if key == "PREFIX" {
				return "/data/data/com.termux/files/usr"
			}
			return ""
		},
		func(name string) (string, error) {
			lookedUp = append(lookedUp, name)
			return name, nil
		},
	)
	if err != nil {
		t.Fatalf("findTermuxNotification returned error %v, want nil", err)
	}
	want := "/data/data/com.termux/files/usr/bin/termux-notification"
	if path != want {
		t.Fatalf("notification path = %q, want %q", path, want)
	}
	if len(lookedUp) != 1 || lookedUp[0] != want {
		t.Fatalf("lookups = %q, want only PREFIX path", lookedUp)
	}
}

// @description    Verifies package notification functions delegate to the selected platform backend.
//
// @param           t  "test handle used for assertions"
func TestPackageFunctionsDelegateToPlatformNotifier(t *testing.T) {
	previous := defaultNotifier
	fake := &fakeNotifier{}
	defaultNotifier = fake
	t.Cleanup(func() { defaultNotifier = previous })

	if err := Alert("title", "content"); err != nil {
		t.Fatalf("Alert returned error %v, want nil", err)
	}
	WarnIfUnavailable(slog.Default())

	if fake.title != "title" || fake.content != "content" {
		t.Fatalf("delegated notification = (%q, %q), want title and content", fake.title, fake.content)
	}
	if !fake.warned {
		t.Fatal("WarnIfUnavailable did not delegate to the platform notifier")
	}
}

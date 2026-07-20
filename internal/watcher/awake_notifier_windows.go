package watcher

import (
	"context"
	"log/slog"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procRegisterSuspendResume   = user32.NewProc("RegisterSuspendResumeNotification")
	procUnregisterSuspendResume = user32.NewProc("UnregisterSuspendResumeNotification")
)

const (
	// deviceNotifyCallback selects the callback delivery mode for RegisterSuspendResumeNotification,
	// where the recipient is a pointer to a DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS structure instead of a
	// window handle.
	deviceNotifyCallback = 0x00000002

	// pbtApmResumAutomatic is delivered every time the system resumes from a low-power state, including
	// unattended (remote wake) resumes.
	pbtApmResumAutomatic = 0x00000012

	// pbtApmResumSuspend is delivered after pbtApmResumAutomatic when the resume follows user input,
	// such as a key press or power button.
	pbtApmResumSuspend = 0x00000007
)

// deviceNotifySubscribeParameters mirrors the Win32 DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS structure: a
// callback function pointer plus an opaque context pointer that Windows passes back to the callback.
type deviceNotifySubscribeParameters struct {
	callback uintptr
	context  uintptr
}

// deviceNotifyCallbackRoutine is the Win32 callback signature. Windows invokes it on suspend and
// resume events; Type carries the PBT_APM* event code. Returning ERROR_SUCCESS (0) acknowledges the
// notification.
type deviceNotifyCallbackRoutine func(context uintptr, eventType uint32, setting uintptr) uintptr

// @description    Calls the Win32 RegisterSuspendResumeNotification with a callback recipient.
//
// registerSuspendResumeNotificationWrap wraps the DEVICE_NOTIFY_CALLBACK form of the power
// notification registration so the caller does not touch LazyProc directly. A null return value is
// translated into the errno reported by GetLastError, or syscall.EINVAL when Windows reports none.
//
// @param           params  "pointer to the callback subscribe parameters Windows invokes on power events"
//
// @param           flags   "DEVICE_NOTIFY_* flags; pass deviceNotifyCallback for callback delivery"
//
// @return          handle  "notification handle to pass to unregisterSuspendResumeNotificationWrap"
//
// @return          error   "nil on success, or the errno from GetLastError when registration fails"
func registerSuspendResumeNotificationWrap(params *deviceNotifySubscribeParameters, flags uint32) (windows.Handle, error) {
	handle, _, err := procRegisterSuspendResume.Call(uintptr(unsafe.Pointer(params)), uintptr(flags))
	if handle == 0 {
		if err == windows.Errno(0) {
			return 0, syscall.EINVAL
		}
		return 0, err
	}
	return windows.Handle(handle), nil
}

// @description    Releases a power notification handle so Windows stops invoking its callback.
//
// unregisterSuspendResumeNotificationWrap wraps UnregisterSuspendResumeNotification so the caller
// does not touch LazyProc directly. It is safe to call once per handle returned by
// registerSuspendResumeNotificationWrap.
//
// @param           handle  "notification handle returned by registerSuspendResumeNotificationWrap"
//
// @return          error   "nil on success, or the errno from GetLastError when unregistration fails"
func unregisterSuspendResumeNotificationWrap(handle windows.Handle) error {
	r, _, err := procUnregisterSuspendResume.Call(uintptr(handle))
	if r == 0 {
		return err
	}
	return nil
}

// AwakeNotifierWindows forwards Windows resume events delivered through RegisterSuspendResumeNotification.
// When registration fails it degrades to a no-op that never forwards events, so the watcher keeps
// running on its periodic and filesystem triggers.
type AwakeNotifierWindows struct {
	logger *slog.Logger
}

// @description    Creates the Windows wake notifier.
//
// NewAwakeNotifier returns a notifier that registers for suspend and resume callbacks when Start
// runs. Construction never fails on Windows 8 and newer, where the power notification API is present.
//
// @param           logger  "repository-scoped logger used for degradation diagnostics"
//
// @return          *AwakeNotifierWindows  "notifier that registers for power callbacks on Start"
//
// @return          error                  "always nil"
func NewAwakeNotifier(logger *slog.Logger) (*AwakeNotifierWindows, error) {
	return &AwakeNotifierWindows{logger: logger}, nil
}

// @description    Forwards Windows resume events until cancellation.
//
// Start registers a power notification callback and forwards each resume event to the supplied
// channel. When registration fails, Start returns nil without forwarding anything, so the watcher
// keeps running on its other triggers. A background goroutine unregisters the notification handle
// when ctx is canceled so Windows stops invoking the callback.
//
// @param           ctx    "context whose cancellation unregisters the power notification"
//
// @param           out    "channel that receives wake notifications"
//
// @return          error  "always nil; registration failures degrade to a no-op instead of an error"
func (a *AwakeNotifierWindows) Start(ctx context.Context, out chan<- bool) error {
	callback := func(_ uintptr, eventType uint32, _ uintptr) uintptr {
		if eventType == pbtApmResumAutomatic || eventType == pbtApmResumSuspend {
			select {
			case out <- true:
			default:
				// The channel is buffered; a dropped wake event is harmless because the periodic ticker
				// re-syncs and only one resume sync is needed.
			}
		}
		return 0
	}

	params := deviceNotifySubscribeParameters{
		callback: windows.NewCallback(deviceNotifyCallbackRoutine(callback)),
	}
	handle, err := registerSuspendResumeNotificationWrap(&params, deviceNotifyCallback)
	if err != nil {
		if a.logger != nil {
			a.logger.Info("awake notifier disabled", "reason", "register suspend-resume failed", "error", err)
		}
		return nil
	}
	if a.logger != nil {
		a.logger.Info("awake notifier started")
	}

	go func() {
		<-ctx.Done()
		_ = unregisterSuspendResumeNotificationWrap(handle)
	}()

	return nil
}

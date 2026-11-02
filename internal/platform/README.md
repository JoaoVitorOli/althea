# Platform

The operating system, behind interfaces. [`platform.go`](platform.go) declares one small
interface per service; `windows/` and `linux/` implement them; [`host/`](host/) picks the set for
the running system. Nothing above this folder calls an OS API, which is why the core builds and
tests on either system and why Linux support is adapters rather than a rewrite.

```
 app ──▶ host.Services() ──▶ platform.Services{Instance, System, Chrome, KeyHook, Locale}
                                   │
                    ┌──────────────┴──────────────┐
                 windows/                       linux/
          Win32 via x/sys/windows        v1.1; most services nil
```

A nil service means the system cannot do it, and callers leave the feature out: on Linux the
System extension shows no commands and onboarding does not offer the Win key.

## Hard-won details

These cost a bug each. Keep them in mind when changing the adapters.

- **Single instance uses a named mutex, not MyGo's lock.** MyGo locks with an AF_UNIX socket in
  the temp folder. Windows binds such a socket under a path with an accent, as
  `C:\Users\João\AppData\Local\Temp`, but cannot connect to it, so the second launch took the
  lock over and two Althea ran. A mutex and an event in the `Local\` namespace depend on no
  path. Linux keeps MyGo's lock.
- **`INPUT` is 40 bytes on 64-bit Windows, with padding after `type`.** Without it `SendInput`
  rejects the masking key, and the Start menu opens alongside Althea on every Win tap. A test
  pins the layout.
- **The keyboard hook must return at once.** Windows silently drops a low-level hook that keeps
  it waiting. The hook only feeds `hotkey.WinTap` and hands the toggle to the main thread.
- **Injected keys are ignored by the Win key detection,** including Althea's own masking key.
  That also means automated tests cannot press the Win key for real; check it by hand.
- **A window's rectangle is larger than the window.** Since Windows 10 it holds an invisible
  border to grab it by, so moving by `GetWindowRect` leaves gaps at the screen's edges. Frames here
  are DWM's extended frame bounds, and `Place` adds the border back. Crossing to a display of
  another scale changes the border as the window arrives, hence its second pass.
- **Store apps' windows belong to `ApplicationFrameHost.exe`.** The real app is the process of a
  child window, and it opens again by `shell:AppsFolder\<AppUserModelID>`, not by its program.
- **System callbacks are made once.** `windows.NewCallback` is never freed and has a limit, so
  enumerations share package-level callbacks under a mutex rather than making closures per call.
- **Binding a hotkey waits for the main thread.** A service unbinds as it stops, and the main
  thread waits for services as Althea quits, so extensions' hotkeys go through a queue.
- **A folder is watched from the first read on, not from opening it.** That read goes out before
  `WatchTree` returns, or a change made right after it is lost. Reads are overlapped, because
  `CancelIoEx` does nothing to a blocking read that has not begun yet, and quitting would hang.
- **The selection is read by pressing Ctrl+C.** Nothing else works on every kind of app. It hides the
  launcher, gives the app the keyboard, waits for the clipboard's sequence number to move, and puts
  the clipboard back; clipboard history ignores what changes meanwhile. Terminals are left alone,
  where Ctrl+C stops what runs, so they have no selection to offer.
- **System programs run by full path** (`System32\shutdown.exe`), never through `PATH`.

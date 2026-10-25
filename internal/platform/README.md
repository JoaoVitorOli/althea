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
- **System programs run by full path** (`System32\shutdown.exe`), never through `PATH`.

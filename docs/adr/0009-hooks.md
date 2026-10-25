# 9. Low-level keyboard hook only when needed

Status: accepted (2026-10-25)

## Decision

Normal hotkeys use `RegisterHotKey` (through MyGo). A `WH_KEYBOARD_LL` hook runs only when the user
picks the bare `Win` key as hotkey or turns on snippet expansion. For `Win`, a press and release
with no other key in between opens Althea, and a masking key suppresses the Start menu. The hook
keeps no keystrokes beyond the snippet keyword buffer and pauses in password fields.

## Consequences

- Offers the bare Win key without making every user run a global hook.
- The hook callback must return fast (Windows drops slow hooks); work is handed to a goroutine.

//go:build windows

package windows

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// KnownFolders returns the user's Desktop, Documents, Downloads, Pictures, Videos and Music
// folders, where they really are: OneDrive may have moved them.
func KnownFolders() []string {
	var out []string
	for _, id := range []*windows.KNOWNFOLDERID{
		windows.FOLDERID_Desktop, windows.FOLDERID_Documents, windows.FOLDERID_Downloads,
		windows.FOLDERID_Pictures, windows.FOLDERID_Videos, windows.FOLDERID_Music,
	} {
		if p, err := windows.KnownFolderPath(id, 0); err == nil && p != "" {
			out = append(out, p)
		}
	}
	return out
}

// treeEvents are the changes a tree watcher hears of: names, attributes that hide files,
// sizes and writes, creations and deletions.
const treeEvents = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_ATTRIBUTES | windows.FILE_NOTIFY_CHANGE_SIZE |
	windows.FILE_NOTIFY_CHANGE_LAST_WRITE | windows.FILE_NOTIFY_CHANGE_CREATION

// WatchTree calls changed with the path of each file or folder that changes anywhere under
// dir, until stop; "" means changes were lost, as when too many came at once, and the whole
// tree should be read again. It implements platform.TreeWatcher.
//
// The system watches from the first read on, so that read goes out before WatchTree returns: a
// change made after it is heard, however late the goroutine gets to run. Reads are overlapped so
// that stop can always end the wait: canceling a blocking read does nothing unless it has begun.
func WatchTree(dir string, changed func(path string)) (stop func(), err error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	completed, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	stopped, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(completed)
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	closeAll := func() {
		_ = windows.CloseHandle(stopped)
		_ = windows.CloseHandle(completed)
		_ = windows.CloseHandle(h)
	}
	const size = 64 << 10
	buf := make([]byte, size)
	overlapped := &windows.Overlapped{HEvent: completed}
	read := func() error {
		var n uint32
		err := windows.ReadDirectoryChanges(h, &buf[0], size, true, treeEvents, &n, overlapped, 0)
		if errors.Is(err, windows.ERROR_IO_PENDING) {
			return nil
		}
		return err
	}
	if err := read(); err != nil {
		closeAll()
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			which, err := windows.WaitForMultipleObjects([]windows.Handle{completed, stopped}, false, windows.INFINITE)
			if err != nil || which != windows.WAIT_OBJECT_0 {
				// The kernel writes into buf until the read ends: wait for it before leaving.
				_ = windows.CancelIoEx(h, overlapped)
				var n uint32
				_ = windows.GetOverlappedResult(h, overlapped, &n, true)
				return
			}
			var n uint32
			err = windows.GetOverlappedResult(h, overlapped, &n, false)
			switch {
			case errors.Is(err, windows.ERROR_NOTIFY_ENUM_DIR) || (err == nil && n == 0):
				changed("") // the buffer overflowed: some changes are lost
			case err != nil:
				changed("")
				return // the folder went away, or the volume
			}
			for off := uint32(0); off < n; {
				info := (*windows.FileNotifyInformation)(unsafe.Pointer(&buf[off]))
				name := windows.UTF16ToString(unsafe.Slice(&info.FileName, info.FileNameLength/2))
				changed(filepath.Join(dir, name))
				if info.NextEntryOffset == 0 {
					break
				}
				off += info.NextEntryOffset
			}
			if err := windows.ResetEvent(completed); err != nil {
				changed("")
				return
			}
			if err := read(); err != nil {
				changed("")
				return
			}
		}
	}()
	var stopping sync.Once
	return func() {
		stopping.Do(func() {
			_ = windows.SetEvent(stopped)
			<-done
			closeAll()
		})
	}, nil
}

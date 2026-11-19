//go:build windows

package main

import (
	"context"
	"log"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

type taskbarState struct{ playing, hasTrack bool }

type thumbButton struct {
	mask   uint32
	id     uint32
	bitmap uint32
	icon   uintptr
	tip    [260]uint16
	flags  uint32
}

var (
	taskbarUpdates            = make(chan taskbarState, 1)
	taskbarStop               = make(chan struct{})
	taskbarStartOnce          sync.Once
	taskbarStopOnce           sync.Once
	user32                    = windows.NewLazySystemDLL("user32.dll")
	enumWindows               = user32.NewProc("EnumWindows")
	getWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	isWindowVisible           = user32.NewProc("IsWindowVisible")
	getWindowText             = user32.NewProc("GetWindowTextW")
	setWindowLongPtr          = user32.NewProc("SetWindowLongPtrW")
	getWindowLongPtr          = user32.NewProc("GetWindowLongPtrW")
	callWindowProc            = user32.NewProc("CallWindowProcW")
	registerWindowMessage     = user32.NewProc("RegisterWindowMessageW")
	createIcon                = user32.NewProc("CreateIcon")
	destroyIcon               = user32.NewProc("DestroyIcon")
	findTaskbarWindowCallback = syscall.NewCallback(func(hwnd, result uintptr) uintptr {
		var processID uint32
		getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&processID)))
		visible, _, _ := isWindowVisible.Call(hwnd)
		if processID != uint32(os.Getpid()) || visible == 0 {
			return 1
		}
		var title [256]uint16
		getWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		if windows.UTF16ToString(title[:]) == "Cryon2" {
			*(*uintptr)(unsafe.Pointer(result)) = hwnd
			return 0
		}
		return 1
	})
)

func startTaskbarControls(ctx context.Context) {
	taskbarStartOnce.Do(func() { go runTaskbarControls(ctx) })
}

func stopTaskbarControls() { taskbarStopOnce.Do(func() { close(taskbarStop) }) }

func setTaskbarPlaybackState(playing, hasTrack bool) {
	state := taskbarState{playing: playing, hasTrack: hasTrack}
	select {
	case <-taskbarUpdates:
	default:
	}
	select {
	case taskbarUpdates <- state:
	default:
	}
}

func findTaskbarWindow() uintptr {
	var found uintptr
	enumWindows.Call(findTaskbarWindowCallback, uintptr(unsafe.Pointer(&found)))
	return found
}

func taskbarIcon(kind string) uintptr {
	const size = 16
	var pixels [size * size * 4]byte
	var mask [size * 2]byte
	for index := range mask {
		mask[index] = 0xff
	}
	for y := 3; y < 13; y++ {
		for x := 2; x < 14; x++ {
			paint := false
			switch kind {
			case "play":
				paint = x >= 4 && x <= 11 && y >= 3+(x-4)/2 && y <= 12-(x-4)/2
			case "pause":
				paint = (x >= 4 && x <= 6) || (x >= 9 && x <= 11)
			case "previous":
				paint = x <= 3 || (x >= 5 && x <= 12 && y >= 3+(12-x)/2 && y <= 12-(12-x)/2)
			case "next":
				paint = x >= 12 || (x >= 3 && x <= 10 && y >= 3+(x-3)/2 && y <= 12-(x-3)/2)
			}
			if !paint {
				continue
			}
			offset := (y*size + x) * 4
			pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = 255, 255, 255, 255
			mask[y*2+x/8] &^= 1 << (7 - uint(x%8))
		}
	}
	icon, _, _ := createIcon.Call(0, size, size, 1, 32, uintptr(unsafe.Pointer(&mask[0])), uintptr(unsafe.Pointer(&pixels[0])))
	return icon
}

func makeThumbButtons(state taskbarState, icons map[string]uintptr) [3]thumbButton {
	items := [3]struct {
		id        uint32
		icon, tip string
	}{
		{1, "previous", "Предыдущий трек"},
		{2, "play", "Воспроизвести"},
		{3, "next", "Следующий трек"},
	}
	if state.playing {
		items[1].icon, items[1].tip = "pause", "Пауза"
	}
	var buttons [3]thumbButton
	for index, item := range items {
		buttons[index].mask = 0x2 | 0x4 | 0x8
		buttons[index].id = item.id
		buttons[index].icon = icons[item.icon]
		if !state.hasTrack {
			buttons[index].flags = 1
		}
		copy(buttons[index].tip[:], windows.StringToUTF16(item.tip))
	}
	return buttons
}

func runTaskbarControls(ctx context.Context) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		log.Printf("taskbar COM: %v", err)
		return
	}
	defer ole.CoUninitialize()
	object, err := ole.CreateInstance(ole.NewGUID("{56FDF344-FD6D-11d0-958A-006097C9A090}"), ole.NewGUID("{EA1AFB91-9E28-4B86-90E9-9E9F8A5EEFAF}"))
	if err != nil {
		log.Printf("taskbar: %v", err)
		return
	}
	defer object.Release()
	methods := (*[21]uintptr)(unsafe.Pointer(object.RawVTable))
	if result, _, _ := syscall.SyscallN(methods[3], uintptr(unsafe.Pointer(object))); int32(result) < 0 {
		log.Printf("taskbar init: %#x", result)
		return
	}
	icons := map[string]uintptr{"previous": taskbarIcon("previous"), "play": taskbarIcon("play"), "pause": taskbarIcon("pause"), "next": taskbarIcon("next")}
	defer func() {
		for _, icon := range icons {
			if icon != 0 {
				destroyIcon.Call(icon)
			}
		}
	}()
	for _, icon := range icons {
		if icon == 0 {
			log.Print("taskbar: cannot create button icons")
			return
		}
	}
	createdMessage, _, _ := registerWindowMessage.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("TaskbarButtonCreated"))))
	refresh := make(chan struct{}, 1)
	var hwnd, previousProc, callback uintptr
	var state taskbarState
	added := false
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		if hwnd != 0 && previousProc != 0 {
			current, _, _ := getWindowLongPtr.Call(hwnd, ^uintptr(3))
			if current == callback {
				setWindowLongPtr.Call(hwnd, ^uintptr(3), previousProc)
			}
		}
	}()
	update := func() {
		if hwnd == 0 {
			return
		}
		buttons := makeThumbButtons(state, icons)
		method := methods[16]
		if !added {
			method = methods[15]
		}
		result, _, _ := syscall.SyscallN(method, uintptr(unsafe.Pointer(object)), hwnd, uintptr(len(buttons)), uintptr(unsafe.Pointer(&buttons[0])))
		if int32(result) >= 0 {
			added = true
		}
	}
	for {
		select {
		case <-taskbarStop:
			return
		case <-ctx.Done():
			return
		case state = <-taskbarUpdates:
			update()
		case <-refresh:
			added = false
			update()
		case <-ticker.C:
			if hwnd == 0 {
				hwnd = findTaskbarWindow()
				if hwnd == 0 {
					continue
				}
				callback = syscall.NewCallback(func(window, message, wparam, lparam uintptr) uintptr {
					if message == createdMessage {
						select {
						case refresh <- struct{}{}:
						default:
						}
					}
					if message == 0x111 && uint16(wparam>>16) == 0x1800 {
						var command string
						switch uint16(wparam) {
						case 1:
							command = "previous"
						case 2:
							command = "toggle"
						case 3:
							command = "next"
						}
						if command != "" {
							go wailsruntime.EventsEmit(ctx, "desktop:media-command", command)
							return 0
						}
					}
					result, _, _ := callWindowProc.Call(previousProc, window, message, wparam, lparam)
					return result
				})
				previousProc, _, _ = setWindowLongPtr.Call(hwnd, ^uintptr(3), callback)
				if previousProc == 0 {
					hwnd = 0
					continue
				}
			}
			if !added {
				update()
			}
		}
	}
}

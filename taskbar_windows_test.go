//go:build windows

package main

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

func TestTaskbarButtonsMatchWindowsLayoutAndPlaybackState(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Sizeof(thumbButton{}) != 552 {
		t.Fatalf("unexpected Windows THUMBBUTTON size: %d", unsafe.Sizeof(thumbButton{}))
	}
	icons := map[string]uintptr{"previous": 1, "play": 2, "pause": 3, "next": 4}
	paused := makeThumbButtons(taskbarState{hasTrack: true}, icons)
	if paused[1].icon != 2 || windows.UTF16ToString(paused[1].tip[:]) != "Воспроизвести" || paused[0].flags != 0 {
		t.Fatalf("unexpected paused buttons: %+v", paused)
	}
	playing := makeThumbButtons(taskbarState{playing: true, hasTrack: true}, icons)
	if playing[1].icon != 3 || windows.UTF16ToString(playing[1].tip[:]) != "Пауза" {
		t.Fatalf("unexpected playing button: %+v", playing[1])
	}
	empty := makeThumbButtons(taskbarState{}, icons)
	if empty[0].flags != 1 || empty[1].flags != 1 || empty[2].flags != 1 {
		t.Fatal("buttons should be disabled without a track")
	}
}

func TestTaskbarCOMInterfaceCanInitialize(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		t.Fatal(err)
	}
	defer ole.CoUninitialize()
	object, err := ole.CreateInstance(ole.NewGUID("{56FDF344-FD6D-11d0-958A-006097C9A090}"), ole.NewGUID("{EA1AFB91-9E28-4B86-90E9-9E9F8A5EEFAF}"))
	if err != nil {
		t.Fatal(err)
	}
	defer object.Release()
	methods := (*[21]uintptr)(unsafe.Pointer(object.RawVTable))
	result, _, _ := syscall.SyscallN(methods[3], uintptr(unsafe.Pointer(object)))
	if int32(result) < 0 {
		t.Fatalf("ITaskbarList3.HrInit failed: %#x", result)
	}
}

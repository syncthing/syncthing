// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.
package internal

import (
	"errors"
	"syscall"
	"unsafe"

	"github.com/syncthing/syncthing/lib/tray/menu"
	"runtime"
	"sync"
)

// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
// This is a customised version of https://github.com/xilp/systray/blob/master/tray_windows.go
// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!

func (p *Tray) Stop() {
	p.mut.Lock()
	defer p.mut.Unlock()

	if !p.running {
		return
	}

	nid := NOTIFYICONDATA{
		UID:  p.id,
		HWnd: HWND(p.hwnd),
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))

	ret, _, err := Shell_NotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		l.Infoln("shell notify delete failed:", err.Error())
	}
}

func (p *Tray) SetOnDoubleClick(fun func()) {
	p.mut.Lock()
	defer p.mut.Unlock()
	if fun == nil {
		fun = func() {}
	}
	p.dclick = fun
}

func (p *Tray) SetOnLeftClick(fun func()) {
	p.mut.Lock()
	defer p.mut.Unlock()
	if fun == nil {
		fun = func() {}
	}
	p.lclick = fun
}

func (p *Tray) SetOnRightClick(fun func()) {
	p.mut.Lock()
	defer p.mut.Unlock()
	if fun == nil {
		fun = func() {}
	}
	p.rclick = fun
}

func (p *Tray) SetMenuCreationCallback(fun func() []menu.Item) {
	p.mut.Lock()
	defer p.mut.Unlock()
	if fun == nil {
		fun = func() []menu.Item { return nil }
	}
	p.menu = fun
}

func (p *Tray) SetTooltip(tooltip string) error {
	p.mut.Lock()
	defer p.mut.Unlock()
	p.tooltip = tooltip

	if p.running {
		return p.setTooltip(p.tooltip)
	}
	return nil
}

func (p *Tray) setTooltip(tooltip string) error {
	nid := NOTIFYICONDATA{
		UID:  p.id,
		HWnd: HWND(p.hwnd),
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))

	nid.UFlags = NIF_TIP
	copy(nid.SzTip[:], syscall.StringToUTF16(tooltip))

	ret, _, _ := Shell_NotifyIcon.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return errors.New("shell notify tooltip failed")
	}
	return nil
}

func (p *Tray) SetVisible(visible bool) error {
	nid := NOTIFYICONDATA{
		UID:  p.id,
		HWnd: HWND(p.hwnd),
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))

	nid.UFlags = NIF_STATE
	nid.DwStateMask = NIS_HIDDEN
	if !visible {
		nid.DwState = NIS_HIDDEN
	}

	ret, _, _ := Shell_NotifyIcon.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return errors.New("shell notify tooltip failed")
	}
	return nil
}

func (p *Tray) SetIcon(hicon HICON) error {
	nid := NOTIFYICONDATA{
		UID:  p.id,
		HWnd: HWND(p.hwnd),
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))

	nid.UFlags = NIF_ICON
	if hicon == 0 {
		nid.HIcon = 0
	} else {
		nid.HIcon = hicon
	}

	ret, _, _ := Shell_NotifyIcon.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return errors.New("shell notify icon failed")
	}
	return nil
}

func (p *Tray) WinProc(hwnd HWND, msg uint32, wparam, lparam uintptr) uintptr {
	if msg == NotifyIconMessageId {
		if lparam == WM_LBUTTONDBLCLK {
			p.dclick()
		} else if lparam == WM_LBUTTONUP {
			p.lclick()
		} else if lparam == WM_RBUTTONUP {
			p.rclick()
		}
	}
	result, _, _ := DefWindowProc.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
	return result
}

func (p *Tray) Serve() {
	// This whole thing has to run on the same thread, as each thread has it's own UI queue?
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p.mut.Lock()
	err := p.initLocked()
	p.running = true
	p.mut.Unlock()

	if err != nil {
		l.Infoln("tray icon failed:", err.Error())
		return
	}

	hwnd := p.mhwnd
	var msg MSG
	for {
		rt, _, err := GetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int(rt) {
		case 0:
			break
		case -1:
			l.Infoln("tray icon failed:", err.Error())
			break
		}

		is, _, _ := IsDialogMessage.Call(hwnd, uintptr(unsafe.Pointer(&msg)))
		if is == 0 {
			TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			DispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
	p.mut.Lock()
	p.running = false
	p.mut.Unlock()
}

func (p *Tray) ShowMenu() {
	menuItems := p.menu()
	if len(menuItems) == 0 {
		l.Debugln("No menu items, not showing menu")
		return
	}

	point := POINT{}
	if r0, _, err := GetCursorPos.Call(uintptr(unsafe.Pointer(&point))); r0 == 0 {
		l.Infof("failed to get mouse cursor position:", err.Error())
		return
	}

	callbacks := make(map[uintptr]func(), 0)

	menu := buildMenu(menuItems, callbacks)
	if menu == 0 {
		return
	}

	r0, _, err := SetForegroundWindow.Call(p.hwnd)
	if r0 == 0 {
		l.Infof("failed to bring window to foreground:", err.Error())
		return
	}

	r0, _, _ = TrackPopupMenu.Call(menu, TPM_BOTTOMALIGN|TPM_RETURNCMD|TPM_NONOTIFY, uintptr(point.X), uintptr(point.Y), 0, p.hwnd, 0)
	if r0 != 0 {
		if cb, ok := callbacks[r0]; ok && cb != nil {
			cb()
		}
	}
}

func buildMenu(items []menu.Item, callbacks map[uintptr]func()) uintptr {
	dropdown, _, err := CreatePopupMenu.Call()
	if dropdown == 0 {
		l.Infoln("failed to build a menu: ", err.Error())
		return 0
	}

	for _, item := range items {
		id := uintptr(0)
		if item.Type&menu.TypeSubMenu == menu.TypeSubMenu {
			id = buildMenu(item.Children, callbacks)
			if id == 0 {
				return 0
			}
		} else {
			for id = uintptr(1); id <= 9999999; id++ {
				if _, ok := callbacks[id]; !ok {
					break
				}
			}
		}
		callbacks[id] = item.OnClick
		r0, _, err := AppendMenu.Call(dropdown, uintptr(item.Type)|uintptr(item.State), uintptr(id), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(item.Name))))
		if r0 == 0 {
			l.Infoln("failed to append menu item", err.Error())
			return 0
		}
	}
	return dropdown
}

func NewTray() (*Tray, error) {
	return &Tray{
		lclick: func() {},
		rclick: func() {},
		dclick: func() {},
		menu: func() []menu.Item {
			return nil
		},
	}, nil
}

// This needs to run on the same thread as the event loop.
func (ni *Tray) initLocked() error {
	MainClassName := "MainForm"
	registerWindow(MainClassName, ni.WinProc)

	mhwnd, _, _ := CreateWindowEx.Call(
		WS_EX_CONTROLPARENT,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(MainClassName))),
		0,
		WS_OVERLAPPEDWINDOW|WS_CLIPSIBLINGS,
		CW_USEDEFAULT,
		CW_USEDEFAULT,
		CW_USEDEFAULT,
		CW_USEDEFAULT,
		0,
		0,
		0,
		0)
	if mhwnd == 0 {
		return errors.New("create main win failed")
	}

	NotifyIconClassName := "NotifyIconForm"
	registerWindow(NotifyIconClassName, ni.WinProc)

	hwnd, _, _ := CreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(NotifyIconClassName))),
		0,
		0,
		0,
		0,
		0,
		0,
		uintptr(HWND_MESSAGE),
		0,
		0,
		0)
	if hwnd == 0 {
		return errors.New("create notify win failed")
	}

	nid := NOTIFYICONDATA{
		HWnd:             HWND(hwnd),
		UFlags:           NIF_MESSAGE | NIF_STATE,
		DwState:          NIS_HIDDEN,
		DwStateMask:      NIS_HIDDEN,
		UCallbackMessage: NotifyIconMessageId,
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))

	ret, _, _ := Shell_NotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return errors.New("shell notify create failed")
	}

	nid.UVersion = NOTIFYICON_VERSION

	ret, _, _ = Shell_NotifyIcon.Call(NIM_SETVERSION, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		ni.Stop()
		return errors.New("shell notify version failed")
	}

	ni.id = nid.UID
	ni.mhwnd = mhwnd
	ni.hwnd = hwnd

	icon, err := findIcon()
	if err != nil {
		ni.Stop()
		return err
	}

	if err = ni.SetIcon(HICON(icon)); err != nil {
		ni.Stop()
		return err
	}

	if err = ni.SetVisible(true); err != nil {
		ni.Stop()
		return err
	}

	if ni.tooltip != "" {
		if err = ni.setTooltip(ni.tooltip); err != nil {
			ni.Stop()
			return err
		}
	}

	return nil
}

type Tray struct {
	id      uint32
	mhwnd   uintptr
	hwnd    uintptr
	tooltip string
	lclick  func()
	rclick  func()
	dclick  func()
	menu    func() []menu.Item
	running bool
	mut     sync.Mutex
}

func findIcon() (uintptr, error) {
	handle, _, _ := GetModuleHandle.Call(0)
	for i := uintptr(0); i <= 256; i++ {
		hicon, _, _ := LoadIcon.Call(handle, i)
		if hicon != 0 {
			return hicon, nil
		}
	}
	return 0, errors.New("failed to find icon")
}

func registerWindow(name string, proc WindowProc) error {
	hinst, _, _ := GetModuleHandle.Call(0)
	if hinst == 0 {
		return errors.New("get module handle failed")
	}
	hicon, err := findIcon()
	if err != nil {
		return err
	}
	hcursor, _, _ := LoadCursor.Call(0, uintptr(IDC_ARROW))
	if hcursor == 0 {
		return errors.New("load cursor failed")
	}

	var wc WNDCLASSEX
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(proc)
	wc.HInstance = HINSTANCE(hinst)
	wc.HIcon = HICON(hicon)
	wc.HCursor = HCURSOR(hcursor)
	wc.HbrBackground = COLOR_BTNFACE + 1
	wc.LpszClassName = syscall.StringToUTF16Ptr(name)

	atom, _, _ := RegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return errors.New("register class failed")
	}
	return nil
}

type WindowProc func(hwnd HWND, msg uint32, wparam, lparam uintptr) uintptr

type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            HICON
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         GUID
}

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     HINSTANCE
	HIcon         HICON
	HCursor       HCURSOR
	HbrBackground HBRUSH
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       HICON
}

type MSG struct {
	HWnd    HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type POINT struct {
	X, Y int32
}

type (
	HANDLE    uintptr
	HINSTANCE HANDLE
	HCURSOR   HANDLE
	HICON     HANDLE
	HWND      HANDLE
	HGDIOBJ   HANDLE
	HBRUSH    HGDIOBJ
)

const (
	WM_LBUTTONUP     = 0x0202
	WM_LBUTTONDBLCLK = 0x0203
	WM_RBUTTONUP     = 0x0205
	WM_USER          = 0x0400
	WM_TRAYICON      = WM_USER + 69

	WS_EX_APPWINDOW     = 0x00040000
	WS_OVERLAPPEDWINDOW = 0X00000000 | 0X00C00000 | 0X00080000 | 0X00040000 | 0X00020000 | 0X00010000
	CW_USEDEFAULT       = 0x80000000

	NIM_ADD        = 0x00000000
	NIM_MODIFY     = 0x00000001
	NIM_DELETE     = 0x00000002
	NIM_SETVERSION = 0x00000004

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004
	NIF_STATE   = 0x00000008

	NIS_HIDDEN = 0x00000001

	TPM_BOTTOMALIGN = 0x0020
	TPM_RETURNCMD   = 0x0100
	TPM_NONOTIFY    = 0x0080

	IDC_ARROW     = 32512
	COLOR_BTNFACE = 15

	WS_CLIPSIBLINGS     = 0X04000000
	WS_EX_CONTROLPARENT = 0X00010000

	HWND_MESSAGE       = ^HWND(2)
	NOTIFYICON_VERSION = 3

	WM_APP              = 32768
	NotifyIconMessageId = WM_APP + iota
)

var (
	kernel32         = syscall.MustLoadDLL("kernel32")
	GetModuleHandle  = kernel32.MustFindProc("GetModuleHandleW")
	GetConsoleWindow = kernel32.MustFindProc("GetConsoleWindow")
	GetLastError     = kernel32.MustFindProc("GetLastError")

	shell32          = syscall.MustLoadDLL("shell32.dll")
	Shell_NotifyIcon = shell32.MustFindProc("Shell_NotifyIconW")

	user32 = syscall.MustLoadDLL("user32.dll")

	GetMessage       = user32.MustFindProc("GetMessageW")
	IsDialogMessage  = user32.MustFindProc("IsDialogMessageW")
	TranslateMessage = user32.MustFindProc("TranslateMessage")
	DispatchMessage  = user32.MustFindProc("DispatchMessageW")

	ShowWindow          = user32.MustFindProc("ShowWindow")
	UpdateWindow        = user32.MustFindProc("UpdateWindow")
	DefWindowProc       = user32.MustFindProc("DefWindowProcW")
	RegisterClassEx     = user32.MustFindProc("RegisterClassExW")
	GetDesktopWindow    = user32.MustFindProc("GetDesktopWindow")
	CreateWindowEx      = user32.MustFindProc("CreateWindowExW")
	SetForegroundWindow = user32.MustFindProc("SetForegroundWindow")
	GetCursorPos        = user32.MustFindProc("GetCursorPos")

	LoadImage  = user32.MustFindProc("LoadImageW")
	LoadIcon   = user32.MustFindProc("LoadIconW")
	LoadCursor = user32.MustFindProc("LoadCursorW")

	TrackPopupMenu  = user32.MustFindProc("TrackPopupMenu")
	CreatePopupMenu = user32.MustFindProc("CreatePopupMenu")
	AppendMenu      = user32.MustFindProc("AppendMenuW")
)

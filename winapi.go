//go:build windows
// +build windows

// Copyright 2026 workturnedplay
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wincoe

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	//"github.com/workturnedplay/wincoe/internal/wincall"
	"golang.org/x/sys/windows"
	// crap: dot import these so I don't have to prefix them!
	//. "github.com/workturnedplay/wincoe/internal/winconsts"
)

//type Exported struct{}

var (
	Iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
	//procGetExtendedUdpTable = Iphlpapi.NewProc("GetExtendedUdpTable")
	callGetExtendedUdpTable = NewBoundProc(Iphlpapi, "GetExtendedUdpTable", CheckErrno)

	Kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	//procQueryFullProcessName = Kernel32.NewProc("QueryFullProcessImageNameW")
	callQueryFullProcessName = NewBoundProc(Kernel32, "GetExtendedUdpTable", CheckBool)
	// procCreateToolhelp32Snapshot = Kernel32.NewProc("CreateToolhelp32Snapshot")
	callCreateToolhelp32Snapshot = NewBoundProc(Kernel32, "CreateToolhelp32Snapshot", CheckHandle)
	// procProcess32First           = Kernel32.NewProc("Process32FirstW")
	callProcess32First = NewBoundProc(Kernel32, "Process32FirstW", CheckBool)
	// procProcess32Next            = Kernel32.NewProc("Process32NextW")
	callProcess32Next = NewBoundProc(Kernel32, "Process32NextW", CheckBool)
)

const (
	AF_INET             = 2
	UDP_TABLE_OWNER_PID = 1 // MIB_UDPTABLE_OWNER_PID
)

// auto runs before main(), loads the DLLs non-lazily.
func init() {
	loadDll(Kernel32)
	loadDll(Iphlpapi)
}

func loadDll(dll *windows.LazyDLL) {
	err := dll.Load()
	if err != nil {
		panic("critical system dll " + dll.Name + " not found, error: " + err.Error())
	}
}

// GetExtendedUDPTable retrieves the system UDP table using the Windows
// GetExtendedUdpTable API and returns the raw buffer containing the table data.
//
// This is a higher-level wrapper over the low-level bound call
// (callGetExtendedUdpTable). It encapsulates:
//
//   - the two-call pattern required by the API (size query + data fetch)
//   - conversion of Win32 error codes into Go errors via wincall.CheckErrno
//   - handling of ERROR_INSUFFICIENT_BUFFER as part of normal control flow
//
// The returned []byte contains a MIB_UDPTABLE_OWNER_PID (or related) structure,
// depending on the flags used internally. Callers are responsible for parsing
// the buffer according to the expected Windows structure layout.
//
// Guarantees:
//   - returns a non-nil error if the underlying API reports failure
//   - never requires callers to inspect r1 or perform manual error checks
//
// Edge cases handled:
//   - initial size query returning ERROR_INSUFFICIENT_BUFFER
//   - zero-sized buffer responses treated as error
//   - propagation of underlying Windows errors with errors.Is compatibility
//
// Note:
//   - this function intentionally operates on raw bytes to avoid committing
//     to a specific struct layout; build a typed parser on top if needed.
func GetExtendedUDPTable() ([]byte, error) {
	var bufSize uint32

	// First call to GetExtendedUdpTable to get required buffer size.
	_, _, err := callGetExtendedUdpTable(
		0,
		uintptr(unsafe.Pointer(&bufSize)),
		0,
		uintptr(AF_INET),
		uintptr(UDP_TABLE_OWNER_PID),
		0,
	)

	if err != nil && !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return nil, err
	}

	if bufSize == 0 {
		return nil, errors.New("GetExtendedUdpTable returned size 0")
	}

	buf := make([]byte, bufSize)

	_, _, err = callGetExtendedUdpTable(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufSize)),
		0,
		uintptr(AF_INET),
		uintptr(UDP_TABLE_OWNER_PID),
		0,
	)

	if err != nil {
		return nil, err
	}

	return buf, nil
}

// QueryFullProcessName retrieves the full executable path of a process given its PID.
//
// This is a higher-level wrapper over callQueryFullProcessName.
// It encapsulates:
//
//   - opening the process handle with PROCESS_QUERY_LIMITED_INFORMATION
//   - preparing a buffer for the UTF16 path
//   - calling the Windows API
//   - converting UTF16 to Go string and trimming whitespace
//
// Returns a non-empty string and nil error on success, or an empty string with error on failure.
func QueryFullProcessName(pid uint32) (string, error) {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", fmt.Errorf("OpenProcess failed for PID %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	const bufChars = 260
	buf := make([]uint16, bufChars)
	size := uint32(bufChars)

	_, _, err = callQueryFullProcessName(
		uintptr(h),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if err != nil {
		return "", fmt.Errorf("QueryFullProcessNameW failed for PID %d: %w", pid, err)
	}

	path := windows.UTF16ToString(buf[:size])
	return strings.TrimSpace(path), nil
}

// exePathFromPID returns process image path for pid or an error.
// Uses QueryFullProcessImageNameW. May fail if insufficient privilege.
//
// ExePathFromPID retrieves the full executable path of a process by PID.
//
// This is a higher-level wrapper over callQueryFullProcessName.
// It handles buffer sizing and UTF16 conversion.
//
// it's a wrapper-alias around QueryFullProcessName
func ExePathFromPID(pid uint32) (string, error) {
	return QueryFullProcessName(pid)
}

func GetProcessName(pid uint32) (string, error) {
	snapshot, err := CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = Process32First(snapshot, &entry)
	for err == nil {
		//TODO: make a hard limit here, so it doesn't loop infinitely just in case.
		if entry.ProcessID == pid {
			return windows.UTF16ToString(entry.ExeFile[:]), nil
		}
		err = Process32Next(snapshot, &entry)
	}

	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return "", err
	}
	return "", fmt.Errorf("not found, err: %w", err)
}

// // CreateProcessSnapshot wraps callCreateToolhelp32Snapshot.
// func (l *Exported) CreateProcessSnapshot() (windows.Handle, error) {
// 	const TH32CS_SNAPPROCESS = 0x00000002
// 	r1, _, err := callCreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
// 	if err != nil {
// 		return 0, err
// 	}
// 	return windows.Handle(r1), nil
// }

// const TH32CS_SNAPPROCESS = 0x00000002

// CreateToolhelp32Snapshot creates a snapshot of the specified processes, threads,
// modules, or heaps in the system. The snapshot can then be used with functions
// like Process32First/Next or Module32First/Next to enumerate the captured entries.
//
// In short: it’s a system-wide “frozen view” of processes or other kernel objects, enabling safe enumeration without interference from runtime changes.
//
// Parameters:
//
//	flagdwFlagss - a bitmask specifying what to include in the snapshot (e.g., TH32CS_SNAPPROCESS).
//	th32ProcessID   - for some snapshots, a process ID to restrict the snapshot to a particular process. (0 = all processes)
//
// Returns:
//
//	A handle to the snapshot, which must be closed with CloseHandle when done.
//	INVALID_HANDLE_VALUE indicates failure, with GetLastError providing details.
//
// Typical usage:
//
//	hSnap, err := CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
//	if err != nil { ... }
//	defer CloseHandle(hSnap)
//	// enumerate processes with Process32First/Next
//
// Returns a valid windows.Handle on success, or a non-nil error on failure.
//
// Notes:
//
// These flags are bitwise combinable. For example, TH32CS_SNAPPROCESS | TH32CS_SNAPTHREAD captures both processes and threads.
// If a flag isn’t used (e.g., you don’t include TH32CS_SNAPPROCESS), CreateToolhelp32Snapshot will not include that object type in the snapshot.
// TH32CS_SNAPPROCESS specifically tells the API to include all processes in the snapshot. Without it, Process32First/Process32Next won’t enumerate any processes.
func CreateToolhelp32Snapshot(dwFlags, th32ProcessID uint32) (windows.Handle, error) {
	r1, _, err := callCreateToolhelp32Snapshot(
		uintptr(dwFlags),
		uintptr(th32ProcessID),
	)
	if err != nil {
		return 0, err
	}
	return windows.Handle(r1), nil
}

// // CreateProcessSnapshot is a convenience wrapper for creating a snapshot of all processes.
// //
// // Internally calls CreateToolhelp32Snapshot with TH32CS_SNAPPROCESS and PID 0.
// func (l *Exported) CreateProcessSnapshot() (windows.Handle, error) {

// 	return l.CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
// }

// Process32First wraps callProcess32First.
func Process32First(snapshot windows.Handle, entry *windows.ProcessEntry32) error {
	_, _, err := callProcess32First(uintptr(snapshot), uintptr(unsafe.Pointer(entry)))
	return err
}

// Process32Next wraps callProcess32Next.
func Process32Next(snapshot windows.Handle, entry *windows.ProcessEntry32) error {
	_, _, err := callProcess32Next(uintptr(snapshot), uintptr(unsafe.Pointer(entry)))
	return err
}

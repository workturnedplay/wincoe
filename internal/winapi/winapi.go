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

package winapi

import (
	"errors"
	"unsafe"

	"github.com/workturnedplay/wincoe/internal/wincall"
	"golang.org/x/sys/windows"
)

var (
	Iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
	//procGetExtendedUdpTable = Iphlpapi.NewProc("GetExtendedUdpTable")
	callGetExtendedUdpTable = wincall.NewBoundProc(Iphlpapi, "GetExtendedUdpTable", wincall.CheckErrno)

	Kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procQueryFullProcessName = Kernel32.NewProc("QueryFullProcessImageNameW")

	procCreateToolhelp32Snapshot = Kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First           = Kernel32.NewProc("Process32FirstW")
	procProcess32Next            = Kernel32.NewProc("Process32NextW")
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

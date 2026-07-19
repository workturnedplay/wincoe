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
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/windows"
)

// Define the cases we want to cover
var tests = []struct {
	name          string
	isFailure     WinCheckFunc
	r1            uintptr
	callErr       error
	wantErr       bool
	expectIsErr   error // The error we expect errors.Is to find
	expectNoIsErr error // The error we expect errors.Is NOT to find
}{
	{
		name:      "Success Case (r1=1, err=nil)",
		isFailure: CheckBool,
		r1:        1,
		callErr:   nil,
		wantErr:   false,
	},
	{
		name:      "Success Case (r1=1, but callErr has old SUCCESS)",
		isFailure: CheckBool,
		r1:        1,
		callErr:   windows.ERROR_SUCCESS,
		wantErr:   false,
	},
	{
		name:        "Standard Failure (r1=0, Access Denied)",
		isFailure:   CheckBool,
		r1:          0,
		callErr:     windows.ERROR_ACCESS_DENIED,
		wantErr:     true,
		expectIsErr: windows.ERROR_ACCESS_DENIED,
	},
	{
		name:          "Silent Failure (r1=0, callErr=nil)",
		isFailure:     CheckBool,
		r1:            0,
		callErr:       nil,
		wantErr:       true,
		expectNoIsErr: windows.ERROR_SUCCESS, // Should NOT be 'Is' compatible with success
	},
	{
		name:          "Silent Failure (r1=0, callErr=SUCCESS)",
		isFailure:     CheckBool,
		r1:            0,
		callErr:       windows.ERROR_SUCCESS,
		wantErr:       true,
		expectNoIsErr: windows.ERROR_SUCCESS, // Should NOT be 'Is' compatible with success
	},
	{
		name:        "Handle Failure (r1=-1)",
		isFailure:   CheckHandle,
		r1:          ^uintptr(0), // -1
		callErr:     windows.ERROR_INVALID_HANDLE,
		wantErr:     true,
		expectIsErr: windows.ERROR_INVALID_HANDLE,
	},

	{
		name:        "Null Pointer Failure (r1=0)",
		isFailure:   CheckNull,
		r1:          0,
		callErr:     windows.ERROR_OUTOFMEMORY,
		wantErr:     true,
		expectIsErr: windows.ERROR_OUTOFMEMORY,
	},

	{
		name:      "HRESULT Failure (E_FAIL)",
		isFailure: CheckHRESULT,
		r1:        uintptr(0x80004005), // Represents -2147467259 in int32
		callErr:   nil,
		wantErr:   true,
	},
	{
		name:      "HRESULT Success (S_OK)",
		isFailure: CheckHRESULT,
		r1:        0,
		callErr:   nil,
		wantErr:   false,
	},
	{
		name:      "Errno Success (r1=0)",
		isFailure: CheckErrno,
		r1:        0,
		callErr:   nil,
		wantErr:   false,
	},
	{
		name:        "Errno Failure (r1=ERROR_ACCESS_DENIED, callErr=nil)",
		isFailure:   CheckErrno,
		r1:          uintptr(windows.ERROR_ACCESS_DENIED),
		callErr:     nil,
		wantErr:     true,
		expectIsErr: windows.ERROR_ACCESS_DENIED,
	},
	{
		name:        "Errno Failure prefers callErr over r1",
		isFailure:   CheckErrno,
		r1:          uintptr(windows.ERROR_ACCESS_DENIED),
		callErr:     windows.ERROR_INVALID_HANDLE,
		wantErr:     true,
		expectIsErr: windows.ERROR_INVALID_HANDLE,
	},
	{
		name:        "Errno Failure with SUCCESS in callErr falls back to r1",
		isFailure:   CheckErrno,
		r1:          uintptr(windows.ERROR_ACCESS_DENIED),
		callErr:     windows.ERROR_SUCCESS,
		wantErr:     true,
		expectIsErr: windows.ERROR_ACCESS_DENIED,
	},
	{
		name:          "Errno Failure (r1=non-zero but unmapped errno)",
		isFailure:     CheckErrno,
		r1:            123456, // arbitrary unknown code
		callErr:       nil,
		wantErr:       true,
		expectNoIsErr: windows.ERROR_SUCCESS,
	},
	// --- CheckAdjustTokenPrivileges ---
	{
		name:      "AdjustTokenPrivileges Failure (r1=0)",
		isFailure: CheckAdjustTokenPrivileges,
		r1:        0,
		callErr:   windows.ERROR_ACCESS_DENIED,
		wantErr:   true,
	},
	{
		name:        "AdjustTokenPrivileges Partial Failure (ERROR_NOT_ALL_ASSIGNED)",
		isFailure:   CheckAdjustTokenPrivileges,
		r1:          1, // API returns TRUE
		callErr:     windows.ERROR_NOT_ALL_ASSIGNED,
		wantErr:     true,
		expectIsErr: windows.ERROR_NOT_ALL_ASSIGNED,
	},
	{
		name:      "AdjustTokenPrivileges Success",
		isFailure: CheckAdjustTokenPrivileges,
		r1:        1,
		callErr:   windows.ERROR_SUCCESS,
		wantErr:   false,
	},

	// --- CheckZero ---
	{
		name:      "CheckZero Failure (r1=0)",
		isFailure: CheckZero,
		r1:        0,
		wantErr:   true,
	},
	{
		name:      "CheckZero Success (r1=1)",
		isFailure: CheckZero,
		r1:        1,
		wantErr:   false,
	},

	// --- CheckMinusOne ---
	{
		name:      "CheckMinusOne Failure (r1=-1)",
		isFailure: CheckMinusOne,
		r1:        ^uintptr(0),
		wantErr:   true,
	},
	{
		name:      "CheckMinusOne Success (r1=0)",
		isFailure: CheckMinusOne,
		r1:        0,
		wantErr:   false,
	},

	// --- CheckNone ---
	{
		name:      "CheckNone Success (r1=0)",
		isFailure: CheckNone,
		r1:        0,
		wantErr:   false, // Always succeeds
	},
	{
		name:      "CheckNone Success (r1=-1)",
		isFailure: CheckNone,
		r1:        ^uintptr(0),
		wantErr:   false, // Always succeeds
	},

	// --- CheckNTSTATUS ---
	{
		name:      "CheckNTSTATUS Failure (STATUS_ACCESS_DENIED)",
		isFailure: CheckNTSTATUS,
		r1:        uintptr(0xC0000022), // High bit set, evaluates to negative int32
		wantErr:   true,
	},
	{
		name:      "CheckNTSTATUS Success (STATUS_SUCCESS)",
		isFailure: CheckNTSTATUS,
		r1:        0x00000000,
		wantErr:   false,
	},

	// --- CheckThreadPriority ---
	{
		name:      "CheckThreadPriority Failure (THREAD_PRIORITY_ERROR_RETURN)",
		isFailure: CheckThreadPriority,
		r1:        uintptr(THREAD_PRIORITY_ERROR_RETURN),
		wantErr:   true,
	},
	{
		name:      "CheckThreadPriority Success",
		isFailure: CheckThreadPriority,
		r1:        0,
		wantErr:   false,
	},

	// --- CheckCLRInvalid & CheckGDIError ---
	{
		name:      "CheckCLRInvalid Failure",
		isFailure: CheckCLRInvalid,
		r1:        uintptr(CLR_INVALID), // 0xffffffff
		wantErr:   true,
	},
	{
		name:      "CheckGDIError Failure",
		isFailure: CheckGDIError,
		r1:        uintptr(GDIError), // 0xffffffff
		wantErr:   true,
	},

	// --- CheckStringLength ---
	{
		name:      "CheckStringLength Failure (r1=0 with real error)",
		isFailure: CheckStringLength,
		r1:        0,
		callErr:   windows.ERROR_ACCESS_DENIED,
		wantErr:   true,
	},
	{
		name:      "CheckStringLength Success (Empty string, no error)",
		isFailure: CheckStringLength,
		r1:        0,
		callErr:   windows.ERROR_SUCCESS,
		wantErr:   false,
	},
	{
		name:      "CheckStringLength Success (Normal string)",
		isFailure: CheckStringLength,
		r1:        5,
		callErr:   nil,
		wantErr:   false,
	},
}

func TestCheckWinResult(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckWinResult(tt.name, tt.isFailure, tt.r1, tt.callErr)
			failed := err != nil

			// 1. Check if we wanted an error at all
			if (failed) != tt.wantErr {
				t.Errorf("CheckWinResult() error = %v, wantErr %v", err, tt.wantErr)
			}

			// 3. Check for positive matches (errors.Is)
			if tt.expectIsErr != nil {
				if !tt.wantErr {
					t.Errorf("Bad coding: In the tests table, tt.wantErr should be true if tt.expectIsErr is set!")
				}
				if !errors.Is(err, tt.expectIsErr) {
					t.Errorf("Expected error to be %v, but it wasn't", tt.expectIsErr)
				}
			}

			// 4. Check for negative matches (Ensure we didn't wrap SUCCESS)
			if tt.expectNoIsErr != nil {
				if !tt.wantErr {
					t.Errorf("Bad coding: In the tests table, tt.wantErr should be true if tt.expectNoIsErr is set!")
				}
				if errors.Is(err, tt.expectNoIsErr) {
					t.Errorf("Footgun detected: error is incorrectly 'Is' compatible with %v", tt.expectNoIsErr)
				}
			}
		})
	}

	t.Run("Empty operation name keeps it empty", func(t *testing.T) {
		err := CheckWinResult("", CheckBool, 0, windows.ERROR_ACCESS_DENIED)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		msg := err.Error()

		if !strings.Contains(msg, `""`) {
			t.Errorf("unexpected non-empty quoted op name in error: %q", msg)
		}
	})
}

func TestWinCall(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockLazyProc{
				name:    "Mock" + tt.name, // helps debugging
				nextR1:  tt.r1,
				nextR2:  0, // we don't care about r2 in these tests
				nextErr: tt.callErr,
			}

			res1 := WinCall(mock, tt.isFailure)
			if res1.R1 != tt.r1 {
				t.Errorf("Mock wincall was badly coded, r1=%d vs expected tt.r1=%d", res1.R1, tt.r1)
			}

			failed := res1.Failed() //err != nil

			// 1. Check if we wanted an error at all
			if (failed) != tt.wantErr {
				t.Errorf("WinCall() returned err = %v (failed=%v), wantErr %v", res1.Err, failed, tt.wantErr)
			}

			// 3. Check for positive matches (errors.Is)
			if tt.expectIsErr != nil {
				if !tt.wantErr {
					t.Errorf("Bad coding: In the tests table, tt.wantErr should be true if tt.expectIsErr is set!")
				}
				if !errors.Is(res1.Err, tt.expectIsErr) {
					t.Errorf("expected errors.Is(err, %v) to be true, got false", tt.expectIsErr)
				}
				if !res1.ErrIs(tt.expectIsErr) {
					t.Errorf("expected errors.Is(err, %v) to be true, got false", tt.expectIsErr)
				}
			}

			// 4. Check for negative matches (Ensure we didn't wrap SUCCESS)
			if tt.expectNoIsErr != nil {
				if !tt.wantErr {
					t.Errorf("Bad coding: In the tests table, tt.wantErr should be true if tt.expectNoIsErr is set!")
				}
				if errors.Is(res1.Err, tt.expectNoIsErr) {
					t.Errorf("Footgun detected: error is incorrectly 'Is' compatible with %v , in other words: unexpected: errors.Is(err, %v) == true", tt.expectNoIsErr, tt.expectNoIsErr)
				}
				if res1.ErrIs(tt.expectNoIsErr) {
					t.Errorf("Footgun detected: error is incorrectly 'Is' compatible with %v , in other words: unexpected: errors.Is(err, %v) == true", tt.expectNoIsErr, tt.expectNoIsErr)
				}
			}
		})
	} // for
	t.Run("WinCall normalizes empty/whitespace proc names", func(t *testing.T) {
		tests := []struct {
			name     string
			procName string
		}{
			{"empty", ""},
			{"single space", " "},
			{"multiple spaces", "   "},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mock := &mockLazyProc{
					name:    tt.procName,
					nextR1:  0,
					nextErr: windows.ERROR_ACCESS_DENIED,
				}

				res2 := WinCall(mock, CheckBool)
				//if err == nil {
				if res2.Succeeded() {
					t.Fatal("expected error, got nil")
				}
				if !res2.Failed() {
					t.Fatal("expected error, got nil")
				}

				msg := res2.Err.Error()

				// because you're using %q
				expectedPrefix := `"` + UnspecifiedWinApi + `"`

				if !strings.HasPrefix(msg, expectedPrefix) {
					t.Errorf("procName=%q: expected prefix %q, got %q", tt.procName, expectedPrefix, msg)
				}
			})
		}
	})
}

// mockLazyProc is a controllable fake for LazyProcish.
// Used only in unit tests to simulate any (r1, r2, err) combination.
type mockLazyProc struct {
	name     string  // what .Name() returns
	nextR1   uintptr // next value returned by .Call()
	nextR2   uintptr
	nextErr  error // next lastErr from .Call()
	findErr  error
	callArgs []uintptr // optional: record arguments for assertions
}

// Name implements LazyProcish
func (m *mockLazyProc) Name() string {
	return m.name
}

// Call implements LazyProcish
func (m *mockLazyProc) Call(a ...uintptr) (r1, r2 uintptr, lastErr error) {
	m.callArgs = a // record for possible assertions
	return m.nextR1, m.nextR2, m.nextErr
}

// Find implements LazyProcish for testing
func (m *mockLazyProc) Find() error {
	return m.findErr
}

func TestWinResult_Methods(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("wrapped: %w", windows.ERROR_ACCESS_DENIED)

	tests := []struct {
		name          string
		res           WinResult
		wantFailed    bool
		wantSucceeded bool

		errTarget error
		wantErrIs bool

		callStatusTarget error
		wantCallStatusIs bool
	}{
		{
			name:          "success",
			res:           WinResult{},
			wantSucceeded: true,
		},
		{
			name: "wrapped Err",
			res: WinResult{
				Err: wrapped,
			},
			wantFailed: true,
			errTarget:  windows.ERROR_ACCESS_DENIED,
			wantErrIs:  true,
		},
		{
			name: "wrapped CallStatus",
			res: WinResult{
				CallStatus: fmt.Errorf("wrapped: %w", windows.ERROR_ALREADY_EXISTS),
			},
			wantSucceeded:    true,
			callStatusTarget: windows.ERROR_ALREADY_EXISTS,
			wantCallStatusIs: true,
		},
		{
			name: "ErrIs negative",
			res: WinResult{
				Err: fmt.Errorf("wrapped: %w", windows.ERROR_ACCESS_DENIED),
			},
			wantFailed:    true,
			wantSucceeded: false,
			errTarget:     windows.ERROR_INVALID_HANDLE,
			wantErrIs:     false,
		},
		{
			name: "CallStatusIs negative",
			res: WinResult{
				CallStatus: fmt.Errorf("wrapped: %w", windows.ERROR_ALREADY_EXISTS),
			},
			wantFailed:       false,
			wantSucceeded:    true,
			callStatusTarget: windows.ERROR_ACCESS_DENIED,
			wantCallStatusIs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.res.Failed(); got != tt.wantFailed {
				t.Fatalf("Failed()=%v want %v", got, tt.wantFailed)
			}

			if got := tt.res.Succeeded(); got != tt.wantSucceeded {
				t.Fatalf("Succeeded()=%v want %v", got, tt.wantSucceeded)
			}

			if tt.errTarget != nil {
				if got := tt.res.ErrIs(tt.errTarget); got != tt.wantErrIs {
					t.Fatalf("ErrIs()=%v want %v", got, tt.wantErrIs)
				}
			}

			if tt.callStatusTarget != nil {
				if got := tt.res.CallStatusIs(tt.callStatusTarget); got != tt.wantCallStatusIs {
					t.Fatalf("CallStatusIs()=%v want %v", got, tt.wantCallStatusIs)
				}
			}
		})
	}
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	fn()
}

func TestRealProc2_NilDLLPanics(t *testing.T) {
	assertPanics(t, func() {
		RealProc2(nil, "MessageBoxW")
	})
}

func TestWinCall_NilProcPanics(t *testing.T) {
	assertPanics(t, func() {
		WinCall(nil, CheckBool)
	})
}

func TestBoolToUintptr(t *testing.T) {
	tests := []struct {
		in   bool
		want uintptr
	}{
		{false, 0},
		{true, 1},
	}

	for _, tt := range tests {
		if got := boolToUintptr(tt.in); got != tt.want {
			t.Fatalf("boolToUintptr(%v)=%d want %d", tt.in, got, tt.want)
		}
	}
}

func TestRealProc2_EmptyNamePanics(t *testing.T) {
	dll := windows.NewLazySystemDLL("kernel32.dll")

	for _, name := range []string{"", " ", "\t", "\n"} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			assertPanics(t, func() {
				RealProc2(dll, name)
			})
		})
	}
}

func TestNewBoundProc_NilCheckFuncPanics(t *testing.T) {
	dll := windows.NewLazySystemDLL("kernel32.dll")

	assertPanics(t, func() {
		NewBoundProc(dll, "GetCurrentProcessId", nil)
	})
}

func TestCallWithRetry_Success(t *testing.T) {
	calls := 0

	buf, err := callWithRetry("test", 0,
		func(_ *byte, _ *uint32) error {
			calls++
			return nil
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	if buf != nil {
		t.Fatalf("expected nil buffer, got len=%d", len(buf))
	}

	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
}

func TestCallWithRetry_RetryThenSuccess(t *testing.T) {
	calls := 0

	buf, err := callWithRetry("test", 0,
		func(_ *byte, size *uint32) error {
			calls++

			switch calls {
			case 1:
				*size = 64
				return windows.ERROR_MORE_DATA

			case 2:
				*size = 128
				return windows.ERROR_MORE_DATA

			default:
				return nil
			}
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(buf) != 128 {
		t.Fatalf("len(buf)=%d want 128", len(buf))
	}

	if calls != 3 {
		t.Fatalf("calls=%d want 3", calls)
	}
}

func TestCallWithRetry_GenericErrorStopsImmediately(t *testing.T) {
	calls := 0

	_, err := callWithRetry("test", 0,
		func(_ *byte, _ *uint32) error {
			calls++
			return windows.ERROR_ACCESS_DENIED
		},
	)

	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
}

func TestCallWithRetry_MaxRetries(t *testing.T) {
	calls := 0

	_, err := callWithRetry("test", 0,
		func(_ *byte, size *uint32) error {
			calls++
			*size += 64
			return windows.ERROR_INSUFFICIENT_BUFFER
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "max retries") {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls != 10 {
		t.Fatalf("calls=%d want 10", calls)
	}
}

func TestGetLoggerOrFallback_UninitializedAtomic(t *testing.T) {
	var ptr atomic.Pointer[slog.Logger]

	got := GetLoggerOrFallback(&ptr, "test")

	if got == nil {
		t.Fatal("returned nil logger")
	}
}

func TestCheckEquals(t *testing.T) {
	const (
		// Pseudo-handles mimicking the ones in main.go
		CURRENT_PROCESS_PSEUDO_HANDLE = ^uintptr(0)
		CURRENT_THREAD_PSEUDO_HANDLE  = ^uintptr(1)
	)

	checkProcess := CheckEquals(CURRENT_PROCESS_PSEUDO_HANDLE)
	checkThread := CheckEquals(CURRENT_THREAD_PSEUDO_HANDLE)

	tests := []struct {
		name      string
		isFailure WinCheckFunc
		r1        uintptr
		callErr   error
		wantErr   bool
	}{
		{
			name:      "CheckEquals Success (Process Pseudo-Handle)",
			isFailure: checkProcess,
			r1:        CURRENT_PROCESS_PSEUDO_HANDLE,
			wantErr:   false,
		},
		{
			name:      "CheckEquals Failure (Process - Got 0)",
			isFailure: checkProcess,
			r1:        0,
			wantErr:   true,
		},
		{
			name:      "CheckEquals Success (Thread Pseudo-Handle)",
			isFailure: checkThread,
			r1:        CURRENT_THREAD_PSEUDO_HANDLE,
			wantErr:   false,
		},
		{
			name:      "CheckEquals Failure (Thread - Got Process Handle)",
			isFailure: checkThread,
			r1:        CURRENT_PROCESS_PSEUDO_HANDLE,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Leverage the existing CheckWinResult flow to ensure it behaves exactly
			// like it would in production through the WinCall engine.
			err := CheckWinResult(tt.name, tt.isFailure, tt.r1, tt.callErr)
			failed := err != nil

			if failed != tt.wantErr {
				t.Errorf("CheckWinResult() with CheckEquals error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

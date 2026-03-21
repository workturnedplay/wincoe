//go:build windows

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
	"strings"
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

			r1, _, err := WinCall(mock, tt.isFailure)
			if r1 != tt.r1 {
				t.Errorf("Mock wincall was badly coded, r1=%d vs expected tt.r1=%d", r1, tt.r1)
			}

			failed := err != nil

			// 1. Check if we wanted an error at all
			if (failed) != tt.wantErr {
				t.Errorf("WinCall() returned err = %v (failed=%v), wantErr %v", err, failed, tt.wantErr)
			}

			// 3. Check for positive matches (errors.Is)
			if tt.expectIsErr != nil {
				if !tt.wantErr {
					t.Errorf("Bad coding: In the tests table, tt.wantErr should be true if tt.expectIsErr is set!")
				}
				if !errors.Is(err, tt.expectIsErr) {
					t.Errorf("expected errors.Is(err, %v) to be true, got false", tt.expectIsErr)
				}
			}

			// 4. Check for negative matches (Ensure we didn't wrap SUCCESS)
			if tt.expectNoIsErr != nil {
				if !tt.wantErr {
					t.Errorf("Bad coding: In the tests table, tt.wantErr should be true if tt.expectNoIsErr is set!")
				}
				if errors.Is(err, tt.expectNoIsErr) {
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

				_, _, err := WinCall(mock, CheckBool)
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				msg := err.Error()

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
	nextErr  error     // next lastErr from .Call()
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

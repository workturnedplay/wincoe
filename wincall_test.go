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
	"testing"

	"golang.org/x/sys/windows"
)

func TestWinCall(t *testing.T) {
	// Define the cases we want to cover
	tests := []struct {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callbackCalled := false
			onFail := func(err error) {
				callbackCalled = true
			}

			_, _, err := WinCall(tt.isFailure, onFail, tt.r1, 0, tt.callErr)

			// 1. Check if we wanted an error at all
			if (err != nil) != tt.wantErr {
				t.Errorf("WinCall() error = %v, wantErr %v", err, tt.wantErr)
			}

			// 2. If it failed, check the callback
			if tt.wantErr && !callbackCalled {
				t.Errorf("Expected callback to be called on failure, but it wasn't")
			}

			// 3. Check for positive matches (errors.Is)
			if tt.expectIsErr != nil {
				if !errors.Is(err, tt.expectIsErr) {
					t.Errorf("Expected error to be %v, but it wasn't", tt.expectIsErr)
				}
			}

			// 4. Check for negative matches (Ensure we didn't wrap SUCCESS)
			if tt.expectNoIsErr != nil {
				if errors.Is(err, tt.expectNoIsErr) {
					t.Errorf("Footgun detected: error is incorrectly 'Is' compatible with %v", tt.expectNoIsErr)
				}
			}
		})
	}
}
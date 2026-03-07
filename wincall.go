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
"fmt"

	"golang.org/x/sys/windows"
)

type WinCheckFunc func(r1 uintptr) bool

var (
    CheckBool   WinCheckFunc = func(r1 uintptr) bool { return r1 == 0 }
    CheckHandle WinCheckFunc = func(r1 uintptr) bool { return r1 == ^uintptr(0) }
)

// WinCall processes a Windows API result.
// It returns nil on success (when isFailure is false).
// On failure, it returns a wrapped error.
func WinCall(
    isFailure WinCheckFunc,
    onFail func(err error), 
    r1, r2 uintptr,
    callErr error,
) (uintptr, uintptr, error) {

    if isFailure(r1) {
        var finalErr error
        
        // If the system says failure but the error code is 0/nil,
        // we return a concrete error message WITHOUT wrapping ERROR_SUCCESS.
        if callErr == nil || errors.Is(callErr, windows.ERROR_SUCCESS) {
            finalErr = fmt.Errorf("system reported failure (ret=%d) but LastError was 0", r1)
        } else {
            // We only use %w when there is a REAL error to wrap.
            finalErr = fmt.Errorf("WinCall failed: %w", callErr)
        }

        if onFail != nil {
            onFail(finalErr)
        }

        return r1, r2, finalErr
    }

    // Success: return nil so 'if err != nil' behaves normally.
    return r1, r2, nil
}
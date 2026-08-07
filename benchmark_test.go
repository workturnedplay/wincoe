package wincoe

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ============================================================================
// 1. BOUND PROCEDURES & MOCKS (Initialized Once)
// ============================================================================

var (
	procArity0 = NewBoundProc0(Kernel32, "GetCurrentProcessId", CheckNone)
	procArity1 = NewBoundProc1(Kernel32, "SetLastError", CheckNone)
	procArity2 = NewBoundProc2(Kernel32, "WaitForSingleObject", CheckNone)
	procArity3 = NewBoundProc3(Kernel32, "MulDiv", CheckNone)

	// Set CheckNone so we test ONLY pointer escape allocs, NOT error formatting allocs
	// it's technically CheckBool but we wanna avoid 2 more allocs due to CheckWinResult's fmt.Errorf formatted-msg allocs!
	procArity4Peek = NewBoundProc4(Kernel32, "PeekConsoleInputW", CheckNone)

	procArity4Post   = NewBoundProc4(User32, "PostThreadMessageW", CheckNone)
	procArityNReal   = NewBoundProcN(Kernel32, "SetLastError", CheckNone)
	procMockedArityN = &BoundProcN{
		proc:  &mockLazyProc4Bench{name: "MockedAPI"},
		check: CheckNone,
	}
)

// mockLazyProc implements your LazyProcishWrapperForMocksN without touching the OS.
type mockLazyProc4Bench struct{ name string }

func (m *mockLazyProc4Bench) Name() string  { return m.name }
func (m *mockLazyProc4Bench) Addr() uintptr { return 0 }
func (m *mockLazyProc4Bench) Find() error   { return nil }
func (m *mockLazyProc4Bench) Call(_ ...uintptr) (uintptr, uintptr, error) {
	return 1, 0, nil
}

// ============================================================================
// 2. REUSABLE WORKLOAD FUNCTIONS (Documented Single Source of Truth)
// ============================================================================

// GetCurrentProcessId is a fast, user-mode API. Expects 0 allocations.
func runArity0Real() {
	_ = procArity0.Call() //nolint:errcheck // don't care
}

// SetLastError is essentially instantaneous. Expects 0 allocations.
func runArity1Real() {
	procArity1.Call(0) //nolint:errcheck // don't care
}

// WaitForSingleObject(0, 0) fails instantly (WAIT_FAILED) with zero pointers. Expects 0 allocations.
func runArity2Real() {
	procArity2.Call(0, 0) //nolint:errcheck // don't care
}

// MulDiv performs math on three integers. Zero pointers, zero heap allocations.
func runArity3Real() {
	procArity3.Call(10, 20, 30) //nolint:errcheck // don't care
}

// We pass an invalid handle (0) to PeekConsoleInputW so it fails immediately
// before attempting to dereference the pointers, making it safe and fast.
// Takes stack-escaped pointers, so it triggers heap allocations.
func runArity4EscapedPointers() {
	var rec inputRecord
	var count uint32
	procArity4Peek.Call(0, uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&count))) //nolint:errcheck // don't care
}

// PostThreadMessageW takes 4 scalar integer arguments, no pointers.
func runArity4ScalarReal() {
	procArity4Post.Call(0, 0, 0, 0) //nolint:errcheck // don't care
}

// Tests the exact same API as Arity1, but forced through the variadic args... wrapper.
// This allocs the slice (1 alloc) and for each arg 8 bytes (in that same 1 alloc).
func runArityNReal() {
	procArityNReal.Call(0) //nolint:errcheck // don't care
}

// SetLastError(0) -> errno = 0 (0 <= 255) => 0 allocs
func runErrno0() {
	procArity1.Call(0) //nolint:errcheck // don't care
}

// SetLastError(87) -> errno = 87 (ERROR_INVALID_PARAMETER, 87 <= 255) => 0 allocs
func runErrno87() {
	procArity1.Call(87) //nolint:errcheck // don't care
}

// SetLastError(1444) -> errno = 1444 (1444 >= 256) => 1 alloc (8 B/op)
// This completely confirms that Go's runtime uses its static lookup table for integers between 0 and 255.
// Returning error codes within this range costs zero heap allocations.
// Only error codes >= 256 incur an 8-byte heap allocation to box the syscall.Errno integer into the error interface.
func runErrno1444() {
	procArity1.Call(1444) //nolint:errcheck // don't care
}

var tid uintptr

// Success Case: Pass GetCurrentThreadId() => PostThreadMessageW succeeds (errno = 0)
// Result: 0 allocs / 0 B/op
func runArity4Success() {
	procArity4Post.Call(tid, 0, 0, 0) //nolint:errcheck // don't care
}

// Failure Case: Pass 0 => Fails with ERROR_INVALID_THREAD_ID (errno = 1444)
// Result: 1 alloc / 8 B/op
func runArity4Failure() {
	procArity4Post.Call(0, 0, 0, 0) //nolint:errcheck // don't care
}

// Mocked variadic call: 24 bytes, 1 alloc (8 bytes per arg into variadic slice).
func runMockedArityN() {
	procMockedArityN.Call(0, 1, 2) //nolint:errcheck // don't care
}

// ============================================================================
// 3. UNIT TEST: Enforces Allocation Expectations via `testing.AllocsPerRun`
// ============================================================================

func TestSyscallAllocations(t *testing.T) {
	tests := []struct {
		name           string
		workload       func()
		expectedAllocs float64
	}{
		{"Arity0_Real", runArity0Real, 0},
		{"Arity1_Real", runArity1Real, 0},
		{"Arity2_Real", runArity2Real, 0},
		{"Arity3_Real", runArity3Real, 0},
		{"Arity4_EscapedPointersSoItWillAlloc_Real", runArity4EscapedPointers, 2},
		{"Arity4_ScalarReal", runArity4ScalarReal, 1}, // Fails with 1444 (1 alloc)
		{"ArityN_AlwaysAllocs_Real", runArityNReal, 1},
		{"Errno_0", runErrno0, 0},
		{"Errno_87", runErrno87, 0},
		{"Errno_1444", runErrno1444, 1},
		{"Arity4_Success_DoesNoAllocs", runArity4Success, 0},
		{"Arity4_Failure_Does1Alloc", runArity4Failure, 1},
		{"Mocked_ArityN_AlwaysAllocs", runMockedArityN, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			tid = uintptr(windows.GetCurrentThreadId()) // used by runArity4Success
			// AllocsPerRun runs the closure 1000 times after a 1-run warmup
			allocs := testing.AllocsPerRun(1000, tt.workload)
			if allocs != tt.expectedAllocs {
				t.Errorf("%s: got %v allocs/op, want %v", tt.name, allocs, tt.expectedAllocs)
			}
		})
	}
}

// ============================================================================
// 4. BENCHMARKS: Profiling Execution Speed & Throughput
// ============================================================================

func BenchmarkWinCall_Arity0_Real(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runArity0Real()
	}
}

func BenchmarkWinCall_Arity1_Real(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runArity1Real()
	}
}

func BenchmarkWinCall_Arity4_EscapedPointersSoItWillAlloc_Real(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runArity4EscapedPointers()
	}
}

func BenchmarkWinCall_Arity2_Real(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runArity2Real()
	}
}

func BenchmarkWinCall_Arity3_Real(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runArity3Real()
	}
}

func BenchmarkWinCall_Arity4_ScalarReal(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runArity4ScalarReal()
	}
}

func BenchmarkWinCall_ArityN_AlwaysAllocs_Real(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runArityNReal()
	}
}

func BenchmarkWinCall_Errno_0(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runErrno0()
	}
}

func BenchmarkWinCall_Errno_87(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runErrno87()
	}
}

func BenchmarkWinCall_Errno_1444(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runErrno1444()
	}
}

func BenchmarkWinCall_Arity4_Success_DoesNoAllocs(b *testing.B) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tid = uintptr(windows.GetCurrentThreadId()) // used by runArity4Success

	b.ResetTimer()
	for b.Loop() {
		runArity4Success()
	}
}

func BenchmarkWinCall_Arity4_Failure_Does1Alloc(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runArity4Failure()
	}
}

func BenchmarkWinCall_Mocked_ArityN_AlwaysAllocs(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		runMockedArityN()
	}
}

// ============================================================================
// FIRST-CALL ALLOCATION TEST (Verifies Eager Loading)
// ============================================================================

// countFirstCallAllocs creates a FRESH procedure and checks allocations on Call #1.
func countFirstCallAllocs(callFirstTime func()) uint64 {
	runtime.GC() // Clean up any lingering heap noise before measuring

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	callFirstTime() // MUST be the very first time this instance's Call() is run!

	runtime.ReadMemStats(&m2)
	return m2.Mallocs - m1.Mallocs
}

func TestFirstCallHasZeroAllocations(t *testing.T) {
	tests := []struct {
		name        string
		makeAndCall func()
	}{
		{
			name: "Arity0_FirstCall",
			makeAndCall: func() {
				// Construct fresh -> Call() #1
				p := NewBoundProc0(Kernel32, "GetCurrentProcessId", CheckNone)

				// Measure ONLY the .Call() execution, not the constructor
				allocs := countFirstCallAllocs(func() {
					p.Call() //nolint:errcheck // don't care
				})
				if allocs != 0 {
					t.Errorf("First call allocated %d times, expected 0", allocs)
				}
			},
		},
		{
			name: "Arity1_Errno0_FirstCall",
			makeAndCall: func() {
				p := NewBoundProc1(Kernel32, "SetLastError", CheckNone)
				allocs := countFirstCallAllocs(func() {
					p.Call(0) //nolint:errcheck // don't care
				})
				if allocs != 0 {
					t.Errorf("First call allocated %d times, expected 0", allocs)
				}
			},
		},
		// {
		// 	name: "Arity4_Errno0_FirstCall",
		// 	makeAndCall: func() {
		// 		// 1. Lock this goroutine to a specific OS thread
		// 		runtime.LockOSThread()
		// 		defer runtime.UnlockOSThread()

		// 		p := NewBoundProc4(User32, "DefWindowProcW", CheckNone)
		// 		allocs := countFirstCallAllocs(func() {
		// 			p.Call(0, 0, 0, 0) // Always succeeds, 0 allocs, 100% deterministic across threads
		// 		})
		// 		if allocs != 0 {
		// 			t.Errorf("First call allocated %d times, expected 0", allocs)
		// 		}
		// 	},
		// },
		{
			name: "Arity4_Success_FirstCall",
			makeAndCall: func() {
				// 1. Lock this goroutine to a specific OS thread
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()

				p := NewBoundProc4(User32, "PostThreadMessageW", CheckNone)
				tid := uintptr(windows.GetCurrentThreadId())

				// // Force Windows to create the thread's message queue upfront
				// // so Call #1 of p.Call does not fail with errno 1444
				// var msg windows.MSG
				// windows.PeekMessage(&msg, 0, 0, 0, 0)

				// // Force Windows to create the thread's message queue BEFORE measuring Call #1
				// ret1 := p.Call(tid, 0, 0, 0)
				// if ret1.R1 == 0 { //ret.Failed() {
				// 	t.Logf("failed with %d and callStatus:%v", ret1.R1, ret1.CallStatus)
				// }

				var ret2 WinResult
				allocs := countFirstCallAllocs(func() {
					ret2 = p.Call(tid, 0, 0, 0)
					// if ret2.R1 == 0 { //ret.Failed() {
					// 	t.Logf("failed with %d and callStatus:%v", ret2.R1, ret2.CallStatus)
					// }
					// _ = ret2
				})
				if ret2.R1 == 0 {
					t.Logf("%s: Call failed unexpectedly with R1:%d status:%v", t.Name(), ret2.R1, ret2.CallStatus)
				}
				// t.Error("on purpose")

				// // SetRect(lprc, left, top, right, bottom) - User32 scalar call that never fails
				// p := NewBoundProc4(User32, "SetRect", CheckNone)

				// // Pass 0 for rect pointer or valid dummy stack rect
				// var rect windows.Rect
				// allocs := countFirstCallAllocs(func() {
				// 	p.Call(uintptr(unsafe.Pointer(&rect)), 0, 0, 100, 100)
				// })

				if allocs != 0 {
					t.Errorf("First call allocated %d times, expected 0", allocs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			tt.makeAndCall()
		})
	}
}

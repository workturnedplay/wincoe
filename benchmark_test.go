package wincoe

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ============================================================================
// 1. BOUND PROCEDURES & MOCKS (Initialized Once)
// ============================================================================

var (
	procArity0       = NewBoundProc0(Kernel32, "GetCurrentProcessId", CheckNone)
	procArity1       = NewBoundProc1(Kernel32, "SetLastError", CheckNone)
	procArity2       = NewBoundProc2(Kernel32, "WaitForSingleObject", CheckNone)
	procArity3       = NewBoundProc3(Kernel32, "MulDiv", CheckNone)
	procArity4Peek   = NewBoundProc4(Kernel32, "PeekConsoleInputW", CheckBool)
	procArity4Post   = NewBoundProc4(User32, "PostThreadMessageW", CheckNone)
	procArityNReal   = NewBoundProcN(Kernel32, "SetLastError", CheckNone)
	procMockedArityN = &BoundProcN{
		Proc:  &mockLazyProc4Bench{name: "MockedAPI"},
		Check: CheckNone,
	}
)

// mockLazyProc implements your LazyProcishWrapperForMocksN without touching the OS.
type mockLazyProc4Bench struct{ name string }

func (m *mockLazyProc4Bench) Name() string  { return m.name }
func (m *mockLazyProc4Bench) Addr() uintptr { return 0 }
func (m *mockLazyProc4Bench) Find() error   { return nil }
func (m *mockLazyProc4Bench) Call(args ...uintptr) (uintptr, uintptr, error) {
	return 1, 0, nil
}

// ============================================================================
// 2. REUSABLE WORKLOAD FUNCTIONS (Documented Single Source of Truth)
// ============================================================================

// GetCurrentProcessId is a fast, user-mode API. Expects 0 allocations.
func runArity0Real() {
	procArity0.Call()
}

// SetLastError is essentially instantaneous. Expects 0 allocations.
func runArity1Real() {
	procArity1.Call(0)
}

// WaitForSingleObject(0, 0) fails instantly (WAIT_FAILED) with zero pointers. Expects 0 allocations.
func runArity2Real() {
	procArity2.Call(0, 0)
}

// MulDiv performs math on three integers. Zero pointers, zero heap allocations.
func runArity3Real() {
	procArity3.Call(10, 20, 30)
}

// We pass an invalid handle (0) to PeekConsoleInputW so it fails immediately
// before attempting to dereference the pointers, making it safe and fast.
// Takes stack-escaped pointers, so it triggers heap allocations.
func runArity4EscapedPointers() {
	var rec inputRecord
	var count uint32
	procArity4Peek.Call(0, uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&count)))
}

// PostThreadMessageW takes 4 scalar integer arguments, no pointers.
func runArity4ScalarReal() {
	procArity4Post.Call(0, 0, 0, 0)
}

// Tests the exact same API as Arity1, but forced through the variadic args... wrapper.
// This allocs the slice (1 alloc) and for each arg 8 bytes (in that same 1 alloc).
func runArityNReal() {
	procArityNReal.Call(0)
}

// SetLastError(0) -> errno = 0 (0 <= 255) => 0 allocs
func runErrno0() {
	procArity1.Call(0)
}

// SetLastError(87) -> errno = 87 (ERROR_INVALID_PARAMETER, 87 <= 255) => 0 allocs
func runErrno87() {
	procArity1.Call(87)
}

// SetLastError(1444) -> errno = 1444 (1444 >= 256) => 1 alloc (8 B/op)
// This completely confirms that Go's runtime uses its static lookup table for integers between 0 and 255.
// Returning error codes within this range costs zero heap allocations.
// Only error codes >= 256 incur an 8-byte heap allocation to box the syscall.Errno integer into the error interface.
func runErrno1444() {
	procArity1.Call(1444)
}

// Success Case: Pass GetCurrentThreadId() => PostThreadMessageW succeeds (errno = 0)
// Result: 0 allocs / 0 B/op
func runArity4Success() {
	tid := uintptr(windows.GetCurrentThreadId())
	procArity4Post.Call(tid, 0, 0, 0)
}

// Failure Case: Pass 0 => Fails with ERROR_INVALID_THREAD_ID (errno = 1444)
// Result: 1 alloc / 8 B/op
func runArity4Failure() {
	procArity4Post.Call(0, 0, 0, 0)
}

// Mocked variadic call: 24 bytes, 1 alloc (8 bytes per arg into variadic slice).
func runMockedArityN() {
	procMockedArityN.Call(0, 1, 2)
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
		{"Arity4_EscapedPointersSoItWillAlloc_Real", runArity4EscapedPointers, 4},
		{"Arity4_ScalarReal", runArity4ScalarReal, 1},
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

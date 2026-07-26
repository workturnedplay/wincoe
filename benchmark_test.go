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

type mockLazyProc4Bench struct{ name string }

func (m *mockLazyProc4Bench) Name() string  { return m.name }
func (m *mockLazyProc4Bench) Addr() uintptr { return 0 }
func (m *mockLazyProc4Bench) Find() error   { return nil }
func (m *mockLazyProc4Bench) Call(args ...uintptr) (uintptr, uintptr, error) {
	return 1, 0, nil
}

// ============================================================================
// 2. REUSABLE WORKLOAD FUNCTIONS (Single Source of Truth)
// ============================================================================

func runArity0Real() {
	procArity0.Call()
}

func runArity1Real() {
	procArity1.Call(0)
}

func runArity2Real() {
	procArity2.Call(0, 0)
}

func runArity3Real() {
	procArity3.Call(10, 20, 30)
}

func runArity4EscapedPointers() {
	var rec inputRecord
	var count uint32
	procArity4Peek.Call(0, uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&count)))
}

func runArity4ScalarReal() {
	procArity4Post.Call(0, 0, 0, 0)
}

func runArityNReal() {
	procArityNReal.Call(0)
}

func runErrno0() {
	procArity1.Call(0)
}

func runErrno87() {
	procArity1.Call(87)
}

func runErrno1444() {
	procArity1.Call(1444)
}

func runArity4Success() {
	tid := uintptr(windows.GetCurrentThreadId())
	procArity4Post.Call(tid, 0, 0, 0)
}

func runArity4Failure() {
	procArity4Post.Call(0, 0, 0, 0)
}

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
			// AllocsPerRun runs the closure 1000 times after a warmup run
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

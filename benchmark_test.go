package wincoe

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// BenchmarkWinCall_Arity0_Real tests 0 allocations on a 0-arity call.
// GetCurrentProcessId is a fast, user-mode API.
func BenchmarkWinCall_Arity0_Real(b *testing.B) {
	proc := NewBoundProc0(Kernel32, "GetCurrentProcessId", CheckNone)

	b.ResetTimer()
	// for i := 0; i < b.N; i++ {
	for b.Loop() {
		proc.Call()
	}
}

// BenchmarkWinCall_Arity1_Real tests 0 allocations on a 1-arity call.
// SetLastError is essentially instantaneous.
func BenchmarkWinCall_Arity1_Real(b *testing.B) {
	proc := NewBoundProc1(Kernel32, "SetLastError", CheckNone)

	b.ResetTimer()
	// for i := 0; i < b.N; i++ {
	for b.Loop() {
		proc.Call(0)
	}
}

// BenchmarkWinCall_Arity4_Real tests a slightly heavier arity.
// We pass an invalid handle (0) to PeekConsoleInputW so it fails immediately
// before attempting to dereference the pointers, making it safe and fast.
func BenchmarkWinCall_Arity4_EscapedPointersSoItWillAlloc_Real(b *testing.B) {
	proc := NewBoundProc4(Kernel32, "PeekConsoleInputW", CheckBool)
	var rec inputRecord
	var count uint32

	b.ResetTimer()
	// for i := 0; i < b.N; i++ {
	for b.Loop() {
		proc.Call(0, uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&count)))
	}
}

// BenchmarkWinCall_Arity2_Real tests a 2-arity scalar call.
// WaitForSingleObject(0, 0) fails instantly (WAIT_FAILED) with zero pointers.
func BenchmarkWinCall_Arity2_Real(b *testing.B) {
	proc := NewBoundProc2(Kernel32, "WaitForSingleObject", CheckNone)

	b.ResetTimer()
	for b.Loop() {
		proc.Call(0, 0)
	}
}

// BenchmarkWinCall_Arity3_Real tests a 3-arity scalar call.
// MulDiv performs math on three integers. Zero pointers, zero heap allocations.
func BenchmarkWinCall_Arity3_Real(b *testing.B) {
	proc := NewBoundProc3(Kernel32, "MulDiv", CheckNone)

	b.ResetTimer()
	for b.Loop() {
		proc.Call(10, 20, 30)
	}
}

// BenchmarkWinCall_Arity4_ScalarReal tests a 4-arity scalar call.
// PostThreadMessageW takes 4 scalar integer arguments, no pointers.
func BenchmarkWinCall_Arity4_ScalarReal(b *testing.B) {
	proc := NewBoundProc4(User32, "PostThreadMessageW", CheckNone)

	b.ResetTimer()
	for b.Loop() {
		proc.Call(0, 0, 0, 0)
	}
}

// BenchmarkWinCall_ArityN_Real tests the exact same API as Arity1,
// but forced through the variadic args... wrapper.
// this allocs the slice (1 alloc) and for each arg 8 bytes (in that same 1 alloc)
func BenchmarkWinCall_ArityN_AlwaysAllocs_Real(b *testing.B) {
	proc := NewBoundProcN(Kernel32, "SetLastError", CheckNone)

	b.ResetTimer()
	// for i := 0; i < b.N; i++ {
	for b.Loop() {
		proc.Call(0)
	}
}

// 1. SetLastError(0) -> errno = 0 (0 <= 255) => 0 allocs
func BenchmarkWinCall_Errno_0(b *testing.B) {
	proc := NewBoundProc1(Kernel32, "SetLastError", CheckNone)
	b.ResetTimer()
	for b.Loop() {
		proc.Call(0)
	}
}

// 2. SetLastError(87) -> errno = 87 (ERROR_INVALID_PARAMETER, 87 <= 255) => 0 allocs
func BenchmarkWinCall_Errno_87(b *testing.B) {
	proc := NewBoundProc1(Kernel32, "SetLastError", CheckNone)
	b.ResetTimer()
	for b.Loop() {
		proc.Call(87)
	}
}

// 3. SetLastError(1444) -> errno = 1444 (1444 >= 256) => 1 alloc (8 B/op)
// This completely confirms that Go's runtime uses its static lookup table for integers between 0 and 255.
// Returning error codes within this range costs zero heap allocations. Only error codes $\ge 256$ incur an 8-byte heap allocation to box the syscall.Errno integer into the error interface.
func BenchmarkWinCall_Errno_1444(b *testing.B) {
	proc := NewBoundProc1(Kernel32, "SetLastError", CheckNone)
	b.ResetTimer()
	for b.Loop() {
		proc.Call(1444)
	}
}

// Success Case: Pass GetCurrentThreadId() => PostThreadMessageW succeeds (errno = 0)
// Result: 0 allocs / 0 B/op
func BenchmarkWinCall_Arity4_Success_DoesNoAllocs(b *testing.B) {
	proc := NewBoundProc4(User32, "PostThreadMessageW", CheckNone)
	tid := uintptr(windows.GetCurrentThreadId())

	b.ResetTimer()
	for b.Loop() {
		proc.Call(tid, 0, 0, 0)
	}
}

// Failure Case: Pass 0 => Fails with ERROR_INVALID_THREAD_ID (errno = 1444)
// Result: 1 alloc / 8 B/op
func BenchmarkWinCall_Arity4_Failure_Does1Alloc(b *testing.B) {
	proc := NewBoundProc4(User32, "PostThreadMessageW", CheckNone)

	b.ResetTimer()
	for b.Loop() {
		proc.Call(0, 0, 0, 0)
	}
}

// ============================================================================
// MOCKED BENCHMARK (Pure Go Overhead)
// ============================================================================

// mockLazyProc implements your LazyProcishWrapperForMocksN without touching the OS.
type mockLazyProc4Bench struct{ name string }

func (m *mockLazyProc4Bench) Name() string  { return m.name }
func (m *mockLazyProc4Bench) Addr() uintptr { return 0 }
func (m *mockLazyProc4Bench) Find() error   { return nil }
func (m *mockLazyProc4Bench) Call(args ...uintptr) (uintptr, uintptr, error) {
	return 1, 0, nil
}

func BenchmarkWinCall_Mocked_ArityN_AlwaysAllocs(b *testing.B) {
	// Bypass NewBoundProcN to avoid actual DLL loading
	proc := &BoundProcN{
		Proc:  &mockLazyProc4Bench{name: "MockedAPI"},
		Check: CheckNone,
	}

	b.ResetTimer()
	// for i := 0; i < b.N; i++ {
	for b.Loop() {
		//proc.Call(0) //8 bytes 1 alloc
		proc.Call(0, 1, 2) //24 bytes 1 alloc, so 8 bytes per arg!
	}
}

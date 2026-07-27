package wincoe_test

import (
	"testing"
	"unsafe"

	"github.com/workturnedplay/wincoe"
)

// -----------------------------------------------------------------------------
// 1. Single Source of Truth for Test Data
// -----------------------------------------------------------------------------

// makeTestInputs is the single place to change slice size, types, or fields.
func makeTestInputs() []wincoe.KEYANDMOUSE_INPUT {
	return []wincoe.KEYANDMOUSE_INPUT{ // this return makes it be on heap!
		{Type: 1}, // INPUT_KEYBOARD
		{Type: 1},
		{Type: 1},
	}
}

// Global slice initialized ONCE at program startup using the generator.
var globalInputs = makeTestInputs()

// Simulated syscall wrapper with //go:uintptrescapes.
//
//go:uintptrescapes
func dummySyscallCall(count, ptr uintptr) uintptr {
	if count == 0 || ptr == 0 {
		return 0
	}
	return 1
}

// -----------------------------------------------------------------------------
// 2. Tests (go test -v)
// -----------------------------------------------------------------------------

// assertAllocs executes fn 1000 times and strictly verifies the allocation count.
func assertAllocs(t *testing.T, name string, expected float64, fn func()) {
	t.Helper()
	allocs := testing.AllocsPerRun(1000, fn)
	t.Logf("%-12s Allocs/Run: %.2f", name, allocs)
	if allocs != expected {
		t.Errorf("%s: expected %.2f allocs, got %.2f", name, expected, allocs)
	}
}

func TestAllocationDifferences(t *testing.T) {
	// Case 1: Dynamic local slice creation (1 alloc per run)
	assertAllocs(t, "Local Slice", 1.0, func() {
		inputs := makeTestInputs()
		_ = dummySyscallCall(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])))
	})

	// Case 2: Pre-allocated global slice (0 allocs per run)
	assertAllocs(t, "Global Slice", 0.0, func() {
		_ = dummySyscallCall(uintptr(len(globalInputs)), uintptr(unsafe.Pointer(&globalInputs[0])))
	})

	// Case 3: Heap slice pre-allocated BEFORE the call loop (0 allocs per run)
	heapInputs := makeTestInputs()
	assertAllocs(t, "Pre-alloc Heap", 0.0, func() {
		_ = dummySyscallCall(uintptr(len(heapInputs)), uintptr(unsafe.Pointer(&heapInputs[0])))
	})

	// Case 4: Pure local array declared right in the function before the loop (0 allocs per run)
	stackArray := [3]wincoe.KEYANDMOUSE_INPUT{ // so technically on stack, but due to go:uintptrescapes it will actually be on heap from the start!
		{Type: 1},
		{Type: 1},
		{Type: 1},
	}
	assertAllocs(t, "Local Array Pre-alloc", 0.0, func() {
		_ = dummySyscallCall(uintptr(len(stackArray)), uintptr(unsafe.Pointer(&stackArray[0])))
	})
}

// -----------------------------------------------------------------------------
// 3. Benchmarks (go test -bench=. -benchmem)
// -----------------------------------------------------------------------------

func BenchmarkLocalSliceAllocation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		inputs := makeTestInputs()
		dummySyscallCall(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])))
	}
}

func BenchmarkGlobalSliceAllocation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		dummySyscallCall(uintptr(len(globalInputs)), uintptr(unsafe.Pointer(&globalInputs[0])))
	}
}

func BenchmarkPreallocatedHeapSliceAllocation(b *testing.B) {
	b.ReportAllocs()
	heapInputs := makeTestInputs() // Allocated on heap before timer resets
	b.ResetTimer()

	for b.Loop() {
		dummySyscallCall(uintptr(len(heapInputs)), uintptr(unsafe.Pointer(&heapInputs[0])))
	}
}

func BenchmarkLocalArrayPreallocated(b *testing.B) {
	b.ReportAllocs()

	// Looks like a normal local variable on the stack!
	stackArray := [3]wincoe.KEYANDMOUSE_INPUT{
		{Type: 1},
		{Type: 1},
		{Type: 1},
	}
	b.ResetTimer()

	for b.Loop() {
		dummySyscallCall(uintptr(len(stackArray)), uintptr(unsafe.Pointer(&stackArray[0])))
	}
}

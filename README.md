# xxcond

![ci](https://github.com/yuchanns/xxcond/actions/workflows/ci.yaml/badge.svg?branch=main)
[![Go Reference](https://pkg.go.dev/badge/go.yuchanns.xyz/xxcond)](https://pkg.go.dev/go.yuchanns.xyz/xxcond)
[![](https://badge.fury.io/go/go.yuchanns.xyz%2Fxxcond.svg)](https://pkg.go.dev/go.yuchanns.xyz/xxcond)
[![License](https://img.shields.io/github/license/yuchanns/xxcond)](https://github.com/yuchanns/xxcond/blob/main/LICENSE)


xxcond provides a lightweight, garbage-collection-reduced condition variable for Go, operating on pre-allocated memory regions chosen by the user. It is designed for scenarios like manual memory management, embedded systems, or custom memory pools, where minimizing Go runtime allocations is desirable.

Unlike sync.Cond, xxcond.Cond does not support multiple concurrent waiters or broadcast signaling. Only a single goroutine may wait on a condition at a time, and signaling is strictly one-to-one. Internally, it uses a minimal Go channel for wakeup, so it is not strictly GC-free.

This library is suitable for applications where memory locality, low GC pressure, and manual memory lifecycle management are preferred, while still requiring basic condition variable synchronization between goroutines.

## Features

- **Low garbage collection pressure**: Uses user-provided, pre-allocated memory to store the Cond structure, reducing allocations via the Go garbage collector.
- **Thread-safe**: Internal spin-lock ensures mutual exclusion for locking and signaling; safe for use between goroutines.
- **Minimal runtime memory**: Although designed to avoid GC overhead, internal synchronization uses a Go `chan`, so some runtime-managed memory is still involved.
- **Suited for manual memory scenarios**: Useful for embedded systems, custom memory pools, or arena allocations where memory control is essential.
- **Simple API**: Provides Lock, Unlock, Wait, and Signal operations, but only supports a single waiter at a time.

## Usage

### Basic Example
```go
package main

import (
    "unsafe"
    "go.yuchanns.xyz/xxcond"
)

func main() {
    // Calculate required memory size
    size := xxcond.Sizeof()
    buf := make([]byte, size)

    // Initialize condition variable using user-allocated memory
    cond := xxcond.Make(unsafe.Pointer(&buf[0]))

    // Example: one goroutine waits; another signals
    go func() {
        cond.Lock()
        defer cond.Unlock()
        cond.Signal() // wake up waiter
    }()

    cond.Lock()
    cond.Wait() // wait for signal
    cond.Unlock()
}
```

### Manual Memory Management

You may allocate the required buffer from pools, arenas, or on the stack. Ensure that the buffer remains valid for the lifetime of the condition variable instance.

```go
var buf [64]byte // size must be sufficient for xxcond.Sizeof()
cond := xxcond.Make(unsafe.Pointer(&buf[0]))
```

## Memory Management & Thread Safety

- The user is responsible for allocating a buffer of sufficient size and keeping that memory valid for the entire lifetime of the condition variable.
- xxcond does not autonomously allocate or deallocate any internal memory managed by Go's runtime, except for a minimal internal use of Go `chan`. Thus, some portion is still under Go's memory management.
- All core operations are thread-safe, using atomic spin-lock for mutual exclusion and signaling. However, only one goroutine may Wait at a time; concurrent waiters will panic.
- Cond instance must not be copied or passed by value after initialization; always use the pointer.

## License

Licensed under the Apache License, Version 2.0.

## Contributing

Contributions are welcome. Please ensure that any changes maintain the thread safety and minimal GC pressure properties.
- Suitable for memory pool, arena allocation, embedded system manual memory control scenarios.

## Limitations

- Only one goroutine may wait at a time (single waiter only). Multiple concurrent waiters will panic.
- Does not support broadcast signaling.
- Not a drop-in replacement for sync.Cond; interface and concurrency model differ.
- Internal implementation still relies on Go's `chan`, so the structure is not completely GC-free or runtime-memory-free.
- Requires manual allocation and management of the underlying memory buffer.
- Cond instance must not be copied or passed by value after initialization; always use the pointer.


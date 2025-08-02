// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Package xxcond provides a garbage collection-free condition variable implementation
// that operates on user-allocated memory blocks.
package xxcond

import (
	"runtime"
	"sync/atomic"
	"unsafe"

	"go.yuchanns.xyz/xxchan"
)

// Cond is a lightweight, thread-safe condition variable designed for use with
// manually managed memory (such as stack, memory pool, or arena allocations).
// Unlike Go's primitive sync.Cond, xxcond.Cond does not allocate internal memory
// managed by the Go garbage collector, making it suitable for systems or embedded
// scenarios where GC overhead is undesirable.
//
// Internally, Cond uses a lock and a minimal xxchan.Channel (capacity 1) to
// handle the single waiter (goroutine) waiting for a signal. Only one goroutine
// may wait at a time, and concurrent waiting is NOT supported.
//
// Usage example:
//
//	// Calculate buffer size and allocate memory
//	size := xxcond.Sizeof()
//	buf := make([]byte, size)
//	c := xxcond.Make(unsafe.Pointer(&buf[0]))
//
//	go func() {
//	    // ...
//	    c.Signal() // wake the waiter
//	}()
//	c.Lock()
//	c.Wait()
//	// ...
//	c.Unlock()
//
// Safety:
// - Only one goroutine may Wait at a time. Multiple concurrent waiters will panic.
// - The provided memory buffer must remain valid for the lifetime of Cond.
type Cond struct {
	l int32

	waiter *xxchan.Channel[chan struct{}]
}

// alignUp rounds n up to the closest multiple of align.
// Used to ensure proper struct and buffer alignment within the allocated memory block.
func alignUp(n, align int) int {
	return (n + align - 1) &^ (align - 1)
}

// Sizeof computes the memory size, in bytes, required to allocate a Cond structure and its internal signaling channel.
//
// Returns:
//   - Total size in bytes for buffer allocation (must be provided when calling Make).
//
// Use this value to allocate a byte buffer large enough to host a Cond instance.
func Sizeof() int {
	var c *Cond
	rawSize := int(unsafe.Sizeof(*c)) + xxchan.Sizeof[chan struct{}](1)
	return alignUp(rawSize, int(unsafe.Alignof(make(chan struct{}))))
}

// Make initializes a new Cond instance using a user-allocated memory block.
//
// Parameters:
//   - ptr: Pointer to a buffer of at least Sizeof() bytes, properly aligned.
//
// Returns:
//   - Pointer to Cond in the provided memory block. The memory must remain valid for the lifetime of Cond.
//
// Safety:
//   - User must provide an adequately sized and properly aligned buffer.
//   - Cond instance must not be copied or passed by value after initialization. Always use by pointer.
func Make(ptr unsafe.Pointer) *Cond {
	c := (*Cond)(ptr)
	c.l = 0
	c.waiter = xxchan.Make[chan struct{}](unsafe.Pointer(uintptr(ptr)+unsafe.Sizeof(*c)), 1)
	return c
}

// assert panics if the receiver is nil; internal error checking.
func (c *Cond) assert() {
	if c == nil {
		panic("xxcond: nil Cond")
	}
}

// Lock acquires the internal spin-lock. Blocks until successful. Not reentrant.
//
// Safety:
//   - Must be paired with Unlock. Not for use with defer (manual is recommended).
//   - Only one goroutine may hold the lock at a time.
func (c *Cond) Lock() {
	c.assert()
	for !atomic.CompareAndSwapInt32(&c.l, 0, 1) {
		runtime.Gosched()
	}
}

// Unlock releases the internal spin-lock.
func (c *Cond) Unlock() {
	c.assert()
	atomic.StoreInt32(&c.l, 0)
}

// Wait releases the lock and blocks until another goroutine signals.
// Only one goroutine may Wait at a time; concurrent Wait calls panic.
//
// Usage:
//   - Caller must hold the lock before calling Wait.
//   - Wait atomically unlocks, blocks, then locks again before returning.
//
// Returns:
//   - ok: true if the wait was successful (will blocks until notified), false indicates an immediate return without waiting.
//
// Safety:
//   - Only a single waiter is supported. Multiple waiters will panic.
func (c *Cond) Wait() (ok bool) {
	c.assert()
	if c.waiter.Len() > 0 {
		panic("xxcond: multiple waiters are not supported")
	}
	c.Unlock()
	done := make(chan struct{})
	ok = c.waiter.Push(done)
	if ok {
		<-done
		c.Lock()
	}
	return
}

// Signal wakes the current waiter, if one exists.
// If no goroutine is waiting, Signal is a no-op.
//
// Usage:
//   - Caller should hold the lock when calling Signal.
func (c *Cond) Signal() {
	c.assert()
	done, ok := c.waiter.Pop()
	if !ok {
		return
	}
	close(done)
}

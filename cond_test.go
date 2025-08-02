// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package xxcond_test

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smasher164/mem"
	"github.com/stretchr/testify/require"
	"go.yuchanns.xyz/xxcond"
)

func TestCond(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	t.Cleanup(func() {
		mem.Free(ptr)
	})

	c := xxcond.Make(ptr)
	assert.NotNil(c)

	go func() {
		time.Sleep(time.Microsecond)
		c.Signal()
	}()

	c.Wait()
}

func TestLockUnlock(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	defer mem.Free(ptr)

	c := xxcond.Make(ptr)
	assert.NotNil(c)

	// Test basic lock/unlock
	c.Lock()
	c.Unlock()

	// Test that lock prevents concurrent access
	var counter int32
	var wg sync.WaitGroup

	numGoroutines := 10
	incrementsPerGoroutine := 100

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				c.Lock()
				temp := atomic.LoadInt32(&counter)
				runtime.Gosched() // Encourage race conditions
				atomic.StoreInt32(&counter, temp+1)
				c.Unlock()
			}
		}()
	}

	wg.Wait()
	expected := int32(numGoroutines * incrementsPerGoroutine)
	assert.Equal(expected, atomic.LoadInt32(&counter))
}

func TestWaitSignal(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	defer mem.Free(ptr)

	c := xxcond.Make(ptr)
	assert.NotNil(c)

	var ready bool
	var mu sync.Mutex

	// Test single waiter
	go func() {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		ready = true
		mu.Unlock()
		c.Signal()
	}()

	c.Lock()
	for !func() bool {
		mu.Lock()
		defer mu.Unlock()
		return ready
	}() {
		c.Wait()
	}
	c.Unlock()

	mu.Lock()
	assert.True(ready)
	mu.Unlock()
}

func TestSignalWithoutWaiters(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	defer mem.Free(ptr)

	c := xxcond.Make(ptr)
	assert.NotNil(c)

	// Signal without any waiters should not panic or block
	c.Signal()
	c.Signal()
	c.Signal()

	// Should still be able to use the condition variable normally
	done := make(chan bool)
	go func() {
		time.Sleep(50 * time.Millisecond)
		c.Signal()
		done <- true
	}()

	c.Lock()
	c.Wait()
	c.Unlock()

	<-done
}

func TestConcurrentLockContention(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	defer mem.Free(ptr)

	c := xxcond.Make(ptr)
	assert.NotNil(c)

	numGoroutines := 50
	var wg sync.WaitGroup
	var successCount int32

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			c.Lock()
			// Hold lock briefly
			time.Sleep(time.Microsecond)
			atomic.AddInt32(&successCount, 1)
			c.Unlock()
		}()
	}

	wg.Wait()
	assert.Equal(int32(numGoroutines), atomic.LoadInt32(&successCount))
}

func TestWaitAfterSignal(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	defer mem.Free(ptr)

	c := xxcond.Make(ptr)
	assert.NotNil(c)

	// Signal first, then wait - should block until next signal
	c.Signal()

	signaled := make(chan bool)
	go func() {
		time.Sleep(100 * time.Millisecond)
		c.Signal()
		signaled <- true
	}()

	start := time.Now()
	c.Lock()
	c.Wait()
	c.Unlock()
	elapsed := time.Since(start)

	<-signaled
	assert.Greater(elapsed, 50*time.Millisecond, "Wait should have blocked")
}

func TestRapidSignaling(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	defer mem.Free(ptr)

	c := xxcond.Make(ptr)
	assert.NotNil(c)

	var waitCount int32
	var wg sync.WaitGroup

	// Start a waiter
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 10 {
			c.Lock()
			c.Wait()
			atomic.AddInt32(&waitCount, 1)
			c.Unlock()
		}
	}()

	// Signal rapidly
	time.Sleep(10 * time.Millisecond) // Let waiter start
	for range 10 {
		c.Signal()
		time.Sleep(time.Millisecond) // Brief pause between signals
	}

	wg.Wait()
	assert.Equal(int32(10), atomic.LoadInt32(&waitCount))
}

func BenchmarkLockUnlock(b *testing.B) {
	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	defer mem.Free(ptr)

	c := xxcond.Make(ptr)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Lock()
			c.Unlock()
		}
	})
}

func BenchmarkWaitSignal(b *testing.B) {
	ptr := mem.Alloc(uint(xxcond.Sizeof()))
	defer mem.Free(ptr)

	c := xxcond.Make(ptr)

	for b.Loop() {
		go func() {
			c.Lock()
			c.Wait()
			c.Unlock()
		}()

		// Brief pause to let waiter start
		runtime.Gosched()
		c.Signal()
	}
}

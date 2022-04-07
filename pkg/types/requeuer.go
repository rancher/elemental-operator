/*
Copyright © 2022 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import "golang.org/x/sync/semaphore"

// Requeuer handle requeues/enqueues into a channel
// to allow job syncronization.
// There might be different implementations
// due to how the blocking aspect is handled.
type Requeuer interface {
	Requeue()
	Dequeue() <-chan interface{}
}

type bufferedRequeuer struct {
	c chan interface{}
}

func (b *bufferedRequeuer) Requeue() {
	b.c <- nil
}

func (b *bufferedRequeuer) Dequeue() <-chan interface{} {
	return b.c
}

type concurrentRequeuer struct {
	c   chan interface{}
	sem *semaphore.Weighted
}

func (b *concurrentRequeuer) Requeue() {
	if b.sem.TryAcquire(1) {
		go func() {
			defer b.sem.Release(1)
			b.c <- nil
		}()
	}
}

func (b *concurrentRequeuer) Dequeue() <-chan interface{} {
	return b.c
}

// BufferedRequeuer returns a static buffered requeuer of a fixed size.
// After the number of sync requests overflow the size the Requeue() calls will be blocking
func BufferedRequeuer(size int) Requeuer {
	return &bufferedRequeuer{c: make(chan interface{}, size)}
}

// ConcurrentRequeuer returns a dynamic requeuer of a maximum size.
// Requeue() will never be blocking, but when the maximum size is crossed we stop
// enqueueing
func ConcurrentRequeuer(max int) Requeuer {
	return &concurrentRequeuer{c: make(chan interface{}), sem: semaphore.NewWeighted(int64(max))}
}

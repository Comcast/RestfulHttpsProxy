/*
Copyright 2019 Comcast Cable Communications Management, LLC

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

package util

import (
	"io"
	"sync"
)

/*-------------------------------------------------*/

type bufferLoader struct {
	buffers  []*buffer
	source   io.ReadCloser
	m        sync.Mutex
	err      error
	closeErr error
	closed   bool
}

func (b *bufferLoader) close() error {
	if !b.closed {
		b.closeErr = b.source.Close()
		b.closed = true
	}
	return b.closeErr
}

func (b *bufferLoader) read(i int) {
	p := make([]byte, i)
	if b.err != nil {
		return
	}
	n, err := b.source.Read(p)
	b.err = err
	p = p[:n]
	for _, buffer := range b.buffers {
		buffer.Write(p)
	}
}

/*-------------------------------------------------*/

type buffer struct {
	Buffer DynamicCircularBuffer
	Source *bufferLoader
}

func (b *buffer) Read(p []byte) (int, error) {
	b.Source.m.Lock()
	defer b.Source.m.Unlock()

	if b.Buffer.Len() < len(p) {
		b.Source.read(len(p) - b.Buffer.Len())
		n, _ := b.Buffer.Read(p)
		return n, b.Source.err
	}

	n, err := b.Buffer.Read(p)

	return n, err
}

func (b *buffer) Write(p []byte) (n int, err error) {
	return b.Buffer.Write(p)
}

func (b *buffer) Close() error {
	b.Source.m.Lock()
	defer b.Source.m.Unlock()
	b.Buffer = DynamicCircularBuffer{}
	return b.Source.close()
}

/*-------------------------------------------------*/
/*
	BufferedSplit(source ReadCloser, number of ways to split) array of derived ReadClosers
*/
func BufferedSplit(rc io.ReadCloser, n int) []io.ReadCloser {
	s := bufferLoader{
		buffers: make([]*buffer, n),
		source:  rc,
		m:       sync.Mutex{},
		err:     nil,
	}
	result := make([]io.ReadCloser, n)
	for i := 0; i < n; i++ {
		b := buffer{
			Buffer: DynamicCircularBuffer{},
			Source: &s,
		}
		result[i] = &b
		s.buffers[i] = &b
	}

	return result
}

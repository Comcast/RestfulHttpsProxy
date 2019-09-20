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

package rewriteLogic

import (
	"fmt"
	"io"
	"regexp"
)

type regexReader struct {
	chunkReader fixedChunkRegexReader
	chunk       []byte
	err         error
}

func RegexReader(data io.Reader, bufferSize int, find *regexp.Regexp, replace []byte) *regexReader {
	var r regexReader
	r.chunkReader.bufferSize = bufferSize
	r.chunkReader.dataSource = data
	r.chunkReader.find = find
	r.chunkReader.replace = replace
	return &r
}

func (r *regexReader) Read(p []byte) (int, error) {
	totalCopied := 0
	for {
		if p == nil || len(p) == 0 {
			break
		}
		if r.err == nil && (r.chunk == nil || len(r.chunk) == 0) {
			r.chunk, r.err = r.chunkReader.Read()
		}
		if len(r.chunk) == 0 {
			return totalCopied, r.err
		}
		numCopied := 0
		if len(p) > len(r.chunk) {
			numCopied = len(r.chunk)
		} else {
			numCopied = len(p)
		}
		copy(p, r.chunk)
		p = p[numCopied:]
		r.chunk = r.chunk[numCopied:]
		totalCopied += numCopied
	}
	return totalCopied, nil
}

type fixedChunkRegexReader struct {
	dataSource io.Reader
	find       *regexp.Regexp
	replace    []byte

	buffer []byte

	rightIndex int
	leftIndex  int
	midIndex   int

	bufferSize int
}

func FixedChunkRegexReader(reader io.Reader, bufferSize int, find *regexp.Regexp, replace []byte) *fixedChunkRegexReader {
	var crr fixedChunkRegexReader
	crr.dataSource = reader
	crr.find = find
	crr.replace = replace
	crr.bufferSize = bufferSize
	fmt.Println("created chunked regex reader")
	return &crr
}

func (r *fixedChunkRegexReader) Read() ([]byte, error) {
	if r.buffer == nil {
		r.buffer = make([]byte, r.bufferSize*2)

		r.leftIndex = 0
		r.midIndex = r.bufferSize
		r.rightIndex = r.bufferSize * 2

		n, err := readUntilFull(
			r.buffer,
			r.dataSource,
		)
		if err != nil {
			buffer := r.buffer[:n]
			buffer = r.find.ReplaceAll(buffer, r.replace)
			return buffer, err
		}

		buffer := r.buffer
		buffer = r.find.ReplaceAll(buffer, r.replace)
		r.rightIndex = len(buffer)
		r.midIndex = r.rightIndex / 2
		r.buffer = make([]byte, len(buffer))
		copy(r.buffer, buffer)
		return buffer[:r.midIndex], err
	}
	rightBufferSize := r.rightIndex - r.midIndex
	buffer := make([]byte, rightBufferSize+r.bufferSize)
	copy(buffer, r.buffer[r.midIndex:])
	r.midIndex = rightBufferSize
	r.rightIndex = len(buffer)
	r.buffer = buffer
	n, err := readUntilFull(
		r.buffer[r.midIndex:],
		r.dataSource,
	)
	if err != nil {
		buffer = r.buffer[:r.midIndex+n]
		buffer = r.find.ReplaceAll(buffer, r.replace)
		return buffer, err
	}

	buffer = r.find.ReplaceAll(r.buffer, r.replace)
	r.rightIndex = len(buffer)
	r.midIndex = (r.rightIndex) / 2

	copy(r.buffer, buffer)

	return r.buffer[:r.midIndex], nil
}

func readUntilFull(buffer []byte, reader io.Reader) (int, error) {
	totalRead := 0
	for {
		numRead, err := reader.Read(buffer[totalRead:])
		totalRead += numRead
		if err != nil {
			return totalRead, err
		}
		if totalRead >= len(buffer) {
			break
		}
	}
	return totalRead, nil
}

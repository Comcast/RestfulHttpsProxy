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
	//"bytes"
	//"errors"
	"io"
	//"sync"
	//"log"
	"strconv"
)

type DynamicCircularBuffer struct {
	start  int
	length int

	data []byte
}

func (dcb *DynamicCircularBuffer) String() string {
	dcb.initIfNeeded()
	end := (dcb.start + dcb.length) % len(dcb.data)
	if end == 0 && dcb.length != 0 {
		end = len(dcb.data)
	}
	dataVisual := ""
	if end <= dcb.start && dcb.length != 0 {
		dataVisual += string(dcb.data[:end]) + ">" + string(dcb.data[end:dcb.start]) + "<" + string(dcb.data[dcb.start:])
	} else {
		dataVisual += string(dcb.data[:dcb.start]) + "<" + string(dcb.data[dcb.start:end]) + ">" + string(dcb.data[end:])
	}
	return "" +
		"[start: " + strconv.Itoa(dcb.start) + "]" +
		"[length: " + strconv.Itoa(dcb.length) + "]" +
		"[capacity: " + strconv.Itoa(len(dcb.data)) + "]" +
		"[data: " + dataVisual + "]"
}

// func (dcb *DynamicCircularBuffer) Len() int {
// 	if dcb.start > dcb.end {
// 		return len(dcb.data) - dcb.start + dcb.end
// 	}
// 	return dcb.end - dcb.start
// }

func (dcb *DynamicCircularBuffer) Len() int {
	return dcb.length
}

func (dcb *DynamicCircularBuffer) initIfNeeded() {
	if dcb.data == nil {
		dcb.data = make([]byte, 1)
	}
}

func (dcb *DynamicCircularBuffer) freeSpace() (n int) {
	return len(dcb.data) - dcb.length
}

func (dcb *DynamicCircularBuffer) changeCapacity(newSize int) {
	if newSize < 1 {
		newSize = 1
	}
	newData := make([]byte, newSize)
	// for i := range newData {
	// 	newData[i] = '_'
	// }
	end := dcb.start + dcb.length
	if end > len(dcb.data) {
		n := copy(newData, dcb.data[dcb.start:])
		copy(newData[n:], dcb.data[:dcb.length-n])
	} else {
		copy(newData, dcb.data[dcb.start:end])
	}
	dcb.data = newData
	dcb.start = 0
}

func (dcb *DynamicCircularBuffer) Write(p []byte) (n int, err error) {
	dcb.initIfNeeded()
	numToWrite := len(p)
	n = 0
	if dcb.freeSpace() < numToWrite {
		dcb.changeCapacity((dcb.length + numToWrite) * 2)
	}
	end := (dcb.start + dcb.length) % len(dcb.data)
	//log.Println(dcb.String())
	n = copy(dcb.data[end:], p)
	n += copy(dcb.data, p[n:])
	dcb.length += n

	return n, err
}

func (dcb *DynamicCircularBuffer) Read(p []byte) (n int, err error) {
	dcb.initIfNeeded()

	numToRead := len(p)
	n = 0

	if numToRead >= dcb.length {
		numToRead = dcb.length
		p = p[:numToRead]
		err = io.EOF
	}
	//end := dcb.start + numToRead
	// if end > len(dcb.data) {
	// 	n = copy(p, dcb.data[dcb.start:])
	// 	n += copy(p[n:], dcb.data[:dcb.length-n])
	// } else {
	// 	n = copy(p, dcb.data[dcb.start:end])
	// }
	n = copy(p, dcb.data[dcb.start:])
	n += copy(p[n:], dcb.data)
	dcb.length -= n
	dcb.start += n
	dcb.start %= len(dcb.data)

	if dcb.length*4 < len(dcb.data) {
		dcb.changeCapacity(dcb.length * 2)
	}

	return n, err
}

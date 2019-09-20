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

package throttle

import (
	"io"
	//"math/rand"
	//"log"
	"sync"
	"time"
)

const Kilobit = 1000
const Megabit = Kilobit * 1000
const Gigabit = Megabit * 1000

type readCloser struct {
	data       io.ReadCloser
	controller *ThrottleController // bitsPerSecond
}

func (r *readCloser) Read(p []byte) (int, error) {
	// buf := p
	// maxBufLen := r.controller.Rate()/(8*600) + 100
	// if maxBufLen < uint64(len(p)) {
	// 	buf = p[:maxBufLen]
	// }

	bytesRead, err := r.data.Read(p)
	r.controller.ConsumeBytes(bytesRead)
	//log.Printf("1.) read: %d, allowed: %d", bytesRead, bytesAllowed)
	return bytesRead, err
}

func (r *readCloser) Close() (err error) {
	r.controller.Remove()
	return r.data.Close()
}

type ThrottleController struct {
	rate      uint64 // bitsPerSecond
	timeRead  time.Time
	bytesRead float64

	allowNextReadAfter time.Duration

	numConn uint32

	mutex     sync.Mutex
	readMutex sync.Mutex
}

func NewThrottleController(rate uint64) *ThrottleController {
	var tc ThrottleController
	tc.rate = rate
	tc.timeRead = time.Now()
	tc.bytesRead = float64(0)
	tc.mutex = sync.Mutex{}
	tc.numConn = 0
	return &tc
}

func (tc *ThrottleController) ReadCloser(reader io.ReadCloser) *readCloser {
	var rc readCloser
	rc.controller = tc
	rc.data = reader
	tc.Add()
	return &rc
}

func (tc *ThrottleController) ConsumeBytes(bytes int) {
	tc.readMutex.Lock()
	defer tc.readMutex.Unlock()

	tc.mutex.Lock()
	rate := float64(tc.rate)
	allowNextReadAfter := tc.allowNextReadAfter
	tc.mutex.Unlock()

	time.Sleep(allowNextReadAfter)

	timeToBlock := float64(bytes) / (rate / 8.0) * float64(time.Second)
	//log.Print(timeToBlock)

	tc.mutex.Lock()
	tc.allowNextReadAfter = time.Duration(timeToBlock)
	tc.mutex.Unlock()
}

func (tc *ThrottleController) Rate() uint64 {
	tc.mutex.Lock()
	rate := tc.rate
	tc.mutex.Unlock()

	return rate
}

func (tc *ThrottleController) SetRate(rate uint64) {
	tc.mutex.Lock()
	tc.rate = rate
	tc.mutex.Unlock()
}

func (tc *ThrottleController) Remove() {
	tc.mutex.Lock()
	tc.numConn--
	tc.mutex.Unlock()
}

func (tc *ThrottleController) Add() {
	tc.mutex.Lock()
	tc.numConn++
	tc.mutex.Unlock()
}

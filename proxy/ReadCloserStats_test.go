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
package proxy

import (
	"bytes"
	"io/ioutil"
	"testing"
)

func TestReadCloserStats(t *testing.T) {
	rc := ioutil.NopCloser(bytes.NewReader([]byte("hello world")))
	rcs := ReadCloserStats(rc)
	if rc != rcs.rc {
		t.Errorf("Failed to set up RCS with proper io.ReadCloser")
	}
}
func TestRCSClose(t *testing.T) {
	rc := ioutil.NopCloser(bytes.NewReader([]byte("hello world")))
	rcs := ReadCloserStats(rc)
	if rcs.Closed || rcs.Used {
		t.Fatalf("readCloserStats not initalized with Closed=false and Used=false")
	}
	rcs.Close()
	if !rcs.Closed || !rcs.Used {
		t.Fatalf("readCloserStats.Close() does not set Closed and Used to true")
	}

}

func TestRCSRead(t *testing.T) {
	rc := ioutil.NopCloser(bytes.NewReader([]byte("hello world")))
	rcs := ReadCloserStats(rc)
	if rcs.Closed || rcs.Used {
		t.Fatalf("readCloserStats not initalized with Closed=false and Used=false")
	}
	rcs.Read([]byte{})
	if !rcs.Used {
		t.Fatalf("readCloserStats.Read() does not set Used to true")
	}
	if rcs.Closed {
		t.Fatalf("readCloserStats.Read set Closed to true")
	}
}

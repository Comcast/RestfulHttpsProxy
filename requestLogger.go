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

package main

import (
	"encoding/json"
	"net/http"
	//"net/http/httputil"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func copyResponse(resp *http.Response) *http.Response {
	var respCopy http.Response
	respCopy = *resp
	resp.Body, respCopy.Body = CloneReadCloser(resp.Body)
	return &respCopy
}

func copyRequest(req *http.Request) *http.Request {
	var reqCopy http.Request
	reqCopy = *req
	req.Body, reqCopy.Body = CloneReadCloser(req.Body)
	return &reqCopy
}

type CustomReadCloser struct {
	Reader io.Reader
	Closer io.Closer
}

func CloneReadCloser(rc io.ReadCloser) (io.ReadCloser, io.ReadCloser) {
	pr, pw := io.Pipe()
	tee := io.TeeReader(rc, pw)

	var teeRc CustomReadCloser
	var dupRc CustomReadCloser
	teeRc.Reader = tee
	teeRc.Closer = rc
	dupRc.Reader = pr
	dupRc.Closer = nil
	return &teeRc, &dupRc
}

// func CloneReadCloser(rc io.ReadCloser) (io.ReadCloser, io.ReadCloser) {
// 	var buf bytes.Buffer
// 	tee := io.TeeReader(rc, &buf)
//
// 	var teeRc CustomReadCloser
// 	var bufRc CustomReadCloser
// 	teeRc.Reader = tee
// 	teeRc.Closer = rc
// 	bufRc.Reader = &buf
// 	bufRc.Closer = nil
// 	return &teeRc, &bufRc
// }

// func CloneReadCloser(rc io.ReadCloser) (io.ReadCloser, io.ReadCloser) {
// 	return rc, nil
// }

func (r *CustomReadCloser) Read(p []byte) (n int, err error) {
	return r.Reader.Read(p)
}

func (r *CustomReadCloser) Close() (err error) {
	if r.Closer == nil {
		return nil
	}
	err = r.Closer.Close()
	return
}

type loggingProperties struct {
	Mutex     sync.Mutex
	recording bool
}

type requestLog struct {
	URL     string `json:"url"`
	Headers string `json:"headers"`
	Body    string `json:"body"`
}

type responseLog struct {
	Status  string `json:"status"`
	Headers string `json:"headers"`
	Body    string `json:"body"`
}

type roundTripLog struct {
	Req  requestLog  `json:"request"`
	Resp responseLog `json:"response,omitempty"`
}

var logProps = make(map[string]*loggingProperties)

func formatJSON(b []byte) ([]byte, error) {
	var out bytes.Buffer
	err := json.Indent(&out, b, "", "\t")
	return out.Bytes(), err
}

func compactJSON(b []byte) ([]byte, error) {
	var out bytes.Buffer
	err := json.Compact(&out, b)
	return out.Bytes(), err
}

func headerToString(h http.Header) string {
	var s strings.Builder
	h.Write(&s)
	return s.String()
}

func startLogging(ip string) {
	if logProps[ip] == nil {
		logProps[ip] = &loggingProperties{}
	}
	logProps[ip].Mutex.Lock()
	logProps[ip].recording = true
	logProps[ip].Mutex.Unlock()
}

func stopLogging(ip string) {
	if logProps[ip] == nil {
		logProps[ip] = &loggingProperties{}
	}
	logProps[ip].Mutex.Lock()
	logProps[ip].recording = false
	logProps[ip].Mutex.Unlock()
}

//json data in body
type logResponseBody struct {
	readCloser io.ReadCloser
	ip         string
}

func (lrc *logResponseBody) Read(p []byte) (int, error) {
	return lrc.readCloser.Read(p)
}

func (lrc *logResponseBody) Close() error {
	err := lrc.readCloser.Close()
	logProps[lrc.ip].Mutex.Unlock()
	return err
}

func getLogs(ip string) io.ReadCloser {
	if logProps[ip] == nil {
		logProps[ip] = &loggingProperties{}
	}
	logProps[ip].Mutex.Lock()
	var readCloser io.ReadCloser
	readCloser = getRequestLogFile("logs/" + ip + ".log")
	if readCloser == nil {
		readCloser = ioutil.NopCloser(strings.NewReader(""))
	}
	var lrc logResponseBody
	lrc.ip = ip
	lrc.readCloser = readCloser
	return &lrc
}

func clearLogs(ip string) {
	if logProps[ip] == nil {
		logProps[ip] = &loggingProperties{}
	}
	logProps[ip].Mutex.Lock()
	os.Remove("logs/" + ip + ".log")
	logProps[ip].Mutex.Unlock()
}

func logRequest(ip string, req *http.Request, resp *http.Response) {
	go func() {
		if logProps[ip] == nil {
			logProps[ip] = &loggingProperties{}
		}
		logProps[ip].Mutex.Lock()
		defer logProps[ip].Mutex.Unlock()
		if logProps[ip].recording == false {
			return
		}
		file := getRequestLogFile("logs/" + ip + ".log")
		defer file.Close()
		token := make([]byte, 1)
		if n, err := file.Read(token); err != nil || n != 1 || token[0] != '[' {
			file.Write([]byte("[]"))
			file.Seek(1, 0) // [|]
		}
		file.Seek(-2, 2) // ...}|]
		if n, err := file.Read(token); err == nil && n == 1 && token[0] == '}' {
			file.Write([]byte(","))
		}
		var log roundTripLog
		log.Req.URL = req.URL.String()
		log.Req.Headers = headerToString(req.Header)
		//reqBody, _ := ioutil.ReadAll(req.Body)
		log.Req.Body = "not supported yet" //string(reqBody)
		log.Resp.Status = resp.Status
		log.Resp.Headers = headerToString(resp.Header)
		//respBody, _ := ioutil.ReadAll(resp.Body)
		log.Resp.Body = "not supported yet" //string(respBody)
		bytes, err := json.Marshal(log)
		if err != nil {
			return
		}
		bytes, err = formatJSON(bytes)
		if err != nil {
			return
		}
		file.Write(bytes)

		file.Write([]byte("]"))
	}()
}

func getRequestLogFile(path string) *os.File {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
	f, _ := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	return f
}

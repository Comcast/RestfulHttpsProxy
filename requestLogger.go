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
	"restfulHttpsProxy/util"
	"strings"
	"sync"
)

func copyResponse(resp *http.Response) *http.Response {
	var respCopy http.Response
	respCopy = *resp

	respCopy.Header = make(http.Header)
	for key, value := range resp.Header {
		respCopy.Header[key] = value
	}

	split := util.BufferedSplit(resp.Body, 2)
	resp.Body, respCopy.Body = split[0], split[1]
	return &respCopy
}

func copyRequest(req *http.Request) *http.Request {
	var reqCopy http.Request
	reqCopy = *req

	reqCopy.Header = make(http.Header)
	for key, value := range req.Header {
		reqCopy.Header[key] = value
	}

	split := util.BufferedSplit(req.Body, 2)
	req.Body, reqCopy.Body = split[0], split[1]
	return &reqCopy
}

type Logger struct {
	logProps sync.Map
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

// var logProps = make(map[string]*loggingProperties)

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

func (l *Logger) startLogging(ip string) {
	log, _ := l.logProps.LoadOrStore(ip, &loggingProperties{})
	if log, ok := log.(*loggingProperties); ok {
		log.Mutex.Lock()
		log.recording = true
		log.Mutex.Unlock()
	}
}

func (l *Logger) stopLogging(ip string) {
	log, _ := l.logProps.LoadOrStore(ip, &loggingProperties{})
	if log, ok := log.(*loggingProperties); ok {
		log.Mutex.Lock()
		log.recording = false
		log.Mutex.Unlock()
	}
}

//json data in body
type logResponseBody struct {
	readCloser io.ReadCloser
	m          *sync.Mutex
}

func (lrc *logResponseBody) Read(p []byte) (int, error) {
	return lrc.readCloser.Read(p)
}

func (lrc *logResponseBody) Close() error {
	err := lrc.readCloser.Close()
	//logProps[lrc.ip].Mutex.Unlock()
	lrc.m.Unlock()
	return err
}

func (l *Logger) getLogs(ip string) io.ReadCloser {
	logUncasted, _ := l.logProps.LoadOrStore(ip, &loggingProperties{})
	log := logUncasted.(*loggingProperties)

	log.Mutex.Lock()

	var readCloser io.ReadCloser
	readCloser = getRequestLogFile("logs/" + ip + ".log")
	if readCloser == nil {
		readCloser = ioutil.NopCloser(strings.NewReader(""))
	}
	var lrc logResponseBody
	lrc.m = &log.Mutex
	lrc.readCloser = readCloser
	return &lrc
}

func (l *Logger) clearLogs(ip string) {
	logUncasted, _ := l.logProps.LoadOrStore(ip, &loggingProperties{})
	log := logUncasted.(*loggingProperties)
	log.Mutex.Lock()
	os.Remove("logs/" + ip + ".log")
	log.Mutex.Unlock()
}

func (l *Logger) logRequest(ip string, req *http.Request) func(resp *http.Response) {
	var log roundTripLog
	wg := sync.WaitGroup{}
	wg.Add(1)
	logUncasted, _ := l.logProps.LoadOrStore(ip, &loggingProperties{})
	file := logUncasted.(*loggingProperties)
	file.Mutex.Lock()
	defer file.Mutex.Unlock()
	if file.recording == false {
		return func(resp *http.Response) {}
	}
	reqCopy := copyRequest(req)
	go func() {
		log.Req.URL = reqCopy.URL.String()
		log.Req.Headers = headerToString(reqCopy.Header)
		reqBody, _ := ioutil.ReadAll(reqCopy.Body)
		reqCopy.Body.Close()
		log.Req.Body = string(reqBody)
		wg.Done()
	}()

	return func(resp *http.Response) {
		respCopy := copyResponse(resp)
		go func() {
			wg.Wait()
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
			log.Resp.Status = respCopy.Status
			log.Resp.Headers = headerToString(respCopy.Header)
			respBody, _ := ioutil.ReadAll(respCopy.Body)
			respCopy.Body.Close()
			log.Resp.Body = string(respBody)
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
}

// func logResponse(ip string, req *http.Request, resp *http.Response) {
// 	go func() {
// 		if logProps[ip] == nil {
// 			logProps[ip] = &loggingProperties{}
// 		}
// 		logProps[ip].Mutex.Lock()
// 		defer logProps[ip].Mutex.Unlock()
// 		if logProps[ip].recording == false {
// 			return
// 		}
// 		file := getRequestLogFile("logs/" + ip + ".log")
// 		defer file.Close()
// 		token := make([]byte, 1)
// 		if n, err := file.Read(token); err != nil || n != 1 || token[0] != '[' {
// 			file.Write([]byte("[]"))
// 			file.Seek(1, 0) // [|]
// 		}
// 		file.Seek(-2, 2) // ...}|]
// 		if n, err := file.Read(token); err == nil && n == 1 && token[0] == '}' {
// 			file.Write([]byte(","))
// 		}
// 		var log roundTripLog
// 		log.Req.URL = req.URL.String()
// 		log.Req.Headers = headerToString(req.Header)
// 		reqBody, _ := ioutil.ReadAll(req.Body)
// 		log.Req.Body = string(reqBody)
// 		log.Resp.Status = resp.Status
// 		log.Resp.Headers = headerToString(resp.Header)
// 		respBody, _ := ioutil.ReadAll(resp.Body)
// 		log.Resp.Body = string(respBody)
// 		bytes, err := json.Marshal(log)
// 		if err != nil {
// 			return
// 		}
// 		bytes, err = formatJSON(bytes)
// 		if err != nil {
// 			return
// 		}
// 		file.Write(bytes)
//
// 		file.Write([]byte("]"))
// 	}()
// }

func getRequestLogFile(path string) *os.File {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
	f, _ := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	return f
}

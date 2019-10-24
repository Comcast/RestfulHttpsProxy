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
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"restfulHttpsProxy/util"
	"sync"
	"time"
)

//type connSet map[*ConnProps]struct{}
type connSet struct {
	m sync.Map // map[*ConnProps]struct{}
	n int64
	l sync.Mutex
}

func (s *connSet) Range(f func(key, value interface{}) bool) {
	s.m.Range(f)
}

func (s *connSet) Len() int64 {
	s.l.Lock()
	defer s.l.Unlock()
	return s.n
}

func (s *connSet) Contains(v *ClientConnProps) bool {
	_, ok := s.m.Load(v)
	return ok
}

func (s *connSet) Add(v *ClientConnProps) {
	if s.Contains(v) {
		return
	}
	s.m.Store(v, nil)
	go func() {
		s.l.Lock()
		s.n++
		s.l.Unlock()
	}()
}

func (s *connSet) Remove(v *ClientConnProps) {
	if !s.Contains(v) {
		return
	}
	s.m.Delete(v)
	go func() {
		s.l.Lock()
		s.n--
		s.l.Unlock()
	}()
}

/*
--------------------------------------------------------------------------------
*/

type ClientConnProps struct {
	lastUsedTime time.Time
	//state        http.ConnState

	Conn                  net.Conn
	idleTimeout           time.Duration
	maxHeaderBytes        int64 // Not implemented yet
	closeConnAfterRequest bool
}

func (client *ClientConnProps) Write(resp *http.Response) error {
	resp.Header.Del("Connection")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Transfer-Encoding")

	if resp.ContentLength == 0 {
		resp.Body = nil
	}

	if resp.Body != nil && resp.ContentLength == -1 {
		bodyBytes := make([]byte, 8192)
		n, err := resp.Body.Read(bodyBytes)
		if err == io.EOF {
			resp.ContentLength = int64(n)
		}
		resp.Body = util.ReadCloserPair{
			Reader: io.MultiReader(bytes.NewBuffer(bodyBytes[:n]), resp.Body),
			Closer: resp.Body,
		}
	}

	if resp.ContentLength < 0 && !resp.Close {
		if resp.ProtoMajor == 1 && resp.ProtoMinor == 1 {
			resp.TransferEncoding = []string{"chunked"}
		} else {
			resp.TransferEncoding = []string{"identity"}
			resp.Close = true
		}
	}
	return resp.Write(client.Conn)
}

func (client *ClientConnProps) Close() error {
	if client.Conn == nil {
		return nil
	}
	return client.Conn.Close()

}

func (client *ClientConnProps) Listen() (*http.Request, error) {
	if client.Conn == nil {
		return nil, errors.New("Conn is nil")
	}
	clientBuffer := bufio.NewReader(
		// Add a limit reader in the future? But only for the header
		client.Conn,
	)
	request, err := http.ReadRequest(clientBuffer)
	if request == nil {
		if err == nil {
			err = errors.New("Request is nil")
		}
	} else {
		if request.URL.Scheme == "" {
			request.URL.Scheme = "https"
		}
		request.URL.Host = resolveRealHost(*request.URL, request.Host)
		request.Host = ""
	}
	return request, err
}

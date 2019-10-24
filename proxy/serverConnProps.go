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
	"compress/flate"
	"compress/gzip"
	"io"
	//"compress/lzw" // Could not find server that uses this to test.
	// TODO: add other compression algorithms
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"restfulHttpsProxy/util"
	"sync"
	"time"
	// 3rd party, probably slower, could use google's alternative
	// but it need to build with c-go
	"bytes"
	"github.com/dsnet/compress/brotli"
)

// This order is really important, will fail to handshake if it isn't in this order for some sites.
var allTlsCipherSuites = []uint16{
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,

	tls.TLS_RSA_WITH_RC4_128_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,

	// we dont support tls1.3 for now, need special env var to enable it.

	// tls.TLS_AES_128_GCM_SHA256,
	// tls.TLS_AES_256_GCM_SHA384,
	// tls.TLS_CHACHA20_POLY1305_SHA256,

	tls.TLS_FALLBACK_SCSV,
}

func createConn(dst *url.URL) (net.Conn, error) {
	dstWithPort := *dst
	insertPort(&dstWithPort)
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}
	if dst.Scheme != "https" {
		server, err := dialer.Dial(
			"tcp",
			dstWithPort.Host,
		)
		if err != nil {
			if server != nil {
				server.Close()
			}
			return nil, err
		}
		return server, err
	}
	config := &tls.Config{
		InsecureSkipVerify:       true,
		CipherSuites:             allTlsCipherSuites,
		MinVersion:               tls.VersionSSL30,
		MaxVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: false,
	}
	server, err := tls.DialWithDialer(
		dialer,
		"tcp",
		dstWithPort.Host,
		config,
	)
	if err != nil {
		if server != nil {
			server.Close()
		}
		return nil, err
	}
	return server, err
}

type ServerConnProps struct {
	lastUsedTime time.Time

	MaxConns              int
	Conn                  net.Conn
	Conns                 map[string]net.Conn // map["scheme://host"]net.Conn
	ResponseHeaderTimeout time.Duration
	maxHeaderBytes        int64 // Not implemented yet

	listenLoopMu sync.Mutex
	writeLoopMu  sync.Mutex
	connsMu      sync.Mutex
}

func (scp *ServerConnProps) Close() error {
	scp.connsMu.Lock()
	defer scp.connsMu.Unlock()
	return scp.close()
}

func (scp *ServerConnProps) close() error {
	var err error
	for _, conn := range scp.Conns {
		newErr := conn.Close()
		if err == nil {
			err = newErr
		}
	}
	scp.Conns = make(map[string]net.Conn)
	scp.Conn = nil
	return err
}

func (scp *ServerConnProps) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	request.Header.Set("Accept-Encoding", "identity, gzip, deflate, br")
	request.Header.Del("Connection")
	request.Header.Del("Content-Length")
	request.Header.Del("Transfer-Encoding")

	if request.ContentLength == 0 {
		request.Body = nil
	}

	if request.Body != nil && request.ContentLength == -1 {
		bodyBytes := make([]byte, 8192)
		n, err := request.Body.Read(bodyBytes)
		if err == io.EOF {
			request.ContentLength = int64(n)
		}
		request.Body = util.ReadCloserPair{
			Reader: io.MultiReader(bytes.NewBuffer(bodyBytes[:n]), request.Body),
			Closer: request.Body,
		}
	}

	if request.ContentLength < 0 && !request.Close {
		if request.ProtoMajor == 1 && request.ProtoMinor == 1 {
			request.TransferEncoding = []string{"chunked"}
		} else {
			request.TransferEncoding = []string{"identity"}
			request.Close = true
		}
	}

	var resp *http.Response
	var errS error
	var errR error

	if request.Body == nil {
		resp, errS, errR = scp.tryRoundTrip(request)
		if errS != nil {
			resp, errS, errR = scp.tryRoundTrip(request)
		}
	} else {
		body := util.ReadCloserStats(request.Body)
		request.Body = body

		resp, errS, errR = scp.tryRoundTrip(request)
		if errS != nil && !body.Used {
			resp, errS, errR = scp.tryRoundTrip(request)
		}
	}
	if errS != nil {
		return resp, errS
	}
	return resp, errR
}

func cancelHandle(f func()) (cancel func(), action func()) {
	var once sync.Once
	cancel = func() {
		once.Do(func() {})
	}
	action = func() {
		once.Do(f)
	}
	return action, cancel
}

// returns (*http.Response, error sending request, error getting response)
func (scp *ServerConnProps) tryRoundTrip(request *http.Request) (*http.Response, error, error) {
	var errS error
	var errR error

	errS = scp.Open(request)
	if errS != nil {
		return nil, errS, errR
	}

	if scp.Conn != nil && request.Header.Get("Remote-Address") != "" {
		request.Header.Set("Remote-Address", scp.Conn.RemoteAddr().String())
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	timeoutFunc, cancelTimeoutFunc := cancelHandle(func() { scp.Close() })
	var resp *http.Response
	go func() {
		errS := scp.Write(request)
		if errS != nil {
			scp.Close()
		} else {
			go func() {
				time.Sleep(scp.ResponseHeaderTimeout)
				timeoutFunc()
			}()
		}
		wg.Done()
	}()
	go func() {
		resp, errR = scp.Listen(request)
		cancelTimeoutFunc()
		if errR != nil {
			scp.Close()
		}
		wg.Done()
	}()
	wg.Wait()

	return resp, errS, errR
}

func (scp *ServerConnProps) Open(request *http.Request) error {
	scp.connsMu.Lock()
	defer scp.connsMu.Unlock()

	if scp.Conns == nil {
		scp.Conns = make(map[string]net.Conn)
	}

	dst := *request.URL
	insertPort(&dst)
	host := dst.Scheme + "://" + dst.Host

	if server, ok := scp.Conns[host]; ok {
		scp.Conns[host] = server
		scp.Conn = server
		return nil
	}
	if len(scp.Conns) > scp.MaxConns {
		scp.close()
	}
	server, err := createConn(&dst)
	if err != nil {
		scp.Conn = nil
		return err
	}
	scp.Conn = server
	scp.Conns[host] = server
	return nil
}

func (scp *ServerConnProps) Write(request *http.Request) error {
	scp.writeLoopMu.Lock()
	defer scp.writeLoopMu.Unlock()

	return request.Write(scp.Conn)
}

func (scp *ServerConnProps) Listen(request *http.Request) (*http.Response, error) {
	scp.listenLoopMu.Lock()
	defer scp.listenLoopMu.Unlock()

	resp, err := http.ReadResponse(
		bufio.NewReader(scp.Conn),
		request,
	)

	if resp == nil {
		return resp, err
	}

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		var gzipReader *gzip.Reader
		if gzipReader, err = gzip.NewReader(resp.Body); err != nil {
			break
		}
		resp.Body = NewReadDoubleCloser(gzipReader, resp.Body)
		resp.Header.Del("Content-Encoding")
		resp.ContentLength = -1
	case "deflate":
		defalteReader := flate.NewReader(resp.Body)
		resp.Body = NewReadDoubleCloser(defalteReader, resp.Body)
		resp.Header.Del("Content-Encoding")
		resp.ContentLength = -1
	case "br":
		var brotliReader *brotli.Reader
		if brotliReader, err = brotli.NewReader(resp.Body, nil); err != nil {
			break
		}
		resp.Body = NewReadDoubleCloser(brotliReader, resp.Body)
		resp.Header.Del("Content-Encoding")
		resp.ContentLength = -1
		// case "compress":
		// 	lzwReader := lzw.NewReader(resp.Body, lzw.LSB, 8)
		// 	resp.Body = NewReadDoubleCloser(lzwReader, resp.Body)
		// 	resp.Header.Del("Content-Encoding")
		// 	resp.ContentLength = -1
	}

	if resp.ContentLength == -1 && !contains(resp.TransferEncoding, "chunked") && !resp.Close {
		resp.TransferEncoding = []string{"chunked"}
	}

	if resp.Close {
		resp.Body = NewReadDoubleCloser(resp.Body, scp.Conn)
	}

	return resp, err
}

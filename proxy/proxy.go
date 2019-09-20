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
	"crypto/tls"
	//"io"
	"io/ioutil"
	//"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"
	//"golang.org/x/net/http2"
	//"net/url"
	//"strings"
)

// var DefaultHTTPTransport = &http.Transport{
// 	Proxy: nil,
// 	// NOTE FOR SAM: This proxy can be configured to use another proxy (Charles).
// 	// Uncomment below.
// 	// Proxy: http.ProxyURL(&url.URL{
// 	// 	Scheme:     "http",
// 	// 	Opaque:     "",               // encoded opaque data
// 	// 	User:       nil,              // username and password information
// 	// 	Host:       "127.0.0.1:8888", // host or host:port
// 	// 	Path:       "",               // path (relative paths may omit leading slash)
// 	// 	RawPath:    "",               // encoded path hint (see EscapedPath method); added in Go 1.5
// 	// 	ForceQuery: false,            // append a query ('?') even if RawQuery is empty; added in Go 1.7
// 	// 	RawQuery:   "",               // encoded query values, without '?'
// 	// 	Fragment:   "",               // fragment for references, without '#'
// 	// }),
// 	DialContext: (&net.Dialer{
// 		Timeout:   30 * time.Second,
// 		KeepAlive: 30 * time.Second,
// 		DualStack: true,
// 	}).DialContext,
// 	MaxIdleConns:          10, // no limit is 0
// 	IdleConnTimeout:       90 * time.Second,
// 	TLSHandshakeTimeout:   10 * time.Second,
// 	ExpectContinueTimeout: 1 * time.Second,
// 	ResponseHeaderTimeout: 5 * time.Second,
// }

type proxy struct {
	Cert   tls.Certificate
	Modify func(request *http.Request, client *ClientConnProps, server *ServerConnProps) (*http.Request, *http.Response)

	conns     connSet
	idleConns connSet

	IdleTimeout           time.Duration
	MaxHeaderBytes        int64
	MaxConnsKeptAlive     int64
	ResponseHeaderTimeout time.Duration
}

func (p *proxy) handleConnect(connectRequest *http.Request, client net.Conn) (net.Conn, error) {
	mitm := true
	if _, port := SplitHostAndPort(connectRequest.URL.Host); port != "443" {
		// If its not port 443 then we have no idea what protocol it is.
		mitm = false
	}

	if _, ok := client.(*tls.Conn); ok {
		mitm = false
	}
	hostAndPort := resolveRealHost(*connectRequest.URL, connectRequest.Host)
	host, _ := SplitHostAndPort(hostAndPort)
	if mitm {
		signedCert, _ := signHost(p.Cert, []string{host})

		config := &tls.Config{
			Certificates:             []tls.Certificate{signedCert},
			InsecureSkipVerify:       true,
			CipherSuites:             allTlsCipherSuites,
			MinVersion:               tls.VersionSSL30,
			PreferServerCipherSuites: true,
		}

		_, err := client.Write([]byte(connectRequest.Proto + " 200 OK\r\n\r\n"))
		if err != nil {
			return nil, err
		}

		// Upgrade to a TLS connection
		clientTLS := tls.Server(client, config)
		if err := clientTLS.Handshake(); err != nil {
			return nil, err
		}
		return clientTLS, nil
	}
	server, err := net.DialTimeout("tcp", hostAndPort, 10*time.Second)
	if err != nil {
		return nil, err
	}

	client.Write([]byte(connectRequest.Proto + " 200 OK\r\n\r\n"))
	doubleSidedCopy(client, server)
	client.Close()
	server.Close()
	return nil, nil
}

func (p *proxy) OnRequest(modify func(request *http.Request, client *ClientConnProps, server *ServerConnProps) (*http.Request, *http.Response)) {
	p.Modify = modify
}

var clientConns int64 = 0
var InBlock int64 = 0

// var serverConns int64 = 0

func (p *proxy) listenConn(client *ClientConnProps) {
	server := &ServerConnProps{
		ResponseHeaderTimeout: p.ResponseHeaderTimeout,
	}

	atomic.AddInt64(&clientConns, 1)
	defer atomic.AddInt64(&clientConns, -1)

	// NOTE FOR SAM: This code block is needed when running the proxy for long periods of time, it might crash sometimes.
	// Uncomment Below:

	// defer func() {
	// 	if r := recover(); r != nil {
	// 		log.Println("Recovered from panic in listenConn", r)
	// 		// defer func() {
	// 		// 	if r := recover(); r != nil {
	// 		// 		log.Println("Recovered from panic in client.Conn.Close()", r)
	// 		// 	}
	// 		// }()
	// 		// client.Conn.Close()
	// 	}
	// }()

	for {
		request, err := client.Listen()
		if err != nil {
			break
		}
		p.idleConns.Remove(client)

		if request.Method == http.MethodConnect {
			// handleConnect will upgrade the connection to TLS
			client.Conn, _ = p.handleConnect(request, client.Conn)
			if client.Conn == nil {
				break
			}
			continue
		}
		request.RemoteAddr = client.Conn.RemoteAddr().String()
		removeProxyHeaders(request)
		removeRedundantPort(request.URL)

		var resp *http.Response

		if p.Modify != nil {
			request, resp = p.Modify(request, client, server)
		}

		if request == nil {
			//client.Conn.Write([]byte(request.Proto + " 404 Not Found\r\n\r\n"))
			break
		}

		if resp == nil {
			resp, err = server.RoundTrip(request)
			if resp == nil {
				//log.Print(err)
				//client.Conn.Write([]byte(request.Proto + " 404 Not Found\r\n\r\n"))
				break
			}
		}

		// if resp.ProtoAtLeast(1, 1) {
		// 	if resp.Header.Get("Connection") == "close" {
		// 		close = true
		// 	} else {
		// 		close = false
		// 	}
		// } else {
		// 	if resp.Header.Get("Connection") == "keep-alive" {
		// 		close = false
		// 	} else {
		// 		close = true
		// 	}
		// }

		if client.closeConnAfterRequest {
			resp.Header.Set("Connection", "close")
			resp.Close = true
		}

		//resp.TransferEncoding = []string{"chunked"}

		// Guard against user error
		if resp.Body == nil {
			resp.Body = ioutil.NopCloser(bytes.NewReader([]byte{}))
		}

		// If its not chunked, we need to calculate the ContentLength or close at end
		if !contains(resp.TransferEncoding, "chunked") && resp.ContentLength == -1 && !resp.Close {
			resp.Header.Set("Connection", "close")
			resp.Close = true
			// or we could do this:
			// resp.TransferEncoding = []string{"chunked"}
		}

		err = client.Write(resp)
		if err != nil {
			//log.Print(err)
			break
		}

		if resp.Close {
			break // close the TCP or TLS connection.
		}

		client.lastUsedTime = time.Now()
		p.idleConns.Add(client)
	}
	server.Close()
	client.Close()
	p.idleConns.Remove(client)
}

func Proxy() *proxy {
	p := proxy{}
	p.IdleTimeout = 100 * time.Second
	p.MaxHeaderBytes = 20000
	p.MaxConnsKeptAlive = 30
	p.ResponseHeaderTimeout = 30 * time.Second
	p.Modify = func(
		req *http.Request,
		conn *ClientConnProps,
		server *ServerConnProps,
	) (
		*http.Request,
		*http.Response,
	) {
		return req, nil
	}
	return &p
}

func NewResponse(req *http.Request) *http.Response {
	resp := &http.Response{}
	resp.Request = req
	resp.TransferEncoding = req.TransferEncoding
	resp.ProtoMajor = req.ProtoMajor
	resp.ProtoMinor = req.ProtoMinor
	resp.Header = make(http.Header)
	resp.StatusCode = 200
	return resp
}

func (p *proxy) Listen(host string) {
	l, err := net.Listen("tcp", host)
	if err != nil {
		//log.Println(err)
		return
	}
	defer l.Close()

	go p.launchTimeoutChecker()

	// go func() {
	// 	for {
	// 		time.Sleep(time.Second * 2)
	// 		log.Print("------------------------")
	// 		log.Print("client: ", atomic.LoadInt64(&clientConns))
	// 		//log.Print("block: ", atomic.LoadInt64(&InBlock))
	// 		log.Print("------------------------")
	// 	}
	// }()

	for {
		c, err := l.Accept()
		if err != nil {
			//log.Print(err)
			return
		}
		close := false
		if p.conns.Len() >= p.MaxConnsKeptAlive {
			close = true
		}
		client := &ClientConnProps{
			lastUsedTime: time.Now(),
			//state:        http.StateNew,

			Conn:                  c,
			idleTimeout:           p.IdleTimeout,
			maxHeaderBytes:        p.MaxHeaderBytes, // not supported
			closeConnAfterRequest: close,
		}

		go func(p *proxy, client *ClientConnProps) {
			p.conns.Add(client)
			p.listenConn(client)
			p.conns.Remove(client)
		}(p, client)
	}
}

func (p *proxy) launchTimeoutChecker() {
	for {
		time.Sleep(time.Second)
		p.idleConns.Range(
			func(key, value interface{}) bool {
				client, ok := key.(*ClientConnProps)
				if !ok {
					return true
				}
				if time.Now().After(client.lastUsedTime.Add(client.idleTimeout)) {
					client.Close()
				}
				return true
			},
		)
	}
}

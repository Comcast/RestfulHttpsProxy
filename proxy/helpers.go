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
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

func doubleSidedCopy(left io.ReadWriter, right io.ReadWriter) error {
	errs := []error{}
	wg := sync.WaitGroup{}
	m := &sync.Mutex{}
	wg.Add(2)
	go func() {
		_, err := io.Copy(left, right)
		if err != nil {
			m.Lock()
			errs = append(errs, err)
			m.Unlock()
		}
		wg.Done()
	}()
	go func() {
		_, err := io.Copy(right, left)
		if err != nil {
			m.Lock()
			errs = append(errs, err)
			m.Unlock()
		}
		wg.Done()
	}()
	wg.Wait()
	return combinedError(errs)
}

func combinedError(errs []error) error {
	combinedErr := error(nil)
	for _, err := range errs {
		if combinedErr == nil {
			combinedErr = errors.New(err.Error())
		} else {
			combinedErr = errors.New(combinedErr.Error() + ", " + err.Error())
		}
	}
	return combinedErr
}

// taken from goproxy, but modified a bit
func removeProxyHeaders(r *http.Request) {
	r.RequestURI = "" // this must be reset when serving a request with the client
	// If no Accept-Encoding header exists, Transport will add the headers it can accept
	// and would wrap the response body with the relevant reader.
	r.Header.Del("Accept-Encoding")
	// curl can add that, see
	// https://jdebp.eu./FGA/web-proxy-connection-header.html
	r.Header.Del("Proxy-Connection")
	r.Header.Del("Proxy-Authenticate")
	r.Header.Del("Proxy-Authorization")
	// Connection, Authenticate and Authorization are single hop Header:
	// http://www.w3.org/Protocols/rfc2616/rfc2616.txt
	// 14.10 Connection
	//   The Connection general-header field allows the sender to specify
	//   options that are desired for that particular connection and MUST NOT
	//   be communicated by proxies over further connections.
	r.Header.Del("Connection")
}

func removeRedundantPort(u *url.URL) {
	host, port := SplitHostAndPort(u.Host)
	if u.Scheme == "http" && port == "80" {
		u.Host = host
	} else if u.Scheme == "https" && port == "443" {
		u.Host = host
	}
}

func SplitHostAndPort(hostWithPort string) (host string, port string) {
	host = ""
	port = ""
	s := strings.Split(hostWithPort, ":")
	if len(s) >= 1 {
		host = s[0]
	}
	if len(s) >= 2 {
		port = s[1]
	}
	return
}

func resolveRealHost(URL url.URL, headerHost string) string {
	//resolve actual host.
	insertPort(&URL)
	hostFromURL, portFromURL := SplitHostAndPort(URL.Host)
	if headerHost != "" {
		host, port := SplitHostAndPort(headerHost)
		if port == "" {
			port = portFromURL
		}
		if host == "" {
			host = hostFromURL
		}
		return host + ":" + port
	}
	return URL.Host
}

func insertPort(u *url.URL) {
	host, port := SplitHostAndPort(u.Host)
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	u.Host = host + ":" + port
}

func contains(slice []string, element string) bool {
	for i := range slice {
		if slice[i] == element {
			return true
		}
	}
	return false
}

// func getAddressFromConn(c net.Conn) string {
// 	var strAddr string = c.RemoteAddr().String()
// 	log.Println("addr.IP.To16()" + strAddr)
// 	// switch addr := netAddr.(type) {
// 	// // case *net.UDPAddr:
// 	// // 	strAddr = addr.IP.To16().String() + ":" + strconv.Itoa(addr.Port)
// 	// // 	log.Println("addr.IP.To16()" + strAddr)
// 	// // case *net.TCPAddr:
// 	// // 	strAddr = addr.IP.To16().String() + ":" + strconv.Itoa(addr.Port)
// 	// // 	log.Println("addr.IP.To16()" + strAddr)
// 	// // }
// 	return strAddr
// }

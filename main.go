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
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"restfulHttpsProxy/proxy"
	"restfulHttpsProxy/prxConfig"
	"restfulHttpsProxy/rewriteLogic"
	"restfulHttpsProxy/throttle"
	"sync"
	"time"
)

const regexBufferSize = 4096 * 4

func setBodyString(resp *http.Response, s string) {
	buf := bytes.NewBufferString(s)
	resp.ContentLength = int64(buf.Len())
	resp.Body = ioutil.NopCloser(buf)
}

func handleProxyAPI(req *http.Request) *http.Response {
	clientIP, _ := proxy.SplitHostAndPort(req.RemoteAddr)
	resp := proxy.NewResponse(req)
	errResp := proxy.NewResponse(req)
	errResp.StatusCode = 404
	setBodyString(errResp, "")
	if req.URL.Path == "/api/rules/set" {
		newRulesBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			setBodyString(errResp, err.Error())
			return errResp
		}

		if newRulesBytes != nil {
			var config prxConfig.Config
			err = json.Unmarshal(newRulesBytes, &config)
			if err != nil {
				setBodyString(errResp, err.Error())
				return errResp
			}
			if config.IP != nil {
				clientIP = *config.IP
			}
			newRewriteRules, err := prxConfig.Compile(config)
			if err != nil {
				setBodyString(errResp, err.Error())
				return errResp
			}
			if newRewriteRules != nil {
				rewriteRules.Store(clientIP, newRewriteRules)
			} else {
				rewriteRules.Delete(clientIP)
			}

			throttledConnections.Delete(clientIP)
		}

		buf := bytes.NewBufferString("setting rules")
		resp.ContentLength = int64(buf.Len())
		resp.Body = ioutil.NopCloser(buf)
	} else if req.URL.Path == "/api/rules/clear" {
		rewriteRules.Delete(clientIP)
		throttledConnections.Delete(clientIP)

		buf := bytes.NewBufferString("clearing rules")
		resp.ContentLength = int64(buf.Len())
		resp.Body = ioutil.NopCloser(buf)
		// } else if req.URL.Path == "/api/logging/start" {
		// 	buf := bytes.NewBufferString("Starting to log")
		// 	resp.ContentLength = int64(buf.Len())
		// 	resp.Body = ioutil.NopCloser(buf)
		// 	startLogging(clientIP)
		// } else if req.URL.Path == "/api/logging/stop" {
		// 	buf := bytes.NewBufferString("Stopping logging")
		// 	resp.ContentLength = int64(buf.Len())
		// 	resp.Body = ioutil.NopCloser(buf)
		// 	stopLogging(clientIP)
		// } else if req.URL.Path == "/api/logging/get" {
		// 	resp.ContentLength = -1
		// 	resp.Body = getLogs(clientIP)
		// } else if req.URL.Path == "/api/logging/clear" {
		// 	buf := bytes.NewBufferString("Clearing Clogs")
		// 	resp.ContentLength = int64(buf.Len())
		// 	resp.Body = ioutil.NopCloser(buf)
		// 	clearLogs(clientIP)
	} else if req.URL.Path == "/ca.pem" {
		buf := bytes.NewReader(caBytes)
		resp.ContentLength = int64(buf.Len())
		resp.Body = ioutil.NopCloser(buf)
		resp.Header.Set("Content-Type", "application/x-pem-file")
	} else {
		return errResp
	}
	resp.StatusCode = 200
	return resp
}

var lastTimeUsed sync.Map // map[string]time.Time

var rewriteRules sync.Map

var throttledConnections sync.Map

func launchSessionCleaner(period time.Duration, expiration time.Duration) {
	for {
		time.Sleep(period)
		lastTimeUsed.Range(
			func(key, val interface{}) bool {
				if used, ok := val.(time.Time); ok {
					if time.Now().After(used.Add(expiration)) {
						lastTimeUsed.Delete(key)
						rewriteRules.Delete(key)
						throttledConnections.Delete(key)
					}
				}
				return true
			},
		)
	}
}

var caBytes []byte

func main() {
	var err error

	var caPath string
	var keyPath string

	flag.StringVar(&caPath, "pem", "ca.pem", "path to pem file")
	flag.StringVar(&keyPath, "key", "key.pem", "path to key file")
	flag.Parse()

	caBytes, _ = ioutil.ReadFile(caPath)

	prx := proxy.Proxy()

	prx.Cert, err = tls.LoadX509KeyPair(caPath, keyPath) // need to cache this
	if err != nil {
		log.Println(err)
		log.Println("Creating new certificate in proxy root folder")
		cert, err := createPemFiles()
		caBytes = cert.Certificate[0]
		if cert == nil {
			log.Println("Failed to create or load certificate")
			if err != nil {
				log.Println(err)
			}
			return
		}
		prx.Cert = *cert
	}

	prx.OnRequest(
		func(
			req *http.Request,
			client *proxy.ClientConnProps,
			server *proxy.ServerConnProps,
		) (
			*http.Request,
			*http.Response,
		) {
			ip, _ := proxy.SplitHostAndPort(req.RemoteAddr)

			log.Print("[" + req.RemoteAddr + "] <" + req.Method + "> " + req.URL.String())

			if req.ProtoMajor == 2 {
				log.Println("SHOULD NOT HAVE HTTP2 REQUESTS")
				log.Println(req.Proto)
				log.Println(req.URL.String())
			}

			var resp *http.Response

			lastTimeUsed.Store(ip, time.Now())

			host, _ := proxy.SplitHostAndPort(req.URL.Host)

			if host == "a.proxi" {
				resp = handleProxyAPI(req)
				return req, resp
			}

			//requestCopy := copyRequest(req)

			val, _ := rewriteRules.Load(ip)
			rewriteRulesForClient, _ := val.(prxConfig.RewriteRules)

			originalReqURL := req.URL.String()

			for _, entry := range rewriteRulesForClient {
				if !entry.URL.MatchString(originalReqURL) {
					continue
				}

				err := rewriteLogic.AlterURL(req.URL, entry.Rewrite.Request.URL)
				if err != nil {
					log.Print(err.Error())
					return nil, nil
				}

				err = rewriteLogic.AlterHeader(&req.Header, entry.Rewrite.Request.Header)
				if err != nil {
					log.Print(err.Error())
					return req, nil
				}

				// empty body might be replaced, fix this later
				if len(entry.Rewrite.Request.Body) > 0 {
					req.Body = rewriteLogic.AlterBody(
						req.Body,
						regexBufferSize,
						entry.Rewrite.Request.Body,
					)
					req.ContentLength = -1
				}

				if entry.UploadSpeed != nil {
					var throttledClient *sync.Map
					var throttleController *throttle.ThrottleController
					if val, ok := throttledConnections.Load(ip); ok {
						throttledClient = val.(*sync.Map)
					} else {
						throttledClient = &sync.Map{}
						throttledConnections.Store(ip, throttledClient)
					}
					if val, ok := throttledClient.Load("rq\n" + entry.URL.String()); ok {
						throttleController = val.(*throttle.ThrottleController)
					} else {
						throttleController = throttle.NewThrottleController(*entry.UploadSpeed)
						throttledClient.Store("rq\n"+entry.URL.String(), throttleController)
					}
					req.Body = throttleController.ReadCloser(req.Body)
				}
			}

			req.Host = req.URL.Host
			resp, err = server.RoundTrip(req)
			if resp == nil {
				log.Print(err)
				return req, nil
			}

			responseDelay := uint64(0)

			for _, entry := range rewriteRulesForClient {
				if !entry.URL.MatchString(originalReqURL) {
					continue
				}

				rewriteLogic.AlterHeader(&resp.Header, entry.Rewrite.Response.Header)
				rewriteLogic.AlterStatus(resp, entry.Rewrite.Response.Status)

				// empty body might be replaced, fix this later
				if len(entry.Rewrite.Response.Body) > 0 {
					resp.Body = rewriteLogic.AlterBody(
						resp.Body,
						regexBufferSize,
						entry.Rewrite.Response.Body,
					)
					resp.ContentLength = -1
				}

				if entry.ResponseDelay != nil && *entry.ResponseDelay > responseDelay {
					responseDelay = *entry.ResponseDelay
				}

				if entry.DownloadSpeed != nil {
					var throttledClient *sync.Map
					var throttleController *throttle.ThrottleController
					if val, ok := throttledConnections.Load(ip); ok {
						throttledClient = val.(*sync.Map)
					} else {
						throttledClient = &sync.Map{}
						throttledConnections.Store(ip, throttledClient)
					}
					if val, ok := throttledClient.Load(entry.URL.String()); ok {
						throttleController = val.(*throttle.ThrottleController)
					} else {
						throttleController = throttle.NewThrottleController(*entry.DownloadSpeed)
						throttledClient.Store(entry.URL.String(), throttleController)
					}
					resp.Body = throttleController.ReadCloser(resp.Body)
				}
			}

			if responseDelay != 0 {
				time.Sleep(time.Duration(responseDelay) * time.Microsecond)
			}

			//responseCopy := copyResponse(resp)
			//logRequest(ip, requestCopy, responseCopy)
			//logRequest(ip, req, resp)
			return req, resp
		},
	)
	go launchSessionCleaner(time.Minute, time.Hour*48)
	go launchExposedAPI(":" + os.Args[1])
	prx.Listen(":" + os.Args[2])
}

func launchExposedAPI(host string) error {
	return http.ListenAndServe(
		host,
		http.HandlerFunc(
			func(w http.ResponseWriter, req *http.Request) {
				resp := handleProxyAPI(req)
				w.WriteHeader(resp.StatusCode)
				io.Copy(w, resp.Body)
			},
		),
	)
}

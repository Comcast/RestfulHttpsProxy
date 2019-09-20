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

package rewriteLogic

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"restfulHttpsProxy/prxConfig"
	"strconv"
	"strings"
)

func applyStreamRule(input io.Reader, bufferSize int, rule prxConfig.Rule) io.Reader {
	if rule.Find != nil && rule.Replace != nil {
		input = RegexReader(input, bufferSize, rule.Find, []byte(*rule.Replace))
	} else if rule.Replace != nil {
		input = strings.NewReader(*rule.Replace)
	} else if rule.Prepend != nil { // remove else?
		input = io.MultiReader(strings.NewReader(*rule.Prepend), input)
	} else if rule.Append != nil { // remove else?
		input = io.MultiReader(input, strings.NewReader(*rule.Prepend))
	}
	return input
}

func applyRule(input string, rule prxConfig.Rule) string {
	if rule.Find != nil && rule.Replace != nil {
		input = rule.Find.ReplaceAllString(input, *rule.Replace)
	} else if rule.Replace != nil {
		input = *rule.Replace
	} else if rule.Prepend != nil { // remove else?
		input = *rule.Prepend + input
	} else if rule.Append != nil { // remove else?
		input = input + *rule.Append
	}
	return input
}

func AlterURL(u *url.URL, URLRules []prxConfig.Rule) error {
	urlStr := u.String()
	for _, URLRule := range URLRules {
		urlStr = applyRule(urlStr, URLRule)
	}
	newURL, err := url.Parse(urlStr)
	if err != nil {
		return err
	}
	*u = *newURL //copy it
	return nil
}

func headerGetKeyValue(headerStr string) (key string, value string) {
	key = ""
	value = ""
	s := strings.SplitN(headerStr, ":", 2)
	if len(s) >= 1 {
		key = s[0]
	}
	if len(s) >= 2 {
		value = strings.TrimSpace(s[1])
	}
	return
}

func AlterStatus(response *http.Response, statusRules []prxConfig.Rule) {
	statusStr := response.Status
	for _, statusRule := range statusRules {
		statusStr = applyRule(statusStr, statusRule)
	}

	s := strings.Split(statusStr, " ")
	code, err := strconv.Atoi(s[0])
	if err != nil {
		return
	}
	response.Status = statusStr
	response.StatusCode = code
}

func headerToString(header http.Header) string {
	log.Print(header)
	str := ""
	for k, vv := range header {
		for _, v := range vv {
			if str != "" {
				str += "\n"
			}
			str += k + ": " + v
		}
	}
	return "\n" + str + "\n"
}

func stringToHeader(str string) http.Header {
	header := make(http.Header)
	lines := strings.Split(str, "\n")
	for _, line := range lines {
		k, v := headerGetKeyValue(line)
		if k != "" || v != "" {
			header.Add(k, v)
		}
	}
	log.Print(header)
	return header
}

func AlterHeader(header *http.Header, headerRules []prxConfig.Rule) error {
	if len(headerRules) <= 0 {
		return nil
	}
	headerStr := headerToString(*header)

	for _, headerRule := range headerRules {
		headerStr = applyRule(headerStr, headerRule)
	}
	if len(headerStr) < 1 || headerStr[0] != '\n' {
		headerStr = "\n" + headerStr
	}
	if len(headerStr) < 2 || headerStr[len(headerStr)-1] != '\n' {
		headerStr = headerStr + "\n"
	}
	//log.Print("\nheader new\n----------\n" + headerStr + "\n")
	*header = stringToHeader(headerStr)
	return nil
}

type readCloser struct {
	data       io.Reader
	dataCloser io.Closer
}

func AlterBody(r io.ReadCloser, bufferSize int, rules []prxConfig.Rule) io.ReadCloser {
	if rules == nil || len(rules) == 0 {
		return r
	}

	var rc readCloser

	var reader io.Reader
	reader = r

	for _, rule := range rules {
		reader = applyStreamRule(reader, bufferSize, rule)
	}

	rc.data = reader
	rc.dataCloser = r

	return &rc
}

func (r *readCloser) Read(p []byte) (int, error) {
	bytesRead, err := r.data.Read(p)
	return bytesRead, err
}

func (r *readCloser) Close() (err error) {
	return r.dataCloser.Close()
}

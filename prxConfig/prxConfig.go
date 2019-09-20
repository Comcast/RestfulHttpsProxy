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

package prxConfig

import (
	// "bytes"
	// "io"
	// "io/ioutil"
	"errors"
	"regexp"
)

type Rule struct {
	Prepend *string

	Find    *regexp.Regexp
	Replace *string

	Append *string
	//no delete, replace with ""
}

type Config struct {
	IP    *string     `json:"ip,omitempty"`
	Rules []EntryJSON `json:"rules,omitempty"`
}

type RuleJSON struct {
	Find    *string `json:"find,omitempty"`
	Replace *string `json:"replace,omitempty"`
	Append  *string `json:"append,omitempty"`
	Prepend *string `json:"prepend,omitempty"`
	Delete  *string `json:"delete,omitempty"`
}

type TypeJSON struct {
	URL    []RuleJSON `json:"url,omitempty"`
	Header []RuleJSON `json:"header,omitempty"`
	Body   []RuleJSON `json:"body,omitempty"`
	Status []RuleJSON `json:"status,omitempty"`
}

type Type struct {
	URL    []Rule
	Header []Rule
	Body   []Rule
	Status []Rule
}

type WhereJSON struct {
	Request  *TypeJSON `json:"request,omitempty"`
	Response *TypeJSON `json:"response,omitempty"`
}

type Where struct {
	Request  *Type
	Response *Type
}

type EntryJSON struct {
	URL           *string `json:"url,omitempty"`
	UploadSpeed   *uint64 `json:"uploadSpeed,omitempty"`
	DownloadSpeed *uint64 `json:"downloadSpeed,omitempty"`
	ResponseDelay *uint64 `json:"responseDelay,omitempty"`

	Rewrite *WhereJSON `json:"rewrite,omitempty"`
}

type Entry struct {
	URL           *regexp.Regexp
	UploadSpeed   *uint64
	DownloadSpeed *uint64
	ResponseDelay *uint64

	Rewrite *Where
}

// type RewriteRulesJSON []EntryJSON
type RewriteRules []Entry

func Compile(configJSON Config) (RewriteRules, error) {

	rewriteRulesJSON := configJSON.Rules
	var rewriteRules RewriteRules
	var err error
	for _, entryJSON := range rewriteRulesJSON {
		entry := Entry{}
		if entryJSON.URL != nil {
			entry.URL, err = regexp.Compile(*entryJSON.URL)
			if err != nil {
				return nil, err
			}
		} else {
			entry.URL, err = regexp.Compile(".*")
		}
		entry.DownloadSpeed = entryJSON.DownloadSpeed
		entry.UploadSpeed = entryJSON.UploadSpeed
		entry.ResponseDelay = entryJSON.ResponseDelay
		if entryJSON.Rewrite == nil {
			entryJSON.Rewrite = &WhereJSON{}
		}
		entry.Rewrite = &Where{}
		if entryJSON.Rewrite.Request == nil {
			entryJSON.Rewrite.Request = &TypeJSON{}
		}
		if entryJSON.Rewrite.Response == nil {
			entryJSON.Rewrite.Response = &TypeJSON{}
		}
		entry.Rewrite.Request, err = compileTypes(entryJSON.Rewrite.Request)
		if err != nil {
			return nil, err
		}
		entry.Rewrite.Response, err = compileTypes(entryJSON.Rewrite.Response)
		if err != nil {
			return nil, err
		}
		rewriteRules = append(rewriteRules, entry)
	}
	return rewriteRules, nil
}

func compileTypes(typesJSON *TypeJSON) (*Type, error) {
	var types Type
	var err error
	types.URL, err = compileRules(typesJSON.URL)
	if err != nil {
		return nil, err
	}
	types.Header, err = compileRules(typesJSON.Header)
	if err != nil {
		return nil, err
	}
	types.Body, err = compileRules(typesJSON.Body)
	if err != nil {
		return nil, err
	}
	types.Status, err = compileRules(typesJSON.Status)
	if err != nil {
		return nil, err
	}
	return &types, nil
}

func compileRules(rulesJSON []RuleJSON) ([]Rule, error) {
	var err error
	var rules []Rule
	for _, ruleJSON := range rulesJSON {
		rule := Rule{}
		if ruleJSON.Replace != nil {
			if ruleJSON.Append != nil || ruleJSON.Prepend != nil || ruleJSON.Delete != nil {
				return nil, errors.New("Illegal field choice in rewrite rule")
			}
			rule.Replace = ruleJSON.Replace
			if ruleJSON.Find != nil {
				rule.Find, err = regexp.Compile(*ruleJSON.Find)
				if err != nil {
					return nil, err
				}
			}
		} else if ruleJSON.Prepend != nil {
			if ruleJSON.Delete != nil || ruleJSON.Find != nil || ruleJSON.Replace != nil || ruleJSON.Append != nil {
				return nil, errors.New("Illegal field choice in rewrite rule")
			}
			rule.Prepend = ruleJSON.Prepend
		} else if ruleJSON.Append != nil {
			if ruleJSON.Delete != nil || ruleJSON.Find != nil || ruleJSON.Replace != nil || ruleJSON.Prepend != nil {
				return nil, errors.New("Illegal field choice in rewrite rule")
			}
			rule.Append = ruleJSON.Append
		} else if ruleJSON.Delete != nil {
			if ruleJSON.Prepend != nil || ruleJSON.Find != nil || ruleJSON.Replace != nil || ruleJSON.Append != nil {
				return nil, errors.New("Illegal field choice in rewrite rule")
			}
			rule.Find, err = regexp.Compile(*ruleJSON.Delete)
			if err != nil {
				return nil, err
			}
			emptyString := ""
			rule.Replace = &emptyString
		} else {
			return nil, errors.New("Illegal field choice in rewrite rule")
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

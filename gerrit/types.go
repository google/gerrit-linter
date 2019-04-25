// Copyright 2019 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gerrit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

var jsonPrefix = []byte(")]}'")

type File struct {
	Status        string
	LinesInserted int `json:"lines_inserted"`
	SizeDelta     int `json:"size_delta"`
	Size          int
	Content       []byte
}

type Change struct {
	Files map[string]*File
}

type CheckerInput struct {
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Repository  string   `json:"repository"`
	Status      string   `json:"status"`
	Blocking    []string `json:"blocking"`
	Query       string   `json:"query"`
}

const timeLayout = "2006-01-02 15:04:05.000000000"

type Timestamp time.Time

func (ts *Timestamp) String() string {
	return ((time.Time)(*ts)).String()
}

func (ts *Timestamp) MarshalJSON() ([]byte, error) {
	t := (*time.Time)(ts)
	return []byte("\"" + t.Format(timeLayout) + "\""), nil
}

func (ts *Timestamp) UnmarshalJSON(b []byte) error {
	b = bytes.TrimPrefix(b, []byte{'"'})
	b = bytes.TrimSuffix(b, []byte{'"'})
	t, err := time.Parse(timeLayout, string(b))
	if err != nil {
		return err
	}
	*ts = Timestamp(t)
	return nil
}

type CheckerInfo struct {
	UUID        string `json:"uuid"`
	Name        string
	Description string
	URL         string `json:"url"`
	Repository  string `json:"repository"`
	Status      string
	Blocking    []string  `json:"blocking"`
	Query       string    `json:"query"`
	Created     Timestamp `json:"created"`
	Updated     Timestamp `json:"updated"`
}

func Unmarshal(content []byte, dest interface{}) error {
	if !bytes.HasPrefix(content, jsonPrefix) {
		if len(content) > 100 {
			content = content[:100]
		}
		bodyStr := string(content)

		return fmt.Errorf("prefix %q not found, got %s", jsonPrefix, bodyStr)
	}

	content = bytes.TrimPrefix(content, []byte(jsonPrefix))

	return json.Unmarshal(content, dest)
}

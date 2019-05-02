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

// Gerrit doesn't use the format with "T" in the middle, so must
// define a custom serializer.

const timeLayout = "2006-01-02 15:04:05.000000000"

type Timestamp time.Time

func (ts *Timestamp) String() string {
	return ((time.Time)(*ts)).String()
}

var _ = (json.Marshaler)((*Timestamp)(nil))

func (ts *Timestamp) MarshalJSON() ([]byte, error) {
	t := (*time.Time)(ts)
	return []byte("\"" + t.Format(timeLayout) + "\""), nil
}

var _ = (json.Unmarshaler)((*Timestamp)(nil))

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

func (info *CheckerInfo) String() string {
	out, _ := json.Marshal(info)
	return string(out)
}

// Unmarshal unmarshals Gerrit JSON, stripping the security prefix.
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

type PendingCheckInfo struct {
	State string
}

type CheckablePatchSetInfo struct {
	Repository   string
	ChangeNumber int `json:"change_number"`
	PatchSetID   int `json:"patch_set_id"`
}

func (in *CheckablePatchSetInfo) String() string {
	out, _ := json.Marshal(in)
	return string(out)
}

type PendingChecksInfo struct {
	PatchSet      *CheckablePatchSetInfo       `json:"patch_set"`
	PendingChecks map[string]*PendingCheckInfo `json:"pending_checks"`
}

func (info *PendingCheckInfo) String() string {
	out, _ := json.Marshal(info)
	return string(out)
}

type CheckInput struct {
	CheckerUUID string     `json:"checker_uuid"`
	State       string     `json:"state"`
	Message     string     `json:"message"`
	URL         string     `json:"url"`
	Started     *Timestamp `json:"started"`
}

func (in *CheckInput) String() string {
	out, _ := json.Marshal(in)
	return string(out)
}

type CheckInfo struct {
	Repository    string    `json:"repository"`
	ChangeNumber  int       `json:"change_number"`
	PatchSetID    int       `json:"patch_set_id"`
	CheckerUUID   string    `json:"checker_uuid"`
	State         string    `json:"state"`
	Message       string    `json:"message"`
	Started       Timestamp `json:"started"`
	Finished      Timestamp `json:"finished"`
	Created       Timestamp `json:"created"`
	Updated       Timestamp `json:"updated"`
	CheckerName   string    `json:"checker_name"`
	CheckerStatus string    `json:"checker_status"`
	Blocking      []string  `json:"blocking"`
}

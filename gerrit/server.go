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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// Server represents a single Gerrit host.
type Server struct {
	UserAgent string
	URL       url.URL
	Client    http.Client

	// Issue trace requests.
	Debug bool

	// Base64 encoded user:secret string.
	BasicAuth string
}

// New creates a Gerrit Server for the given URL.
func New(u url.URL) *Server {
	g := &Server{
		URL: u,
	}

	g.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return nil
	}

	return g
}

// GetPath runs a Get on the given path.
func (g *Server) GetPath(p string) ([]byte, error) {
	u := g.URL
	u.Path = path.Join(u.Path, p)
	if strings.HasSuffix(p, "/") && !strings.HasSuffix(u.Path, "/") {
		// Ugh.
		u.Path += "/"
	}
	return g.Get(&u)
}

// Do runs a HTTP request against the remote server.
func (g *Server) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", g.UserAgent)
	if g.BasicAuth != "" {
		req.Header.Set("Authorization", "Basic "+string(g.BasicAuth))
	}

	if g.Debug {
		if req.URL.RawQuery != "" {
			req.URL.RawQuery += "&trace=0x1"
		} else {
			req.URL.RawQuery += "trace=0x1"
		}
	}
	return g.Client.Do(req)
}

// Get runs a HTTP GET request on the given URL.
func (g *Server) Get(u *url.URL) ([]byte, error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	rep, err := g.Do(req)
	if err != nil {
		return nil, err
	}
	if rep.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Get %s: status %d", u.String(), rep.StatusCode)
	}

	defer rep.Body.Close()
	return ioutil.ReadAll(rep.Body)
}

// PostPath posts the given data onto a path.
func (g *Server) PostPath(p string, contentType string, content []byte) ([]byte, error) {
	u := g.URL
	u.Path = path.Join(u.Path, p)
	if strings.HasSuffix(p, "/") && !strings.HasSuffix(u.Path, "/") {
		// Ugh.
		u.Path += "/"
	}
	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(content))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	rep, err := g.Do(req)
	if err != nil {
		return nil, err
	}
	if rep.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Post %s: status %d", u.String(), rep.StatusCode)
	}

	defer rep.Body.Close()
	return ioutil.ReadAll(rep.Body)
}

// GetContent returns the file content from a file in a change.
func (g *Server) GetContent(changeID string, revID string, fileID string) ([]byte, error) {
	u := g.URL
	path := path.Join(u.Path, fmt.Sprintf("changes/%s/revisions/%s/files/",
		url.PathEscape(changeID), revID))
	u.Path = path + "/" + fileID + "/content"
	u.RawPath = path + "/" + url.PathEscape(fileID) + "/content"
	c, err := g.Get(&u)
	if err != nil {
		return nil, err
	}

	dest := make([]byte, base64.StdEncoding.DecodedLen(len(c)))
	n, err := base64.StdEncoding.Decode(dest, c)
	if err != nil {
		return nil, err
	}
	return dest[:n], nil
}

// GetChange returns the Change (including file contents) for a given change.
func (g *Server) GetChange(changeID string, revID string) (*Change, error) {
	content, err := g.GetPath(fmt.Sprintf("changes/%s/revisions/%s/files/",
		url.PathEscape(changeID), revID))
	if err != nil {
		return nil, err
	}
	content = bytes.TrimPrefix(content, jsonPrefix)

	files := map[string]*File{}
	if err := json.Unmarshal(content, &files); err != nil {
		return nil, err
	}

	for name, file := range files {
		if file.Status == "D" {
			continue
		}
		c, err := g.GetContent(changeID, revID, name)
		if err != nil {
			return nil, err
		}

		files[name].Content = c
	}
	return &Change{files}, nil
}

func (s *Server) PendingChecksByScheme(scheme string) ([]*PendingChecksInfo, error) {
	u := s.URL

	// The trailing '/' handling is really annoying.
	u.Path = path.Join(u.Path, "a/plugins/checks/checks.pending/") + "/"

	q := "scheme:" + scheme
	u.RawQuery = "query=" + q
	content, err := s.Get(&u)
	if err != nil {
		return nil, err
	}

	var out []*PendingChecksInfo
	if err := Unmarshal(content, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// PendingChecks returns the checks pending for the given checker.
func (s *Server) PendingChecks(checkerUUID string) ([]*PendingChecksInfo, error) {
	u := s.URL

	// The trailing '/' handling is really annoying.
	u.Path = path.Join(u.Path, "a/plugins/checks/checks.pending/") + "/"

	q := "checker:" + checkerUUID
	u.RawQuery = "query=" + url.QueryEscape(q)

	content, err := s.Get(&u)
	if err != nil {
		return nil, err
	}

	var out []*PendingChecksInfo
	if err := Unmarshal(content, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// PostCheck posts a single check result onto a change.
func (s *Server) PostCheck(changeID string, psID int, input *CheckInput) (*CheckInfo, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	res, err := s.PostPath(fmt.Sprintf("a/changes/%s/revisions/%d/checks/", changeID, psID),
		"application/json", body)
	if err != nil {
		return nil, err
	}

	var out CheckInfo
	if err := Unmarshal(res, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

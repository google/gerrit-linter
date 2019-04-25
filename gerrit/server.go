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

type Server struct {
	UserAgent string
	URL       url.URL
	Client    http.Client

	// Base64 encoded user:secret string.
	BasicAuth string
}

func New(u url.URL) *Server {
	g := &Server{
		URL: u,
	}

	g.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		g.setRequest(req)
		return nil
	}

	return g
}

func (g *Server) setAuth(auth []byte) {
}

func (g *Server) setRequest(req *http.Request) {
	req.Header.Set("User-Agent", g.UserAgent)
	req.Header.Set("Authorization", "Basic "+string(g.BasicAuth))
}

func (g *Server) GetPath(p string) ([]byte, error) {
	u := g.URL
	u.Path = path.Join(u.Path, p)
	if strings.HasSuffix(p, "/") && !strings.HasSuffix(u.Path, "/") {
		// Ugh.
		u.Path += "/"
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	g.setRequest(req)
	rep, err := g.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if rep.StatusCode != 200 {
		return nil, fmt.Errorf("Get %s: status %d", u.String(), rep.StatusCode)
	}

	defer rep.Body.Close()
	return ioutil.ReadAll(rep.Body)
}

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
	g.setRequest(req)
	req.Header.Set("Content-Type", contentType)
	rep, err := g.Client.Do(req)
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
	c, err := g.GetPath(fmt.Sprintf("changes/%s/revisions/%s/files/%s/content",
		url.PathEscape(changeID), revID, url.PathEscape(fileID)))
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

	for name := range files {
		c, err := g.GetContent(changeID, revID, name)
		if err != nil {
			return nil, err
		}

		files[name].Content = c
	}
	return &Change{files}, nil
}

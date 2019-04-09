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

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/rpc"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/fmtserver"
	"github.com/google/slothfs/cookie"
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

type gerrit struct {
	UserAgent string
	URL       url.URL
	client    http.Client
}

func (g *gerrit) getPath(p string) ([]byte, error) {
	u := g.URL
	u.Path = path.Join(u.Path, p)
	if strings.HasSuffix(p, "/") && !strings.HasSuffix(u.Path, "/") {
		// Ugh.
		u.Path += "/"
	}

	rep, err := g.client.Get(u.String())
	if err != nil {
		return nil, err
	}
	if rep.StatusCode != 200 {
		return nil, fmt.Errorf("Get %s: status %d", u.String(), rep.StatusCode)
	}

	defer rep.Body.Close()
	return ioutil.ReadAll(rep.Body)
}

func (g *gerrit) postPath(p string, contentType string, content []byte) ([]byte, error) {
	u := g.URL
	u.Path = path.Join(u.Path, p)

	rep, err := g.client.Post(u.String(), contentType, bytes.NewBuffer(content))
	if err != nil {
		return nil, err
	}
	if rep.StatusCode != 200 {
		return nil, fmt.Errorf("Get %s: status %d", u, rep.StatusCode)
	}

	defer rep.Body.Close()
	return ioutil.ReadAll(rep.Body)
}

// GetContent returns the file content from a file in a change.
func (g *gerrit) GetContent(changeID string, revID string, fileID string) ([]byte, error) {
	c, err := g.getPath(fmt.Sprintf("changes/%s/revisions/%s/files/%s/content",
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
func (g *gerrit) GetChange(changeID string, revID string) (*Change, error) {
	content, err := g.getPath(fmt.Sprintf("changes/%s/revisions/%s/files/",
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

type CheckerInput struct {
	Name        string
	Description string
	URL         string `json:"url"`
	Repository  string
	Status      string
	Blocking    string
	Query       string
}

type CheckerInfo struct {
	UUID        string
	Name        string
	Description string
	URL         string `json:"url"`
	Repository  string
	Status      string
	Blocking    string
	Query       string
	Created     time.Time
	Updated     time.Time
}

func (g *gerrit) CreateChecker() error {
	in := CheckerInput{
		Name:        "fmtserver",
		Description: "check source code formatting.",
		Status:      "ENABLED",
		// TODO: should list all file extensions in the query?
	}

	body, err := json.Marshal(&in)
	if err != nil {
		return err
	}

	content, err := g.postPath("plugins/checks/checkers", "application/json", body)

	if !bytes.HasPrefix(content, jsonPrefix) {
		if len(content) > 100 {
			content = content[:100]
		}
		bodyStr := string(content)

		return fmt.Errorf("prefix %q not found, got %s", jsonPrefix, bodyStr)
	}

	content = bytes.TrimPrefix(content, []byte(jsonPrefix))
	out := CheckerInfo{}

	if err := json.Unmarshal(content, &out); err != nil {
		return fmt.Errorf("Unmarshal: %v", err)
	}

	return nil
}

type gerritChecker struct {
	gerrit    *gerrit
	fmtClient *rpc.Client
}

func (c *gerritChecker) checkChange(changeID string) error {
	ch, err := c.gerrit.GetChange(changeID, "current")
	if err != nil {
		return err
	}
	req := fmtserver.FormatRequest{}
	for n, f := range ch.Files {
		req.Files = append(req.Files,
			fmtserver.File{
				Name:    n,
				Content: f.Content,
			})
	}
	rep := fmtserver.FormatReply{}
	if err := c.fmtClient.Call("Server.Format", &req, &rep); err != nil {
		_, ok := err.(rpc.ServerError)
		if ok {
			return fmt.Errorf("server returned: %s", err)
		}
		return err
	}

	for _, f := range rep.Files {
		orig := ch.Files[f.Name]
		if orig == nil {
			return fmt.Errorf("result had unknown file %q", f.Name)
		}
		if !bytes.Equal(f.Content, orig.Content) {
			msg := f.Message
			if msg == "" {
				msg = "needs formatting"
			}
			log.Printf("file %s: %s", f.Name, f.Message)
		}
	}

	return nil
}

func main() {
	gerritURL := flag.String("gerrit", "", "URL to gerrit host")
	addr := flag.String("addr", "", "Address of the fmtserver")
	register := flag.Bool("register", false, "Register with the host")
	list := flag.Bool("list", false, "List pending checks")
	agent := flag.String("agent", "fmtserver", "user-agent for the fmtserver.")
	cookieJar := flag.String("cookies", "", "path to cURL-style cookie jar file.")
	flag.Parse()

	if *gerritURL == "" {
		log.Fatal("must set --gerrit")
	}

	u, err := url.Parse(*gerritURL)
	if err != nil {
		log.Fatalf("url.Parse: %v", err)
	}

	g := &gerrit{
		URL: *u,
	}
	if nm := *cookieJar; nm != "" {
		jar, err := cookie.NewJar(nm)
		if err != nil {
			log.Fatal("NewJar(%s): %v", nm, err)
		}
		if err := cookie.WatchJar(jar, nm); err != nil {
			log.Printf("WatchJar: %v", err)
			log.Println("continuing despite WatchJar failure", err)
		}

		g.client.Jar = jar
	}
	g.UserAgent = *agent

	// Allow redirects so login forms can work.
	g.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// only some User-Agents can jump the auth
		req.Header.Set("User-Agent", g.UserAgent)
		return nil
	}

	_ = *register
	if *list {
		if c, err := g.getPath("plugins/checks/checkers/"); err != nil {
			log.Fatalf("ListCheckers: %v", err)
		} else {
			io.Copy(os.Stdout, bytes.NewBuffer(c))
		}
		os.Exit(0)
	}

	if *addr == "" {
		log.Fatal("must set --addr")
	}

	client, err := rpc.DialHTTP("tcp", *addr)
	if err != nil {
		log.Fatalf("DialHTTP(%s): %v", *addr, err)
	}

	gc := gerritChecker{
		gerrit:    g,
		fmtClient: client,
	}

	if len(flag.Args()) < 1 {
		log.Fatal("pass change IDs on command line")
	}
	for _, a := range flag.Args() {
		if err := gc.checkChange(a); err != nil {
			log.Printf("change %s: %v", a, err)
		}
	}
}

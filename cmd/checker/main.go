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
	"io/ioutil"
	"log"
	"net/http"
	"net/rpc"
	"net/url"

	"github.com/google/fmtserver"
)

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
	Host   string
	client http.Client
}

func (g *gerrit) GetContent(changeID string, revID string, fileID string) ([]byte, error) {
	u := fmt.Sprintf("https://%s/changes/%s/revisions/%s/files/%s/content",
		g.Host, url.PathEscape(changeID), revID, url.PathEscape(fileID))
	resp, err := g.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("Get %s: %v", u, err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Get %s: status %d", u, resp.StatusCode)
	}
	c, err := ioutil.ReadAll(resp.Body)
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

func (g *gerrit) GetChange(changeID string, revID string) (*Change, error) {
	u := fmt.Sprintf("https://%s/changes/%s/revisions/%s/files/",
		g.Host, url.PathEscape(changeID), revID)
	resp, err := g.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("Get %s: %v", u, err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Get %s: status %d", u, resp.StatusCode)
	}

	c, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c = bytes.TrimPrefix(c, []byte(")]}'"))

	files := map[string]*File{}
	if err := json.Unmarshal(c, &files); err != nil {
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
	host := flag.String("host", "", "Gerrit host to check")
	addr := flag.String("addr", "", "Address of the fmtserver")
	flag.Parse()

	if *host == "" {
		log.Fatal("must set --host")
	}
	g := &gerrit{
		Host: *host,
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

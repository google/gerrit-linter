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
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/gerritfmt"
	"github.com/google/gerritfmt/gerrit"
	"github.com/google/slothfs/cookie"
)

type hardcodedJar struct {
	mu      sync.Mutex
	cookies []*http.Cookie
}

func NewHardcodedJar(nm string) (*hardcodedJar, error) {
	j := &hardcodedJar{}
	if err := j.read(nm); err != nil {
		return nil, err
	}
	go func() {
		// TODO: use inotify
		for range time.Tick(time.Minute) {
			if err := j.read(nm); err != nil {
				log.Println("read %s: %v", nm, err)
			}
		}
	}()
	return j, nil
}

func (j *hardcodedJar) read(nm string) error {
	f, err := os.Open(nm)
	if err != nil {
		return err
	}
	defer f.Close()

	cs, err := cookie.ParseCookieJar(f)
	if err != nil {
		return err
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	j.cookies = cs
	return nil
}

func (j *hardcodedJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()

	var r []*http.Cookie
	for _, c := range j.cookies {
		if strings.HasSuffix(u.Host, c.Domain) {
			r = append(r, c)
		}
	}
	return r
}

func (w *hardcodedJar) SetCookies(u *url.URL, cs []*http.Cookie) {
	log.Println("SetCookies", u, cs)
}

func main() {
	gerritURL := flag.String("gerrit", "", "URL to gerrit host")
	register := flag.Bool("register", false, "Register with the host")
	update := flag.Bool("update", false, "Update an existing checker on the host")
	list := flag.Bool("list", false, "List pending checks")
	agent := flag.String("agent", "fmtserver", "user-agent for the fmtserver.")
	cookieJar := flag.String("cookies", "", "comma separated paths to cURL-style cookie jar file.")
	authFile := flag.String("auth_file", "", "file containing user:password")
	repo := flag.String("repo", "", "the repository (project) name to apply the checker to.")
	language := flag.String("language", "", "the language that the checker should apply to.")
	flag.Parse()
	if *gerritURL == "" {
		log.Fatal("must set --gerrit")
	}

	// Add path to self to $PATH, for easy deployment.
	if exe, err := os.Executable(); err == nil {
		os.Setenv("PATH", filepath.Dir(exe)+":"+os.Getenv("PATH"))
	}

	u, err := url.Parse(*gerritURL)
	if err != nil {
		log.Fatalf("url.Parse: %v", err)
	}

	if *authFile == "" && *cookieJar == "" {
		log.Fatal("must set --auth_file or --cookies")
	}

	g := gerrit.New(*u)

	if nm := *cookieJar; nm != "" {
		g.Client.Jar, err = NewHardcodedJar(nm)
		if err != nil {
			log.Fatal("NewHardcodedJar: %v", err)
		}
	}
	g.UserAgent = *agent

	if *authFile != "" {
		if content, err := ioutil.ReadFile(*authFile); err != nil {
			log.Fatal(err)
		} else {
			auth := bytes.TrimSpace(content)
			encoded := make([]byte, base64.StdEncoding.EncodedLen(len(auth)))
			base64.StdEncoding.Encode(encoded, auth)
			g.BasicAuth = string(encoded)
		}
	}

	// Do a GET first to complete any cookie dance, because POST aren't redirected properly.
	if _, err := g.GetPath("a/accounts/self"); err != nil {
		log.Fatalf("accounts/self: %v", err)
	}

	gc, err := NewGerritChecker(g)
	if err != nil {
		log.Fatal(err)
	}

	if *list {
		if out, err := gc.ListCheckers(); err != nil {
			log.Fatalf("List: %v", err)
		} else {
			for _, ch := range out {
				json, _ := json.Marshal(ch)
				os.Stdout.Write(json)
				os.Stdout.Write([]byte{'\n'})
			}
		}

		os.Exit(0)
	}

	if *register || *update {
		if *repo == "" {
			log.Fatalf("need to set --repo")
		}

		if *language == "" {
			log.Fatalf("must set --language.")
		}

		if !gerritfmt.IsSupported(*language) {
			log.Fatalf("language is not supported. Choices are %s", gerritfmt.SupportedLanguages())
		}

		ch, err := gc.PostChecker(*repo, *language, *update)
		if err != nil {
			log.Fatalf("CreateChecker: %v", err)
		}
		log.Printf("CreateChecker result: %v", ch)
		os.Exit(0)
	}

	gc.Serve()
}

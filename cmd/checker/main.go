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
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/google/gerritfmt"
	"github.com/google/gerritfmt/gerrit"
	"github.com/google/slothfs/cookie"
)

const checkerScheme = "fmt:"

func PostChecker(s *gerrit.Server, repo, language string, create bool) (*gerrit.CheckerInfo, error) {
	hash := sha1.New()
	hash.Write([]byte(repo))

	uuid := fmt.Sprintf("%s%s-%x", checkerScheme, language, hash.Sum(nil))
	in := gerrit.CheckerInput{
		UUID:        uuid,
		Name:        language + " formatting",
		Repository:  repo,
		Description: "check source code formatting.",
		Status:      "ENABLED",
		Query:       gerritfmt.Formatters[language].Query,
	}

	body, err := json.Marshal(&in)
	if err != nil {
		return nil, err
	}

	log.Println(string(body))

	path := "a/plugins/checks/checkers/"
	if !create {
		path += uuid
	}
	content, err := s.PostPath(path, "application/json", body)
	if err != nil {
		return nil, err
	}

	out := gerrit.CheckerInfo{}
	if err := gerrit.Unmarshal(content, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// ListCheckers returns all the checkers that conform to our scheme.
func ListCheckers(g *gerrit.Server) ([]*gerrit.CheckerInfo, error) {
	c, err := g.GetPath("a/plugins/checks/checkers/")
	if err != nil {
		log.Fatalf("ListCheckers: %v", err)
	}

	var out []*gerrit.CheckerInfo
	if err := gerrit.Unmarshal(c, &out); err != nil {
		return nil, err
	}

	filtered := out[:0]
	for _, o := range out {
		if !strings.HasPrefix(o.UUID, checkerScheme) {
			continue
		}
		if _, ok := checkerLanguage(o.UUID); !ok {
			continue
		}

		filtered = append(filtered, o)
	}
	return filtered, nil
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

	u, err := url.Parse(*gerritURL)
	if err != nil {
		log.Fatalf("url.Parse: %v", err)
	}

	g := gerrit.New(*u)

	if nm := *cookieJar; nm != "" {
		jar, err := cookie.NewJar(nm)
		if err != nil {
			log.Fatalf("NewJar(%s): %v", nm, err)
		}
		if err := cookie.WatchJar(jar, nm); err != nil {
			log.Printf("WatchJar: %v", err)
			log.Println("continuing despite WatchJar failure", err)
		}
		g.Client.Jar = jar
	}
	g.UserAgent = *agent

	if *authFile == "" {
		log.Fatal("must set --auth_file")
	}
	if content, err := ioutil.ReadFile(*authFile); err != nil {
		log.Fatal(err)
	} else {
		auth := bytes.TrimSpace(content)
		encoded := make([]byte, base64.StdEncoding.EncodedLen(len(auth)))
		base64.StdEncoding.Encode(encoded, auth)
		g.BasicAuth = string(encoded)
	}

	// Do a GET first to complete any cookie dance, because POST aren't redirected properly.
	if _, err := g.GetPath("a/accounts/self"); err != nil {
		log.Fatalf("accounts/self: %v", err)
	}

	if *list {
		if out, err := ListCheckers(g); err != nil {
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

		ch, err := PostChecker(g, *repo, *language, *update)
		if err != nil {
			log.Fatalf("CreateChecker: %v", err)
		}
		log.Printf("CreateChecker result: %v", ch)
		os.Exit(0)
	}

	gc, err := NewGerritChecker(g)
	if err != nil {
		log.Fatal(err)
	}

	gc.Serve()
}

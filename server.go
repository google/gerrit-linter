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

package fmtserver

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Server struct {
	JavaJar       string
	Buildifier    string
	languageRegex map[string]*regexp.Regexp
	formatterMap  map[string]formatterFunc
}

type formatterFunc func(in []File, out io.Writer) ([]FormattedFile, error)

func NewServer() *Server {
	type formatter struct {
		lang  string
		regex *regexp.Regexp
		fun   formatterFunc
	}

	s := &Server{
		languageRegex: map[string]*regexp.Regexp{},
		formatterMap:  map[string]formatterFunc{},
	}

	formatters := []formatter{
		{"java", regexp.MustCompile(`\.java$`), s.javaFormat},
		{"bazel", regexp.MustCompile(`(\.bzl|/BUILD|^BUILD)$`), s.bazelFormat},
		{"go", regexp.MustCompile(`\.go$`), s.goFormat},
		{"commit-msg", regexp.MustCompile(`^/COMMIT_MSG$`), s.commitCheck},
	}

	for _, l := range formatters {
		s.languageRegex[l.lang] = l.regex
	}
	for _, l := range formatters {
		s.formatterMap[l.lang] = l.fun
	}
	return s
}

func (s *Server) splitByLang(in []File) map[string][]File {
	res := map[string][]File{}
	for _, f := range in {
		for lang, regex := range s.languageRegex {
			if regex.MatchString(f.Name) {
				res[lang] = append(res[lang], f)
				break
			}
		}
	}
	return res
}

func (s *Server) Format(req *FormatRequest, rep *FormatReply) error {
	for lang, files := range s.splitByLang(req.Files) {
		var buf bytes.Buffer
		out, err := s.formatterMap[lang](files, &buf)
		if err != nil {
			return err
		}

		if len(out) > 0 && out[0].Message == "" {
			out[0].Message = buf.String()
		}
		rep.Files = append(rep.Files, out...)
	}
	return nil
}

func (s *Server) commitCheck(in []File, outSink io.Writer) (out []FormattedFile, err error) {
	complaint := s.checkCommitMessage(string(in[0].Content))
	ff := FormattedFile{}
	ff.Name = in[0].Name
	if complaint != "" {
		ff.Message = complaint
	} else {
		ff.Content = in[0].Content
	}
	out = append(out, ff)
	return out, nil
}

func (s *Server) checkCommitMessage(msg string) (complaint string) {
	lines := strings.Split(msg, "\n")
	if len(lines) < 2 {
		return "must have multiple lines"
	}

	if len(lines[1]) > 1 {
		return "subject and body must be separated by blank line"
	}

	if len(lines[0]) > 70 {
		return "subject must be less than 70 chars"
	}

	if strings.HasSuffix(lines[0], ".") {
		return "subject must not end in '.'"
	}

	return ""
}

func (s *Server) javaFormat(in []File, outSink io.Writer) (out []FormattedFile, err error) {
	if _, err := os.Stat(s.JavaJar); err != nil {
		return nil, fmt.Errorf("Stat(%q): %v", s.JavaJar, err)
	}
	cmd := exec.Command(
		"java",
		"-jar",
		s.JavaJar,
		"-i",
	)
	return s.inlineFixTool(cmd, in, outSink)
}

func (s *Server) bazelFormat(in []File, outSink io.Writer) (out []FormattedFile, err error) {
	if _, err := os.Stat(s.Buildifier); err != nil {
		return nil, fmt.Errorf("Stat(%q): %v", s.Buildifier, err)
	}
	cmd := exec.Command(
		s.Buildifier,
		"-mode=fix",
	)
	return s.inlineFixTool(cmd, in, outSink)
}

func (s *Server) goFormat(in []File, outSink io.Writer) (out []FormattedFile, err error) {
	if _, err := os.Stat(s.Buildifier); err != nil {
		return nil, fmt.Errorf("Stat(%q): %v", s.Buildifier, err)
	}
	cmd := exec.Command(
		s.Gofmt,
		"-mode=fix",
	)
	return s.inlineFixTool(cmd, in, outSink)
}

func (s *Server) inlineFixTool(cmd *exec.Cmd, in []File, outSink io.Writer) (out []FormattedFile, err error) {
	tmpDir, err := ioutil.TempDir("", "fmtserver")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	for _, f := range in {
		dir, base := filepath.Split(f.Name)
		dir = filepath.Join(tmpDir, dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}

		if err := ioutil.WriteFile(filepath.Join(dir, base), f.Content, 0644); err != nil {
			return nil, err
		}

		cmd.Args = append(cmd.Args, f.Name)
	}
	cmd.Dir = tmpDir
	cmd.Stdout = outSink
	cmd.Stderr = outSink
	log.Println("running", cmd.Args, "in", tmpDir)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	for _, f := range in {
		c, err := ioutil.ReadFile(filepath.Join(tmpDir, f.Name))
		if err != nil {
			return nil, err
		}

		out = append(out, FormattedFile{
			File: File{
				Name:    f.Name,
				Content: c,
			},
		})
	}

	return out, nil
}

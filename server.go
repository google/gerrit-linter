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

package gerritfmt

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
	"sort"
	"strings"
)

// Formatter is a definition of a formatting engine
type Formatter interface {
	// Format returns the files but formatted. All files are
	// assumed to have the same language.
	Format(in []File, outSink io.Writer) (out []FormattedFile, err error)
}

// FormatterConfig defines the mapping configurable
type FormatterConfig struct {
	// Regex is the typical filename regexp to use
	Regex *regexp.Regexp

	// Query is used to filter inside Gerrit
	Query string

	// The formatter
	Formatter Formatter
}

// Formatters holds all the formatters supported
var Formatters = map[string]*FormatterConfig{
	"commitmsg": {
		Regex:     regexp.MustCompile(`^/COMMIT_MSG$`),
		Formatter: &commitMsgFormatter{},
	},
}

func init() {
	// Add path to self to $PATH, for easy deployment.
	if exe, err := os.Executable(); err == nil {
		os.Setenv("PATH", filepath.Dir(exe)+":"+os.Getenv("PATH"))
	}

	gjf, err := exec.LookPath("google-java-format.jar")
	if err == nil {
		Formatters["java"] = &FormatterConfig{
			Regex: regexp.MustCompile(`\.java$`),
			Query: "ext:java",
			Formatter: &toolFormatter{
				bin:  "java",
				args: []string{"-jar", gjf, "-i"},
			},
		}
	} else {
		log.Printf("LookPath google-java-format: %v PATH=%s", err, os.Getenv("PATH"))
	}

	bzl, err := exec.LookPath("buildifier")
	if err == nil {
		Formatters["bzl"] = &FormatterConfig{
			Regex: regexp.MustCompile(`(\.bzl|/BUILD|^BUILD)$`),
			Query: "(ext:bzl OR file:BUILD OR file:WORKSPACE)",
			Formatter: &toolFormatter{
				bin:  bzl,
				args: []string{"-mode=fix"},
			},
		}
	} else {
		log.Printf("LookPath buildifier: %v, PATH=%s", err, os.Getenv("PATH"))
	}

	gofmt, err := exec.LookPath("gofmt")
	if err == nil {
		Formatters["go"] = &FormatterConfig{
			Regex: regexp.MustCompile(`\.go$`),
			Query: "ext:go",
			Formatter: &toolFormatter{
				bin:  gofmt,
				args: []string{"-w"},
			},
		}
	} else {
		log.Printf("LookPath gofmt: %v, PATH=%s", err, os.Getenv("PATH"))
	}
}

// IsSupported returns if the given language is supported.
func IsSupported(lang string) bool {
	_, ok := Formatters[lang]
	return ok
}

// SupportedLanguages returns a list of languages.
func SupportedLanguages() []string {
	var r []string
	for l := range Formatters {
		r = append(r, l)
	}
	sort.Strings(r)
	return r
}

func splitByLang(in []File) map[string][]File {
	res := map[string][]File{}
	for _, f := range in {
		res[f.Language] = append(res[f.Language], f)
	}
	return res
}

// Format formats all the files in the request for which a formatter exists.
func Format(req *FormatRequest, rep *FormatReply) error {
	for _, f := range req.Files {
		if f.Language == "" {
			return fmt.Errorf("file %q has empty language", f.Name)
		}
	}

	for language, fs := range splitByLang(req.Files) {
		var buf bytes.Buffer
		entry := Formatters[language]
		log.Println("init", Formatters)

		out, err := entry.Formatter.Format(fs, &buf)
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

type commitMsgFormatter struct{}

func (f *commitMsgFormatter) Format(in []File, outSink io.Writer) (out []FormattedFile, err error) {
	complaint := checkCommitMessage(string(in[0].Content))
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

func checkCommitMessage(msg string) (complaint string) {
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

type toolFormatter struct {
	bin  string
	args []string
}

func (f *toolFormatter) Format(in []File, outSink io.Writer) (out []FormattedFile, err error) {
	cmd := exec.Command(f.bin, f.args...)

	tmpDir, err := ioutil.TempDir("", "gerritfmt")
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

	var errBuf, outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	log.Println("running", cmd.Args, "in", tmpDir)
	if err := cmd.Run(); err != nil {
		log.Printf("error %v, stderr %s, stdout %s", err, errBuf.String(),
			outBuf.String())
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

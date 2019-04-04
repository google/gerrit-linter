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
	"flag"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os/exec"
	"strconv"

	"github.com/google/fmtserver"
)

func main() {
	port := flag.Int("port", 4001, "port to serve on")
	java := flag.String("java_jar", "", "where to find the google-java-format jar")
	buildifier := flag.String("buildifier", "", "where to find the buildifier")
	gofmt := flag.String("gofmt", "", "where to find gofmt")

	*java, _ = exec.LookPath("google-java-format-all-deps.jar")
	*buildifier, _ = exec.LookPath("buildifier")
	*gofmt, _ = exec.LookPath("gofmt")
	flag.Parse()

	s := fmtserver.NewServer()
	var err error
	s.JavaJar = *java
	s.Buildifier = *buildifier
	if err != nil {
		log.Fatal(err)
	}
	rpc.Register(s)
	rpc.HandleHTTP()

	addr := ":" + strconv.Itoa(*port)
	log.Printf("fmtserver serving on %s", addr)

	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("listen error:", err)
	}
	http.Serve(l, nil)

	log.Fatal(http.ListenAndServe(addr, nil))
}

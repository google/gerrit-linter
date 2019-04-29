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
	"encoding/json"
	"fmt"
	"log"
	"net/rpc"
	"strconv"
	"strings"
	"time"

	"github.com/google/gerritfmt"
	"github.com/google/gerritfmt/gerrit"
)

type gerritChecker struct {
	server    *gerrit.Server
	fmtClient *rpc.Client

	checkerUUIDs []string

	// XXX The similarity between PendingChecksInfo and PendingCheckInfo is annoying.
	todo chan *gerrit.PendingChecksInfo
}

func NewGerritChecker(server *gerrit.Server, fmtClient *rpc.Client) (*gerritChecker, error) {
	gc := &gerritChecker{
		server:    server,
		fmtClient: fmtClient,
		todo:      make(chan *gerrit.PendingChecksInfo, 5),
	}

	// XXX It would be nicer to query for the scheme "gerritfmt:"
	if out, err := ListCheckers(server); err != nil {
		return nil, err
	} else {
		for _, checker := range out {
			gc.checkerUUIDs = append(gc.checkerUUIDs, checker.UUID)
		}
	}

	go gc.pendingLoop()
	return gc, nil
}

func (c *gerritChecker) checkChange(changeID string, psID int) ([]string, error) {
	ch, err := c.server.GetChange(changeID, strconv.Itoa(psID))
	if err != nil {
		return nil, err
	}
	req := gerritfmt.FormatRequest{}
	for n, f := range ch.Files {
		req.Files = append(req.Files,
			gerritfmt.File{
				Name:    n,
				Content: f.Content,
			})
	}
	rep := gerritfmt.FormatReply{}
	if err := c.fmtClient.Call("Server.Format", &req, &rep); err != nil {
		_, ok := err.(rpc.ServerError)
		if ok {
			return nil, fmt.Errorf("server returned: %s", err)
		}
		return nil, err
	}

	var msgs []string
	for _, f := range rep.Files {
		orig := ch.Files[f.Name]
		if orig == nil {
			return nil, fmt.Errorf("result had unknown file %q", f.Name)
		}
		if !bytes.Equal(f.Content, orig.Content) {
			msg := f.Message
			if msg == "" {
				msg = "found a difference"
			}
			msgs = append(msgs, fmt.Sprintf("%s: %s", f.Name, msg))
			log.Printf("file %s: %s", f.Name, f.Message)
		} else {
			log.Printf("file %s: OK", f.Name)
		}
	}

	return msgs, nil
}

func (c *gerritChecker) pendingLoop() {
	for {
		for _, uuid := range c.checkerUUIDs {
			pending, err := c.server.PendingChecks(uuid)
			if err != nil {
				log.Printf("PendingChecks: %v", err)
				continue
			}
			if len(pending) == 0 {
				log.Printf("no pending checks")
			}

			for _, pc := range pending {
				select {
				case c.todo <- pc:
					log.Println("posted check.")
				default:
					log.Println("too busy; dropping pending check.")
				}
			}
		}
		// TODO: real rate limiting.
		time.Sleep(10 * time.Second)
	}
}

func (gc *gerritChecker) Serve() {
	for p := range gc.todo {
		// TODO: parallel.
		if err := gc.executeCheck(p); err != nil {
			log.Printf("executeCheck(%v): %v", p, err)
		}
	}
}

func (gc *gerritChecker) executeCheck(pc *gerrit.PendingChecksInfo) error {
	out, _ := json.Marshal(pc)
	fmt.Println("checking", string(out))

	changeID := strconv.Itoa(pc.PatchSet.ChangeNumber)
	psID := pc.PatchSet.PatchSetID
	for uuid := range pc.PendingChecks {
		checkInput := gerrit.CheckInput{
			CheckerUUID: uuid,
			State:       "RUNNING",
			Started:     gerrit.Timestamp(time.Now()),
		}
		_, err := gc.server.PostCheck(
			changeID, psID, &checkInput)
		if err != nil {
			return err
		}
	}

	status := "SUCCESSFUL"
	msgs, err := gc.checkChange(changeID, psID)
	if err != nil {
		msgs = []string{fmt.Sprintf("tool failure: %v", err)}

		// XXX would be nice to have a "TOOL_FAILURE" status
		status = "FAILED"
	} else if len(msgs) > 0 {
		status = "FAILED"
	}

	msg := strings.Join(msgs, ", ")
	if len(msg) > 80 {
		msg = msg[:77] + "..."
	}

	fmt.Printf("status %s for %v", status, pc.PatchSet)
	for uuid := range pc.PendingChecks {
		_, err := gc.server.PostCheck(changeID, psID,
			&gerrit.CheckInput{
				CheckerUUID: uuid,
				State:       status,
				Message:     msg,
			})
		if err != nil {
			return err
		}
	}
	return nil
}

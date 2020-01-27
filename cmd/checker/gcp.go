// Copyright 2020 Google Ltd. All rights reserved.
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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/gerritfmt/gerrit"
)

// The token as it comes from metadata service.
type gcpToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// tokenCache fetches a bearer token from the GCP metadata service,
// and refreshes it before it expires.
type tokenCache struct {
	account string

	mu      sync.Mutex
	current *gcpToken
}

// Implement the Authenticator interface.
func (tc *tokenCache) Authenticate(req *http.Request) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.current == nil {
		return fmt.Errorf("no token")
	}

	req.Header.Set("Authorization", "Bearer "+tc.current.AccessToken)
	return nil
}

// fetch gets the token from the metadata server.
func (tc *tokenCache) fetch() (*gcpToken, error) {
	u := fmt.Sprintf("http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/%s/token",
		tc.account)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := http.DefaultClient.Do(req.WithContext(context.Background()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	all, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s failed (%d): %s", req, resp.StatusCode, string(all))
	}

	tok := &gcpToken{}
	if err := json.Unmarshal(all, tok); err != nil {
		return nil, fmt.Errorf("can't unmarshal %s: %v", string(all), err)
	}

	return tok, nil
}

// NewGCPServiceAccount returns a Authenticator that will use GCP
// bearer-tokens. The tokens are refreshed automatically.
func NewGCPServiceAccount(account string) (gerrit.Authenticator, error) {
	tc := tokenCache{
		account: account,
	}

	tc.current, err := tc.fetch()
	if err != nil {
		return nil, err
	}

	go tc.loop()

	return &tc, nil
}

// loop refreshes the token periodically.
func (tc *tokenCache) loop() {
	delaySecs := tc.current.ExpiresIn - 1
	for {
		time.Sleep(time.Duration(delaySecs) * time.Second)
		tok, err := tc.fetch()
		if err != nil {
			log.Printf("fetching token failed:  %s", err)
			delaySecs = 2
		} else {
			delaySecs = tok.ExpiresIn - 1
		}

		tc.mu.Lock()
		tc.current = tok
		tc.mu.Unlock()
	}
}

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
	"strings"
	"sync"
	"time"

	"github.com/google/gerrit-linter/gerrit"
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

// The name of the scope that is necessary to access googlesource.com
// gerrit instances.
const gerritScope = "https://www.googleapis.com/auth/gerritcodereview"

// scopeURL returns the URL where GCP serves scopes for an account.
func scopeURL(account string) string {
	return fmt.Sprintf("http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/%s/scopes",
		account)
}

// tokenURL returns the URL where GCP serves tokens for an account.
func tokenURL(account string) string {
	return fmt.Sprintf("http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/%s/token",
		account)
}

// fetchScopes returns the scopes for the configured service account.
func (tc *tokenCache) fetchScopes() ([]string, error) {
	req, err := http.NewRequest("GET", scopeURL(tc.account), nil)
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

	return strings.Split(strings.TrimSpace(string(all)), "\n"), nil
}

// fetch gets the token from the metadata server.
func (tc *tokenCache) fetchToken() (*gcpToken, error) {
	req, err := http.NewRequest("GET", tokenURL(tc.account), nil)
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
		return nil, fmt.Errorf("%v failed (%d): %s", req, resp.StatusCode, string(all))
	}

	tok := &gcpToken{}
	if err := json.Unmarshal(all, tok); err != nil {
		return nil, fmt.Errorf("can't unmarshal %s: %v", string(all), err)
	}

	return tok, nil
}

// NewGCPServiceAccount returns a Authenticator that will use GCP
// bearer-tokens to authenticate against a googlesource.com Gerrit
// instance. The tokens are refreshed automatically.
func NewGCPServiceAccount(account string) (gerrit.Authenticator, error) {
	tc := tokenCache{
		account: account,
	}

	scopes, err := tc.fetchScopes()
	if err != nil {
		return nil, err
	}

	found := false
	for _, s := range scopes {
		if s == gerritScope {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("missing scope %q, got %q", gerritScope, scopes)
	}

	tc.current, err = tc.fetchToken()
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
		tok, err := tc.fetchToken()
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

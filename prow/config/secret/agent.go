/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package secret implements an agent to read and reload the secrets.
package secret

import (
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Agent watches a path and automatically loads the secrets stored.
type Agent struct {
	sync.RWMutex
	secretsMap map[string][]byte
}

// Start creates goroutines to monitor the files that contain the secret value.
func (a *Agent) Start(paths []string) error {
	secretsMap, err := LoadSecrets(paths)
	if err != nil {
		return err
	}

	a.secretsMap = secretsMap

	// Start one goroutine for each file to monitor and update the secret's values.
	for secretPath := range secretsMap {
		go a.reloadSecret(secretPath)
	}

	return nil
}

// reloadSecret will begin polling the secret file at the path. If the first load
// fails, Start with return the error and abort. Future load failures will log
// the failure message but continue attempting to load.
func (a *Agent) reloadSecret(secretPath string) {
	var lastModTime time.Time
	logger := logrus.NewEntry(logrus.StandardLogger())

	skips := 0
	for range time.Tick(1 * time.Second) {
		if skips < 600 {
			// Check if the file changed to see if it needs to be re-read.
			secretStat, err := os.Stat(secretPath)
			if err != nil {
				logger.WithField("secret-path", secretPath).
					WithError(err).Error("Error loading secret file.")
				continue
			}

			recentModTime := secretStat.ModTime()
			if !recentModTime.After(lastModTime) {
				skips++
				continue // file hasn't been modified
			}
			lastModTime = recentModTime
		}

		if secretValue, err := LoadSingleSecret(secretPath); err != nil {
			logger.WithField("secret-path: ", secretPath).
				WithError(err).Error("Error loading secret.")
		} else {
			a.setSecret(secretPath, secretValue)
			skips = 0
		}
	}
}

// GetSecret returns the value of a secret stored in a map.
func (a *Agent) GetSecret(secretPath string) []byte {
	a.RLock()
	defer a.RUnlock()
	return a.secretsMap[secretPath]
}

// setSecret sets a value in a map of secrets.
func (a *Agent) setSecret(secretPath string, secretValue []byte) {
	a.Lock()
	defer a.Unlock()
	a.secretsMap[secretPath] = secretValue
}

// GetTokenGenerator returns a function that gets the value of a given secret.
func (a *Agent) GetTokenGenerator(secretPath string) func() []byte {
	return func() []byte {
		return a.GetSecret(secretPath)
	}
}

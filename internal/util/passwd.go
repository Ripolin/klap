/*
Copyright 2025.

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

package util

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"regexp"
)

var HashedPassword = regexp.MustCompile(`^\{(SHA|SSHA|MD5|SMD5|CRYPT)\}`)

type SSHAEncoder struct{}

// Encode encodes the []byte of raw password
func (enc SSHAEncoder) Encode(rawPassPhrase []byte) ([]byte, error) {
	hash := makeSSHAHash(rawPassPhrase, makeSalt())
	b64 := base64.StdEncoding.EncodeToString(hash)
	return fmt.Appendf(nil, "{SSHA}%s", b64), nil
}

// makeSalt make a 4 byte array containing random bytes.
func makeSalt() []byte {
	sbytes := make([]byte, 4)
	rand.Read(sbytes)
	return sbytes
}

// makeSSHAHash make hasing using SHA-1 with salt. This is not the final output though. You need to append {SSHA} string with base64 of this hash.
func makeSSHAHash(passphrase, salt []byte) []byte {
	sha := sha1.New()
	sha.Write(passphrase)
	sha.Write(salt)
	h := sha.Sum(nil)
	return append(h, salt...)
}

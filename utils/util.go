// Contains utility functions, such as generating random election timeouts.

package utils

import (
	"math/rand"
	"time"
)

// Initializing the random number generator.
func Init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}

// Generates random election timeouts to prevent split votes.
// this is very crutial because if 2 notes have the same election timeout then
// both might try to become a candidate and try to become a leader which we dont want that to happen
func RandomElectionTimeout() time.Duration {
	return time.Duration(rand.Intn(1000)+500) * time.Millisecond // 500ms to 1500ms
}

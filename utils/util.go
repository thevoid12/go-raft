// Contains utility functions, such as generating random election timeouts.

package utils

import (
	"log"
	"math/rand"
	"time"
)

// Initializing the random number generator.
func Init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}

// Generates random election timeouts between min and max time to prevent split votes.
// this is very crutial because if 2 notes have the same election timeout then
// both might try to become a candidate and try to become a leader which we dont want that to happen
// RandomElectionTimeout generates a random election timeout between min and max.
// If max <= min, it returns min directly to prevent calling rand.Int63n(0).
func RandomElectionTimeout(min, max time.Duration) time.Duration {
	if max <= min {
		log.Printf("RandomElectionTimeout: max (%v) <= min (%v), returning min", max, min)
		return min
	}
	diff := int64(max - min)
	if diff <= 0 {
		log.Printf("RandomElectionTimeout: max (%v) - min (%v) <=0, returning min", max, min)
		return min
	}
	timeout := time.Duration(rand.Int63n(diff)) + min
	log.Printf("RandomElectionTimeout: selected timeout %v", timeout)
	return timeout
}

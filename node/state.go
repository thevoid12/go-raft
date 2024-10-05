// Defines different states (Follower, Candidate, Leader) and related logic.
package node

type State int

const (
	Follower State = iota
	Candidate
	Leader
)

func (s State) String() string {
	return [...]string{"Follower", "Candidate", "Leader"}[s]
}

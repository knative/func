package function

import (
	"sort"
	"sync"
)

// sorted set of strings.
//
// write-optimized and suitable only for fairly small values of N.
// Should this increase dramatically in size, a different implementation,
// such as a linked list, might be more appropriate.
type sortedSet struct {
	members map[string]bool
	sync.Mutex
}

func newSortedSet() *sortedSet {
	return &sortedSet{
		members: make(map[string]bool),
	}
}

func (s *sortedSet) Add(value string) {
	s.Lock()
	s.members[value] = true
	s.Unlock()
}

func (s *sortedSet) Remove(value string) {
	s.Lock()
	delete(s.members, value)
	s.Unlock()
}

func (s *sortedSet) Items() []string {
	s.Lock()
	defer s.Unlock()
	n := []string{}
	for k := range s.members {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

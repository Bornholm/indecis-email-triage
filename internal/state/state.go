// Package state remembers the messages already sorted, per mailbox, so
// that each is sorted once. A message is known by its UID, valid as long
// as the mailbox's UIDVALIDITY does not change.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// State is the content of the state file.
type State struct {
	Mailboxes map[string]*Mailbox `json:"mailboxes"`
	path      string
}

// Mailbox holds the sorted UIDs of a mailbox.
type Mailbox struct {
	UIDValidity uint32   `json:"uidvalidity"`
	Done        []uint32 `json:"done"`
	done        map[uint32]bool
}

// Load reads the state file, empty if it does not exist.
func Load(path string) (*State, error) {
	s := &State{Mailboxes: map[string]*Mailbox{}, path: path}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Mailboxes == nil {
		s.Mailboxes = map[string]*Mailbox{}
	}
	return s, nil
}

// Mailbox returns the state of a mailbox; a new UIDVALIDITY forgets its
// UIDs, which no longer designate the same messages.
func (s *State) Mailbox(name string, validity uint32) *Mailbox {
	mb := s.Mailboxes[name]
	if mb == nil || mb.UIDValidity != validity {
		mb = &Mailbox{UIDValidity: validity}
		s.Mailboxes[name] = mb
	}
	if mb.done == nil {
		mb.done = make(map[uint32]bool, len(mb.Done))
		for _, u := range mb.Done {
			mb.done[u] = true
		}
	}
	return mb
}

// IsDone reports whether a message was sorted.
func (mb *Mailbox) IsDone(uid uint32) bool { return mb.done[uid] }

// MarkDone records a sorted message.
func (mb *Mailbox) MarkDone(uid uint32) {
	if !mb.done[uid] {
		mb.done[uid] = true
		mb.Done = append(mb.Done, uid)
	}
}

// Save writes the state file, replacing it in one step.
func (s *State) Save() error {
	for _, mb := range s.Mailboxes {
		sort.Slice(mb.Done, func(i, j int) bool { return mb.Done[i] < mb.Done[j] })
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

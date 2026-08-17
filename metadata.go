package gofret

// Metadata records what a conversion did. Pass one to Codec.ToIntoMeta to
// collect it; nothing is tracked otherwise, so the default path stays cheap.
type Metadata struct {
	// Keys holds the dotted path of every destination field that was
	// written, for example "database.host".
	Keys []string
	// Unused holds the dotted path of every input key that matched no
	// destination field. It is the usual way to warn about typos in
	// configuration files.
	Unused []string
	// Unset holds the dotted path of every destination field that no input
	// key supplied.
	Unset []string
}

// Reset clears the metadata while keeping the allocated slices, so a single
// Metadata can be reused across conversions.
func (m *Metadata) Reset() {
	if m == nil {
		return
	}

	m.Keys = m.Keys[:0]
	m.Unused = m.Unused[:0]
	m.Unset = m.Unset[:0]
}

// The recorders below take the state rather than a string so that the path is
// only rendered when metadata was actually asked for. Building it eagerly cost
// an allocation per field on the hot path.

func (s *state) recordKey() {
	if s.meta != nil {
		s.meta.Keys = append(s.meta.Keys, s.pathString())
	}
}

func (s *state) recordUnused() {
	if s.meta != nil {
		s.meta.Unused = append(s.meta.Unused, s.pathString())
	}
}

func (s *state) recordUnset() {
	if s.meta != nil {
		s.meta.Unset = append(s.meta.Unset, s.pathString())
	}
}

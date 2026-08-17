package gofret

import (
	"fmt"
	"reflect"
)

// fieldInfo is the analysed form of one reachable struct field.
//
// Both conversion directions read this and nothing else, which is what keeps
// struct-to-map and map-to-struct from drifting apart.
type fieldInfo struct {
	// Name is the resolved map key.
	Name string
	// Norm is Name run through the codec's key normalizer, precomputed so
	// that matching never allocates.
	Norm string
	// Index is the path to the field, in the shape reflect.Value.FieldByIndex
	// expects. It has more than one element for inlined fields.
	Index []int
	// Type is the declared type of the field.
	Type reflect.Type
	// Tag is the parsed struct tag.
	Tag Tag
	// Anonymous reports whether the field is embedded.
	Anonymous bool
}

// structInfo is the cached analysis of a struct type for one codec.
type structInfo struct {
	Type   reflect.Type
	Fields []fieldInfo

	// byName maps the exact key to an index into Fields.
	byName map[string]int
	// byNorm maps the normalized key to an index into Fields. It is nil when
	// the codec has no normalizer.
	byNorm map[string]int

	// Remain is the index into Fields of the `remain` field, or -1.
	Remain int

	// Err is the analysis error. It is returned on every use of this type so
	// a bad tag is reported consistently rather than intermittently.
	Err error
}

// lookup finds the field for an incoming key: exact match first, then the
// normalized form. It returns -1 when no field claims the key.
//
// The second result reports whether the key matched a field name exactly,
// which lets the caller settle a clash between two keys that normalize alike
// without depending on map iteration order.
func (si *structInfo) lookup(key string, norm KeyNormalizer) (int, bool) {
	if i, ok := si.byName[key]; ok {
		return i, true
	}

	if si.byNorm == nil || norm == nil {
		return -1, false
	}

	if i, ok := si.byNorm[norm(key)]; ok {
		return i, false
	}

	return -1, false
}

// structInfoOf returns the cached analysis of t, computing it on first use.
func (c *Codec) structInfoOf(t reflect.Type) *structInfo {
	if v, ok := c.cache.Load(t); ok {
		return v.(*structInfo)
	}

	si := c.analyze(t)

	actual, _ := c.cache.LoadOrStore(t, si)

	return actual.(*structInfo)
}

// analyze walks t breadth-first so that shallower fields shadow deeper
// inlined ones, mirroring how Go resolves promoted fields.
func (c *Codec) analyze(t reflect.Type) *structInfo {
	si := &structInfo{
		Type:   t,
		byName: make(map[string]int),
		Remain: -1,
	}

	if c.cfg.normalizer != nil {
		si.byNorm = make(map[string]int)
	}

	type queued struct {
		typ   reflect.Type
		index []int
	}

	var (
		errs     errorList
		expanded = map[reflect.Type]bool{t: true}
		level    = []queued{{typ: t}}
	)

	for len(level) > 0 {
		var next []queued

		for _, q := range level {
			for i := range q.typ.NumField() {
				sf := q.typ.Field(i)

				fi, inlineOf, err := c.analyzeField(sf, q.index, i)
				if err != nil {
					errs.add(fmt.Errorf("%s.%s: %w", q.typ, sf.Name, err))

					continue
				}

				switch {
				case fi == nil && inlineOf == nil:
					// Field is skipped.
				case inlineOf != nil:
					// Expand each struct type at most once so that a type
					// that inlines itself cannot loop forever.
					if expanded[inlineOf] {
						continue
					}

					expanded[inlineOf] = true
					next = append(next, queued{typ: inlineOf, index: fi.Index})
				case fi.Tag.Has(OptRemain):
					if si.Remain >= 0 {
						errs.add(fmt.Errorf("%s.%s: duplicate `remain` field, already set by %q",
							q.typ, sf.Name, si.Fields[si.Remain].Name))

						continue
					}

					si.Remain = len(si.Fields)
					si.Fields = append(si.Fields, *fi)
				default:
					si.addField(*fi)
				}
			}
		}

		level = next
	}

	if !errs.empty() {
		si.Err = fmt.Errorf("%w: %w", ErrInvalidTag, errs.err())
	}

	return si
}

// addField records fi unless a shallower field already claimed the key.
func (si *structInfo) addField(fi fieldInfo) {
	if _, taken := si.byName[fi.Name]; taken {
		return
	}

	idx := len(si.Fields)

	si.Fields = append(si.Fields, fi)
	si.byName[fi.Name] = idx

	// A normalized collision keeps the first declaration, so lookups stay
	// deterministic instead of depending on map iteration order.
	if si.byNorm != nil {
		if _, taken := si.byNorm[fi.Norm]; !taken {
			si.byNorm[fi.Norm] = idx
		}
	}
}

// analyzeField classifies one struct field.
//
// It returns (nil, nil, nil) when the field is skipped, and a non-nil second
// result when the field must be inlined into its parent, in which case the
// returned fieldInfo carries only the index path.
func (c *Codec) analyzeField(sf reflect.StructField, parent []int, i int) (*fieldInfo, reflect.Type, error) {
	raw, tagged := c.lookupTag(sf)

	tag, err := parseTag(raw)
	if err != nil {
		return nil, nil, err
	}

	if tag.Has(OptSkip) {
		return nil, nil, nil
	}

	index := make([]int, len(parent)+1)
	copy(index, parent)
	index[len(parent)] = i

	inline := tag.Has(OptInline) || (c.cfg.inlineEmbedded && sf.Anonymous)

	// A field embedded under an unexported type name cannot be addressed as a
	// whole, so promoting its exported fields is the only reading that leaves
	// anything reachable. This matches how encoding/json treats it.
	if sf.Anonymous && !sf.IsExported() && indirectKind(sf.Type) == reflect.Struct {
		inline = true
	}

	if !sf.IsExported() && !inline {
		return nil, nil, nil
	}

	if inline {
		st := sf.Type
		for st.Kind() == reflect.Pointer {
			st = st.Elem()
		}

		if st.Kind() != reflect.Struct {
			return nil, nil, fmt.Errorf("`inline` requires a struct or pointer to struct, got %s", sf.Type)
		}

		return &fieldInfo{Index: index}, st, nil
	}

	if !tagged && c.cfg.taggedOnly {
		return nil, nil, nil
	}

	fi := fieldInfo{
		Name:      c.keyName(sf.Name, tag),
		Index:     index,
		Type:      sf.Type,
		Tag:       tag,
		Anonymous: sf.Anonymous,
	}

	if c.cfg.normalizer != nil {
		fi.Norm = c.cfg.normalizer(fi.Name)
	}

	return &fi, nil, nil
}

// indirectKind reports the kind of t after following any pointers.
func indirectKind(t reflect.Type) reflect.Kind {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Kind()
}

// keyName resolves the map key for a field: the tag name if given, otherwise
// the field name run through the codec's key function.
func (c *Codec) keyName(fieldName string, tag Tag) string {
	if tag.Name != "" {
		return tag.Name
	}

	if c.cfg.keyFunc != nil {
		return c.cfg.keyFunc(fieldName)
	}

	return fieldName
}

// lookupTag reads the primary tag, falling back to the configured
// alternatives. The boolean reports whether any of them was present.
func (c *Codec) lookupTag(sf reflect.StructField) (string, bool) {
	if raw, ok := sf.Tag.Lookup(c.cfg.tag); ok {
		return raw, true
	}

	for _, name := range c.cfg.tagFallback {
		if raw, ok := sf.Tag.Lookup(name); ok {
			return raw, true
		}
	}

	return "", false
}

// fieldByIndexRead walks an index path for reading. It reports false when a
// nil pointer makes the field unreachable.
func fieldByIndexRead(v reflect.Value, index []int) (reflect.Value, bool) {
	for i, x := range index {
		if i > 0 {
			if v.Kind() == reflect.Pointer {
				if v.IsNil() {
					return reflect.Value{}, false
				}

				v = v.Elem()
			}
		}

		v = v.Field(x)
	}

	return v, true
}

// fieldByIndexWrite walks an index path for writing, allocating any nil
// pointers found along the way.
func fieldByIndexWrite(v reflect.Value, index []int) (reflect.Value, error) {
	for i, x := range index {
		if i > 0 {
			if v.Kind() == reflect.Pointer {
				if v.IsNil() {
					if !v.CanSet() {
						return reflect.Value{}, fmt.Errorf("cannot allocate embedded %s", v.Type())
					}

					v.Set(reflect.New(v.Type().Elem()))
				}

				v = v.Elem()
			}
		}

		v = v.Field(x)
	}

	return v, nil
}

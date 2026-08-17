package gofret

import (
	"fmt"
	"reflect"
)

// toStruct writes into a struct destination.
func (s *state) toStruct(src, dst reflect.Value) {
	if src.Type() == dst.Type() {
		dst.Set(src)

		return
	}

	switch src.Kind() {
	case reflect.Map:
		s.structFromMap(src, dst)
	case reflect.Struct:
		s.structFromStruct(src, dst)
	default:
		s.fail(src, dst, fmt.Errorf("%w: expected a map or struct", ErrUnconvertible))
	}
}

// convertField converts one struct field, applying the options that only
// make sense at field level.
func (s *state) convertField(src, dst reflect.Value, tag Tag) {
	// `string` says this field travels as text, so parsing it back is allowed
	// even when the codec is otherwise strict.
	if tag.Has(OptString) && !s.weak {
		s.weak = true
		defer func() { s.weak = false }()
	}

	s.convert(src, dst, tag)
}

// ---------------------------------------------------------------------------
// struct -> map
// ---------------------------------------------------------------------------

// mapFromStruct writes the fields of a struct into a map.
func (s *state) mapFromStruct(src, dst reflect.Value) {
	si := s.c.structInfoOf(src.Type())
	if si.Err != nil {
		s.fail(src, dst, si.Err)

		return
	}

	s.ensureMap(dst)

	keyType := dst.Type().Key()
	elemType := dst.Type().Elem()

	// The overwhelmingly common map is keyed by plain string, where the field
	// name is already the key and needs no conversion at all.
	plainKey := keyType.Kind() == reflect.String && keyType == stringType

	s.eachField(src, si, func(name string, fv reflect.Value, tag Tag) bool {
		if tag.Has(OptOmitEmpty) && isEmptyValue(fv) {
			return true
		}

		if s.c.cfg.omitNil && isNilLike(fv) {
			return true
		}

		if tag.Has(OptDeref) || s.c.cfg.derefPointers {
			fv = derefOrZero(fv)
		}

		s.push(name)

		ok := s.writeMapEntry(dst, keyType, elemType, plainKey, name, fv, tag)

		s.pop()

		return ok
	})
}

func (s *state) writeMapEntry(
	dst reflect.Value,
	keyType, elemType reflect.Type,
	plainKey bool,
	name string,
	fv reflect.Value,
	tag Tag,
) bool {
	key := reflect.ValueOf(name)
	if !plainKey {
		key = reflect.New(keyType).Elem()

		s.convert(reflect.ValueOf(name), key, Tag{})
	}

	elem := reflect.New(elemType).Elem()

	if tag.Has(OptString) {
		str, err := stringify(fv)
		if err != nil {
			return !s.fail(fv, elem, err)
		}

		s.convert(reflect.ValueOf(str), elem, Tag{})
	} else {
		s.convert(fv, elem, tag)
	}

	if s.errs.stopped {
		return false
	}

	s.recordKey()

	dst.SetMapIndex(key, elem)

	return true
}

// derefOrZero follows a pointer, substituting the zero value for nil so that
// `deref` never drops a key.
func derefOrZero(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Zero(v.Type().Elem())
		}

		v = v.Elem()
	}

	return v
}

// eachField walks the readable fields of a struct in declaration order,
// expanding the `remain` map so its entries look like ordinary fields.
//
// Both struct-to-map and struct-to-struct go through here, which is what
// keeps the two directions consistent.
func (s *state) eachField(src reflect.Value, si *structInfo, fn func(name string, v reflect.Value, tag Tag) bool) bool {
	for i := range si.Fields {
		fi := &si.Fields[i]

		fv, ok := fieldByIndexRead(src, fi.Index)
		if !ok {
			// An embedded nil pointer makes the field unreachable.
			continue
		}

		if i == si.Remain {
			if !s.eachRemain(fv, fn) {
				return false
			}

			continue
		}

		if !fn(fi.Name, fv, fi.Tag) {
			return false
		}
	}

	return true
}

// eachRemain replays the entries of a `remain` map as if they were fields, so
// that keys captured on the way in are written back out.
func (s *state) eachRemain(fv reflect.Value, fn func(string, reflect.Value, Tag) bool) bool {
	fv = derefOrZero(fv)

	if fv.Kind() != reflect.Map || fv.IsNil() {
		return true
	}

	iter := fv.MapRange()
	for iter.Next() {
		if !fn(keyLabel(iter.Key()), iter.Value(), Tag{}) {
			return false
		}
	}

	return true
}

// ---------------------------------------------------------------------------
// map -> struct
// ---------------------------------------------------------------------------

func (s *state) structFromMap(src, dst reflect.Value) {
	si := s.c.structInfoOf(dst.Type())
	if si.Err != nil {
		s.fail(src, dst, si.Err)

		return
	}

	// Match first, convert second.
	//
	// Converting inside the map walk would tie the order of the work, and so
	// the order of any errors and of the metadata, to Go's randomised map
	// iteration. Collecting the matches and then walking the fields in
	// declaration order makes the result stable from run to run, at the cost
	// of one slice that also does the job of tracking which fields were seen.
	vals := make([]reflect.Value, len(si.Fields))

	var leftover []reflect.Value

	iter := src.MapRange()
	for iter.Next() {
		idx, exact := si.lookup(keyLabel(iter.Key()), s.c.cfg.normalizer)
		if idx < 0 || idx == si.Remain {
			leftover = append(leftover, iter.Key())

			continue
		}

		// Two keys can fold to the same field. An exact name match is the
		// more specific one and takes the slot whichever order they turned up
		// in; no extra bookkeeping is needed because map keys are unique, so
		// at most one of them can match a given field name exactly.
		if vals[idx].IsValid() && !exact {
			continue
		}

		vals[idx] = iter.Value()
	}

	for i := range si.Fields {
		if !vals[i].IsValid() {
			continue
		}

		if !s.assignField(dst, &si.Fields[i], vals[i]) {
			return
		}
	}

	s.finishStruct(src, dst, si, vals, leftover)
}

// assignField writes one value into a struct field. It reports whether
// conversion should continue.
func (s *state) assignField(dst reflect.Value, fi *fieldInfo, val reflect.Value) bool {
	fv, err := fieldByIndexWrite(dst, fi.Index)
	if err != nil {
		return !s.fail(val, dst, err)
	}

	if !fv.CanSet() {
		return true
	}

	s.push(fi.Name)
	s.convertField(val, fv, fi.Tag)
	s.recordKey()
	s.pop()

	return !s.errs.stopped
}

// finishStruct deals with the keys no field claimed and records metadata.
// A field is considered supplied when its slot in vals holds a value.
func (s *state) finishStruct(src, dst reflect.Value, si *structInfo, vals []reflect.Value, leftover []reflect.Value) {
	if len(leftover) > 0 {
		switch {
		case si.Remain >= 0:
			s.fillRemain(src, dst, si, leftover)
		default:
			for _, k := range leftover {
				s.push(keyLabel(k))
				s.recordUnused()
				s.pop()
			}

			if s.c.cfg.errorUnused {
				s.fail(src, dst, fmt.Errorf("%w: %s", ErrUnusedKeys, joinKeys(leftover)))
			}
		}
	}

	if s.meta == nil {
		return
	}

	for i := range si.Fields {
		if vals[i].IsValid() || i == si.Remain {
			continue
		}

		s.push(si.Fields[i].Name)
		s.recordUnset()
		s.pop()
	}
}

// fillRemain gathers the unclaimed keys into the `remain` field.
func (s *state) fillRemain(src, dst reflect.Value, si *structInfo, leftover []reflect.Value) {
	fi := &si.Fields[si.Remain]

	fv, err := fieldByIndexWrite(dst, fi.Index)
	if err != nil {
		s.fail(src, dst, err)

		return
	}

	if !fv.CanSet() {
		return
	}

	target := fv
	for target.Kind() == reflect.Pointer {
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}

		target = target.Elem()
	}

	if target.Kind() != reflect.Map {
		s.fail(src, fv, fmt.Errorf("%w: `remain` requires a map, got %s", ErrUnsupportedType, fi.Type))

		return
	}

	rest := reflect.MakeMapWithSize(src.Type(), len(leftover))
	for _, k := range leftover {
		rest.SetMapIndex(k, src.MapIndex(k))
	}

	s.push(fi.Name)
	s.convert(rest, target, fi.Tag)
	s.pop()
}

func joinKeys(keys []reflect.Value) string {
	out := make([]byte, 0, len(keys)*8)

	for i, k := range keys {
		if i > 0 {
			out = append(out, ", "...)
		}

		out = append(out, keyLabel(k)...)
	}

	return string(out)
}

// ---------------------------------------------------------------------------
// struct -> struct
// ---------------------------------------------------------------------------

// structFromStruct copies field by field using both type analyses, without
// building an intermediate map.
func (s *state) structFromStruct(src, dst reflect.Value) {
	ssi := s.c.structInfoOf(src.Type())
	if ssi.Err != nil {
		s.fail(src, dst, ssi.Err)

		return
	}

	dsi := s.c.structInfoOf(dst.Type())
	if dsi.Err != nil {
		s.fail(src, dst, dsi.Err)

		return
	}

	seen := make([]bool, len(dsi.Fields))
	rest := make(map[string]reflect.Value)

	s.eachField(src, ssi, func(name string, fv reflect.Value, tag Tag) bool {
		if tag.Has(OptOmitEmpty) && isEmptyValue(fv) {
			return true
		}

		if s.c.cfg.omitNil && isNilLike(fv) {
			return true
		}

		if tag.Has(OptDeref) || s.c.cfg.derefPointers {
			fv = derefOrZero(fv)
		}

		idx, _ := dsi.lookup(name, s.c.cfg.normalizer)
		if idx < 0 || idx == dsi.Remain {
			rest[name] = fv

			return true
		}

		seen[idx] = true

		return s.assignField(dst, &dsi.Fields[idx], fv)
	})

	if s.errs.stopped {
		return
	}

	s.finishStructPairs(src, dst, dsi, seen, rest)
}

// finishStructPairs is finishStruct for a struct source, where the unclaimed
// values are already keyed by name.
func (s *state) finishStructPairs(src, dst reflect.Value, dsi *structInfo, seen []bool, rest map[string]reflect.Value) {
	if len(rest) > 0 {
		if dsi.Remain >= 0 {
			m := reflect.MakeMapWithSize(mapStringAnyType, len(rest))
			for k, v := range rest {
				m.SetMapIndex(reflect.ValueOf(k), toAny(v))
			}

			fi := &dsi.Fields[dsi.Remain]

			fv, err := fieldByIndexWrite(dst, fi.Index)
			if err != nil {
				s.fail(src, dst, err)

				return
			}

			s.push(fi.Name)
			s.convert(m, fv, fi.Tag)
			s.pop()
		} else {
			for k := range rest {
				s.push(k)
				s.recordUnused()
				s.pop()
			}

			if s.c.cfg.errorUnused {
				s.fail(src, dst, fmt.Errorf("%w: %d unmatched field(s)", ErrUnusedKeys, len(rest)))
			}
		}
	}

	if s.meta == nil {
		return
	}

	for i := range dsi.Fields {
		if seen[i] || i == dsi.Remain {
			continue
		}

		s.push(dsi.Fields[i].Name)
		s.recordUnset()
		s.pop()
	}
}

// toAny boxes a value so it can be stored in a map[string]any.
func toAny(v reflect.Value) reflect.Value {
	out := reflect.New(anyType).Elem()

	if v.IsValid() {
		out.Set(v)
	}

	return out
}

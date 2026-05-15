package persiststore

import (
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// pathElem represents a parsed path element with an optional module hint.
//
// The module is derived from a prefix in the form "<module>:<name>" when present.
// The name field always contains the unprefixed element name.
type pathElem struct {
	name   string // unprefixed name
	module string // optional module hint
}

// parsePathElems parses gNMI-like path elements that may optionally carry a module
// prefix in the form "<module>:<name>".
//
// It returns a slice of pathElem values where:
// - name is always the unprefixed element name
// - module is set only when a prefix was present
//
// Note: only the first ':' is treated as the prefix separator.

func parsePathElems(pes []string) []pathElem {
	out := make([]pathElem, 0, len(pes))
	for _, pe := range pes {
		if i := strings.IndexByte(pe, ':'); i > 0 {
			out = append(out, pathElem{
				module: pe[:i],
				name:   pe[i+1:],
			})
		} else {
			out = append(out, pathElem{
				name: pe,
			})
		}
	}
	return out
}

// uniqueStrings returns the input slice with duplicates removed while preserving
// the original order of first occurrence.

func uniqueStrings(in []string) []string {
	m := make(map[string]struct{})
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := m[s]; !ok {
			m[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// schemaElemModuleName extracts the module name from a SchemaElem, regardless of
// whether the element is a container, leaf, or leaf-list.
//
// If the schema element is nil or of an unknown/unsupported oneof type, an empty
// string is returned.

func schemaElemModuleName(sce *sdcpb.SchemaElem) string {
	switch s := sce.Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		return s.Container.ModuleName
	case *sdcpb.SchemaElem_Leaflist:
		return s.Leaflist.ModuleName
	case *sdcpb.SchemaElem_Field:
		return s.Field.ModuleName
	default:
		return ""
	}
}

// hasPrefix reports whether the provided path element contains a module prefix.
// A prefix is identified by the presence of ':' anywhere in the string.

func hasPrefix(pe string) bool {
	return strings.Contains(pe, ":")
}

// stripPrefix removes the module prefix from a path element of the form
// "<module>:<name>".
//
// If no ':' is present, the input is returned unchanged.

func stripPrefix(pe string) string {
	if i := strings.IndexByte(pe, ':'); i != -1 {
		return pe[i+1:]
	}
	return pe
}

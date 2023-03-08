package conversion

import (
	"fmt"
	"math"
	"regexp"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

func Convert(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	switch lst.Type {
	case "string":
		// TODO: length
		return ConvertString(value, lst)
	case "union":
		return ConvertUnion(value, lst.UnionTypes)
	case "boolean":
		return ConvertBoolean(value, lst)
	case "int8":
		// TODO: HEX and OCTAL pre-processing for all INT types
		// https://www.rfc-editor.org/rfc/rfc6020.html#page-112
		return ConvertInt8(value, lst)
	case "int16":
		return ConvertInt16(value, lst)
	case "int32":
		return ConvertInt32(value, lst)
	case "int64":
		return ConvertInt64(value, lst)
	case "uint8":
		return ConvertUint8(value, lst)
	case "uint16":
		return ConvertUint16(value, lst)
	case "uint32":
		return ConvertUint32(value, lst)
	case "uint64":
		// TODO: fraction-digits (https://www.rfc-editor.org/rfc/rfc6020.html#section-9.3.4)
		return ConvertUint64(value, lst)
	case "enumeration":
	case "bits":
	// TODO: https://www.rfc-editor.org/rfc/rfc6020.html#section-9.7
	case "binary": // TODO: https://www.rfc-editor.org/rfc/rfc6020.html#section-9.8
	case "leafref": // TODO: https://www.rfc-editor.org/rfc/rfc6020.html#section-9.9
	case "identityref": //TODO: https://www.rfc-editor.org/rfc/rfc6020.html#section-9.10
	case "instance-identifier": //TODO: https://www.rfc-editor.org/rfc/rfc6020.html#section-9.13
	}
	log.Errorf("type %q not implemented", lst.Type)
	return ConvertString(value, lst)
}

func ConvertBoolean(value string, _ *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	var bval bool
	switch value {
	case "true":
		bval = true
	case "false":
		bval = false
	default:
		return nil, fmt.Errorf("illegal value %q for boolean type", value)
	}
	return &schemapb.TypedValue{
		Value: &schemapb.TypedValue_BoolVal{
			BoolVal: bval,
		},
	}, nil
}
func ConvertUint8(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewUrnges(lst.Range, 0, math.MaxUint8)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertUint16(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewUrnges(lst.Range, 0, math.MaxUint16)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertUint32(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewUrnges(lst.Range, 0, math.MaxUint32)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertUint64(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewUrnges(lst.Range, 0, math.MaxUint64)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertInt8(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewSrnges(lst.Range, math.MinInt8, math.MaxInt8)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertInt16(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewSrnges(lst.Range, math.MinInt16, math.MaxInt16)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertInt32(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewSrnges(lst.Range, math.MinInt32, math.MaxInt32)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}
func ConvertInt64(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewSrnges(lst.Range, math.MinInt64, math.MaxInt64)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertString(value string, lst *schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	overallMatch := true
	// If the type has multiple "pattern" statements, the expressions are
	// ANDed together, i.e., all such expressions have to match.
	for _, sp := range lst.Patterns {
		re, err := regexp.Compile(sp.Pattern)
		if err != nil {
			log.Errorf("unable to compile regex %q", sp.Pattern)
		}
		match := re.MatchString(value)
		// if it is a match and not inverted
		// or it is not a match but inverted
		// then this is valid
		if (match && !sp.Inverted) || (!match && sp.Inverted) {
			continue
		} else {
			overallMatch = false
			break
		}
	}
	if overallMatch {
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_StringVal{
				StringVal: value,
			},
		}, nil
	}
	return nil, fmt.Errorf("%q does not match patterns", value)

}

func ConvertUnion(value string, slts []*schemapb.SchemaLeafType) (*schemapb.TypedValue, error) {
	// iterate over the union types try to convert without error
	for _, slt := range slts {
		tv, err := Convert(value, slt)
		// if no error type conversion was fine
		if err != nil {
			continue
		}
		// return the TypedValue
		return tv, nil
	}
	return nil, fmt.Errorf("no union type fit the provided value %q", value)
}

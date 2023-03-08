package conversion

import (
	"fmt"
	"strconv"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

// urnges represents a collection of rng (range)
type SRnges struct {
	rnges []*SRng
}

// urng represents a single unsigned range
type SRng struct {
	min int64
	max int64
}

func NewSrnges(rangeDefinition string, min, max int64) *SRnges {
	r := &SRnges{}
	r.parse(rangeDefinition, min, max)
	return r
}

func (r *SRng) isInRange(value int64) bool {
	// return the result
	return r.min <= value && value <= r.max
}

func (r *SRnges) isWithinAnyRange(value string) (*schemapb.TypedValue, error) {
	intValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}

	// create the TypedValue already
	tv := &schemapb.TypedValue{
		Value: &schemapb.TypedValue_IntVal{
			IntVal: intValue,
		},
	}
	// if no ranges defined, return the tv
	if len(r.rnges) == 0 {
		return tv, nil
	}
	// check the ranges
	for _, rng := range r.rnges {
		if rng.isInRange(intValue) {
			return tv, nil
		}
	}
	return nil, fmt.Errorf("%q not within ranges", value)
}

func (r *SRnges) parse(rangeDef string, min, max int64) error {
	// to make sure the value is in the general limits of the datatype uint8|16|32|64
	// we add the min max as a seperate additional range
	r.rnges = append(r.rnges, &SRng{
		min: min,
		max: max,
	})

	// process all the schema based range definitions
	rangeStrings := strings.Split(rangeDef, "|")
	for _, rangeString := range rangeStrings {
		range_minmax := strings.Split(rangeString, "..")
		if len(range_minmax) != 2 {
			return fmt.Errorf("illegal range expression %q", rangeString)
		}

		var err error
		if range_minmax[0] != "min" {
			min, err = strconv.ParseInt(range_minmax[0], 10, 64)
			if err != nil {
				return err
			}
		}
		if range_minmax[1] != "max" {
			max, err = strconv.ParseInt(range_minmax[1], 10, 64)
			if err != nil {
				return err
			}
		}
		r.rnges = append(r.rnges, &SRng{
			min: min,
			max: max,
		})
	}
	return nil
}

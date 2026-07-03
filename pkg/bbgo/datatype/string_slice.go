package datatype

import (
	"encoding/json"
	"fmt"
)

type StringSlice []string

func (s *StringSlice) decode(a any) error {
	switch d := a.(type) {
	case string:
		*s = append(*s, d)

	case []string:
		*s = append(*s, d...)

	case []any:
		for _, de := range d {
			if err := s.decode(de); err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("unexpected type %T for StringSlice: %+v", d, d)
	}

	return nil
}

func (s *StringSlice) UnmarshalYAML(unmarshal func(any) error) (err error) {
	var ss []string
	err = unmarshal(&ss)
	if err == nil {
		*s = ss
		return
	}

	var as string
	err = unmarshal(&as)
	if err == nil {
		*s = append(*s, as)
	}

	return err
}

func (s *StringSlice) UnmarshalJSON(b []byte) error {
	var a any
	var err = json.Unmarshal(b, &a)
	if err != nil {
		return err
	}

	return s.decode(a)
}

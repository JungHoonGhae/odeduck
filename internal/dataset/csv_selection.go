package dataset

import "fmt"

// CSVSelection combines fields with AND and values within In with OR. It is a
// bounded exact-string predicate, not an expression language or provider query.
// Full scans use this contract; legacy bounded readers still accept Equals only.
type CSVSelection struct {
	Equals map[string]string
	In     map[string][]string
}

func (s CSVSelection) Validate() error {
	_, err := s.compile()
	return err
}

func (s CSVSelection) compile() (map[string]map[string]bool, error) {
	if err := ValidateCSVSelection(s.Equals); err != nil {
		return nil, err
	}
	if len(s.Equals)+len(s.In) > 8 {
		return nil, fmt.Errorf("CSV selection allows at most 8 fields across equality and value sets")
	}
	fields := map[string]map[string]bool{}
	for field, value := range s.Equals {
		fields[field] = map[string]bool{value: true}
	}
	for field, values := range s.In {
		if fields[field] != nil || len(values) < 1 || len(values) > 32 {
			return nil, fmt.Errorf("CSV value sets require 1–32 distinct values per field and cannot overlap equality fields")
		}
		allowed := map[string]bool{}
		for _, value := range values {
			if !validCSVSelectionTerm(field, value) || allowed[value] {
				return nil, fmt.Errorf("CSV value sets require distinct nonblank field/value strings of at most 256 bytes")
			}
			allowed[value] = true
		}
		fields[field] = allowed
	}
	return fields, nil
}

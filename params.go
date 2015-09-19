package router

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Params is similar to url.Values, but with a few more utility functions
type Params map[string][]string

// Map gets the params as a flat map[string]string, discarding any multiple values.
func (p Params) Map() map[string]string {
	flat := make(map[string]string)

	for k, v := range p {
		flat[k] = v[0]
	}

	return flat
}

// Flatten deflates a set of params (of any type) to a comma separated list (only for simple params)
func (p Params) Flatten(k string) string {
	flat := ""

	for i, v := range p[k] {
		if i > 0 {
			flat = fmt.Sprintf("%s,%s", flat, v)
		} else {
			flat = v
		}
	}

	// replace this key with an array containing only flat
	p[k] = []string{flat}

	return flat
}

// GetDate returns the first value associated with a given key as a time, using the given time format.
func (p Params) GetDate(key string, format string) (time.Time, error) {
	v := p.Get(key)
	return time.Parse(format, v)
}

// GetInt returns the first value associated with the given key as an integer. If there is no value or a parse error, it returns 0
// If the string contains non-numeric characters, they are first stripped
func (p Params) GetInt(key string) int64 {
	var i int64
	v := p.Get(key)
	// We truncate the string at the first non-numeric character
	v = v[0 : strings.LastIndexAny(v, "0123456789")+1]
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return i
}

// GetInts returns all values associated with the given key as an array of integers.
func (p Params) GetInts(key string) []int64 {
	ints := []int64{}

	for _, v := range p.GetAll(key) {
		vi, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			vi = 0
		}
		ints = append(ints, vi)
	}

	return ints
}

// GetUniqueInts returns all unique non-zero int values associated with the given key as an array of integers
func (p Params) GetUniqueInts(key string) []int64 {
	ints := []int64{}

	for _, v := range p.GetAll(key) {
		if string(v) == "" {
			continue // ignore blank ints
		}
		vi, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			vi = 0
		}
		if !contains(ints, vi) {
			ints = append(ints, vi)
		}
	}

	return ints
}

// GetIntsString returns all values associated with the given key as a comma separated string
func (p Params) GetIntsString(key string) string {
	ints := ""

	for _, v := range p.GetAll(key) {
		if "" == string(v) {
			continue // ignore blank ints
		}

		if len(ints) > 0 {
			ints += "," + string(v)
		} else {
			ints += string(v)
		}

	}

	return ints
}

// GetAll returns all values associated with the given key - equivalent to params[key].
func (p Params) GetAll(key string) []string {
	return p[key]
}

// Get gets the first value associated with the given key.
// If there are no values returns the empty string.
func (p Params) Get(key string) string {
	if p == nil {
		return ""
	}

	v := p[key]

	if v == nil || len(v) == 0 {
		return ""
	}

	return v[0]
}

// Blank returns true if the value corresponding to key is an empty string
func (p Params) Blank(key string) bool {
	v := p.Get(key)
	return v == ""
}

// Set sets the key to a string value replacing any existing values.
func (p Params) Set(key, value string) {
	p[key] = []string{value}
}

// SetInt sets the key to a single int value as string replacing any existing values.
func (p Params) SetInt(key string, value int64) {
	p[key] = []string{fmt.Sprintf("%d", value)}
}

// Add adds the value, if necessary appending to any existing values associated with key.
func (p Params) Add(key, value string) {
	p[key] = append(p[key], value)
}

// Remove deletes the values associated with key.
func (p Params) Remove(key string) {
	delete(p, key)
}

// Contains returns true if this array of ints contains the given int
func contains(list []int64, item int64) bool {
	for _, b := range list {
		if b == item {
			return true
		}
	}
	return false
}

// Copyright © 2016 Asteris, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package preprocessor

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/asteris-llc/converge/graph"
	"github.com/asteris-llc/converge/parse"
	"github.com/asteris-llc/converge/resource/module"
)

// ErrUnresolvable indicates that a field exists but is unresolvable due to nil
// references
var ErrUnresolvable = errors.New("field is unresolvable")

// fieldMapCache caches the results of field map generation to avoid
// recalculating it during execution.
type lockedFieldMapCache struct {
	innerLock *sync.RWMutex
	vals      map[reflect.Type]map[string]string
}

var fieldMapCache = makeCache()

func makeCache() *lockedFieldMapCache {
	return &lockedFieldMapCache{innerLock: new(sync.RWMutex), vals: make(map[reflect.Type]map[string]string)}
}

func (f *lockedFieldMapCache) Get(t reflect.Type) (map[string]string, bool) {
	f.innerLock.Lock()
	defer f.innerLock.Unlock()
	val, ok := f.vals[t]
	return val, ok
}

func (f *lockedFieldMapCache) Put(t reflect.Type, m map[string]string) {
	f.innerLock.Lock()
	defer f.innerLock.Unlock()
	f.vals[t] = m
}

// Preprocessor is a template preprocessor
type Preprocessor struct {
	vertices map[string]struct{}
}

// New creates a new preprocessor for the specified graph
func New(g *graph.Graph) *Preprocessor {
	m := make(map[string]struct{})
	for _, vertex := range g.Vertices() {
		m[vertex] = struct{}{}
	}
	return &Preprocessor{m}
}

// SplitTerms takes a string and splits it on '.'
func SplitTerms(in string) []string {
	return strings.Split(in, ".")
}

// JoinTerms takes a list of terms and joins them with '.'
func JoinTerms(s []string) string {
	return strings.Join(s, ".")
}

// Inits returns a list of heads of the string,
// e.g. [1,2,3] -> [[1,2,3],[1,2],[1]]
func Inits(in []string) [][]string {
	var results [][]string
	for i := 0; i < len(in); i++ {
		results = append([][]string{in[0 : i+1]}, results...)
	}
	return results
}

// Prefixes returns a set of prefixes for a string, e.g. "a.b.c.d" will yield
// []string{"a.b.c.d","a.b.c","a.b.","a"}
func Prefixes(in string) (out []string) {
	for _, termSet := range Inits(SplitTerms(in)) {
		out = append(out, JoinTerms(termSet))
	}
	return out
}

// Find returns the first element of the string slice for which f returns true
func Find(slice []string, f func(string) bool) (string, bool) {
	for _, elem := range slice {
		if f(elem) {
			return elem, true
		}
	}
	return "", false
}

// MkCallPipeline transforms a term group (b.c.d) into a pipeline (b | c | d)
func MkCallPipeline(s string) string {
	return strings.Join(SplitTerms(s), " | ")
}

// DesugarCall takes a call in the form of "a.b.c.d" and returns a desugared
// string that will work with the language extension provided by calling
// .Language()
func DesugarCall(g *graph.Graph, call string) (string, error) {
	var out bytes.Buffer
	pfx, rest, found := VertexSplit(g, call)
	if !found {
		return "", errors.New("syntax error call to non-existant dependency")
	}
	out.WriteString(fmt.Sprintf("(noderef %q)", pfx))
	if rest != "" {
		out.WriteString(fmt.Sprintf("| %s", MkCallPipeline(rest)))
	}
	return out.String(), nil
}

// VertexSplit takes a graph with a set of vertexes and a string, and returns
// the longest vertex id from the graph and the remainder of the string.  If no
// matching vertex is found 'false' is returned.
func VertexSplit(g *graph.Graph, s string) (string, string, bool) {
	prefix, found := Find(Prefixes(s), g.Contains)
	if !found {
		return "", s, false
	}
	if prefix == s {
		return prefix, "", true
	}
	return prefix, s[len(prefix)+1:], true
}

// VertexSplitTraverse will act like vertex split, looking for a prefix matching
// the current set of graph nodes, however unlike `VertexSplit`, if a node is
// not found at the current level it will look at the parent level to the
// provided starting node, unless stop(parent) returns true.
func VertexSplitTraverse(g *graph.Graph, toFind string, startingNode string, stop func(*graph.Graph, string) bool, history map[string]struct{}) (string, string, bool) {
	history[startingNode] = struct{}{}

	for _, child := range g.Children(startingNode) {
		if _, ok := history[child]; ok {
			continue
		}
		if stop(g, child) {
			continue
		}
		vertex, middle, found := VertexSplitTraverse(g, toFind, child, stop, history)
		if found {
			return vertex, middle, found
		}
	}
	if stop(g, startingNode) {
		return "", toFind, false
	}

	fqgn := graph.SiblingID(startingNode, toFind)
	vertex, middle, found := VertexSplit(g, fqgn)
	if found {
		return vertex, middle, found
	}
	parentID := graph.ParentID(startingNode)
	return VertexSplitTraverse(g, toFind, parentID, stop, history)
}

// TraverseUntilModule is a function intended to be used with
// VertexSplitTraverse and will cause vertex splitting to propogate upwards
// until it encounters a module
func TraverseUntilModule(g *graph.Graph, id string) bool {
	if graph.IsRoot(id) {
		return true
	}
	elemMeta, ok := g.Get(id)
	if !ok {
		return true
	}
	elem := elemMeta.Value()
	if _, ok := elem.(*module.Module); ok {
		return true
	}
	if _, ok := elem.(*module.Preparer); ok {
		return true
	}
	if node, ok := elem.(*parse.Node); ok {
		return node.Kind() == "module"
	}
	return false
}

// HasField returns true if the provided struct has the defined field
func HasField(obj interface{}, fieldName string) bool {
	var v reflect.Type
	switch oType := obj.(type) {
	case reflect.Type:
		v = oType
	case reflect.Value:
		v = oType.Type()
	default:
		v = reflect.TypeOf(obj)
	}
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	fieldName, err := LookupCanonicalFieldName(v, fieldName)
	if err != nil {
		return false
	}
	_, hasField := v.FieldByName(fieldName)
	return hasField
}

// ListFields returns a list of fields for the struct
func ListFields(obj interface{}) ([]string, error) {
	var results []string
	var v reflect.Type
	switch oType := obj.(type) {
	case reflect.Type:
		v = oType
	case reflect.Value:
		v = oType.Type()
	default:
		v = reflect.TypeOf(obj)
	}
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	e := reflect.Zero(v)
	if reflect.Struct != e.Kind() {
		return results, fmt.Errorf("element is %s, not a struct", e.Type())
	}
	for idx := 0; idx < e.Type().NumField(); idx++ {
		field := e.Type().Field(idx)
		results = append(results, field.Name)
	}
	return results, nil
}

// HasMethod returns true if the provided struct supports the defined method
func HasMethod(obj interface{}, methodName string) bool {
	_, found := reflect.TypeOf(obj).MethodByName(methodName)
	return found
}

// EvalMember gets a member from a stuct, dereferencing pointers as necessary
func EvalMember(name string, obj interface{}) (reflect.Value, error) {
	keys, fields := lookupMap(FieldMap(obj))
	k, ok := keys[strings.ToLower(name)]
	if !ok {
		var validValues []string
		for k := range keys {
			validValues = append(validValues, k)
		}
		return reflect.ValueOf(obj), fmt.Errorf("%T has no field %s. Must be one of: %v", obj, name, validValues)
	}
	return fields[k], nil
}

// Returns true if this is a non-nil pointer or interface
func indirectStruct(v reflect.Value) bool {
	return (v.Kind() == reflect.Ptr && !v.IsNil()) || v.Kind() == reflect.Interface
}

// FieldMap generates a map of field names to values for an interface, including
// embedded structs and interfaces.  If a field is defined in more than one
// embedded struct or interface then it is excluded (unless it is also present
// in the parent struct).  Fields in obj will take priority over those in
// embedded structs.
func FieldMap(obj interface{}) map[string]reflect.Value {
	embeddedRefs := make(map[string]int)
	unembeddedRefs := make(map[string]int)

	results := make(map[string]reflect.Value)
	val := reflect.ValueOf(obj)
	if canBeNil(val) && val.IsNil() {
		return results
	}

	for {
		if !indirectStruct(val) {
			break
		}
		val = val.Elem()
	}

	for idx := 0; idx < val.Type().NumField(); idx++ {
		sfield := val.Type().Field(idx)
		if sfield.Anonymous {
			if canBeNil(val.Field(idx)) && val.Field(idx).IsNil() {
				results[sfield.Name] = val.Field(idx)
				continue
			}
			for k, v := range FieldMap(val.Field(idx).Interface()) {
				if val, ok := embeddedRefs[k]; ok {
					embeddedRefs[k] = val + 1
				} else {
					embeddedRefs[k] = 1
				}
				if _, ok := results[k]; !ok {
					results[k] = v
				}
			}
		} else {
			unembeddedRefs[sfield.Name] = idx
		}
		results[sfield.Name] = val.Field(idx)
	}

	for ref, count := range embeddedRefs {
		if count <= 1 {
			continue
		}
		if idx, ok := unembeddedRefs[ref]; ok {
			results[ref] = val.Field(idx)
		} else {
			delete(results, ref)
		}
	}
	return results
}

func canBeNil(r reflect.Value) bool {
	switch r.Kind() {
	case reflect.Ptr, reflect.Interface:
		return true
	case reflect.Slice, reflect.Array, reflect.Map:
		return true
	case reflect.Chan, reflect.Func:
		return true
	}
	return false
}

// HasPath returns true of the set of terms can resolve to a value
func HasPath(obj interface{}, terms ...string) error {
	for _, term := range terms {
		term = strings.ToLower(term)
		lookupMap, fieldMap := lookupMap(FieldMap(obj))
		key, ok := lookupMap[term]
		if !ok {
			var validKeys []string
			for k := range lookupMap {
				validKeys = append(validKeys, k)
			}
			return fmt.Errorf("%T has no defined field named %s: should be one of: %v", obj, term, validKeys)
		}
		val := fieldMap[key]
		if val.Kind() == reflect.Ptr && val.IsNil() {
			return fmt.Errorf("field is nil")
		}
		obj = val.Interface()
	}
	return nil
}

func lookupMap(src map[string]reflect.Value) (map[string]string, map[string]reflect.Value) {
	keys := make(map[string]string)
	for k := range src {
		keys[strings.ToLower(k)] = k
	}
	return keys, src
}

// EvalTerms acts as a left fold over a list of term accessors
func EvalTerms(obj interface{}, terms ...string) (interface{}, error) {
	for _, term := range terms {
		term = strings.ToLower(term)
		lookupMap, fieldMap := lookupMap(FieldMap(obj))
		key, ok := lookupMap[term]
		if !ok {
			var validKeys []string
			for k := range lookupMap {
				validKeys = append(validKeys, k)
			}
			return nil, fmt.Errorf("%T has no defined field named %s: should be one of: %v", obj, term, validKeys)
		}
		val := fieldMap[key]
		if val.Kind() == reflect.Ptr && val.IsNil() {
			return nil, ErrUnresolvable
		}
		obj = val.Interface()
	}
	return obj, nil
}

// For a given interface, fieldMap returns a map with keys being the lowercase
// versions of the string, and values being the correct version.  It returns an
// error if the interface is not a struct, or a reflect.Type or reflect.Value of
// a struct.
func fieldMap(val interface{}) (map[string]string, error) {
	fieldMap := make(map[string]string)
	conflictMap := make(map[string]struct{})
	var t reflect.Type
	switch typed := val.(type) {
	case reflect.Type:
		t = typed
	case reflect.Value:
		t = typed.Type()
	default:
		t = reflect.TypeOf(val)
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("cannot access fields of non-struct type %T", val)
	}
	return addFieldsToMap(fieldMap, conflictMap, t)
}

func addFieldsToMap(m map[string]string, conflicts map[string]struct{}, t reflect.Type) (map[string]string, error) {
	if cached, ok := fieldMapCache.Get(t); ok {
		return cached, nil
	}
	for idx := 0; idx < t.NumField(); idx++ {
		field := t.Field(idx)
		if field.Anonymous {
			lower := strings.ToLower(field.Name)
			if _, ok := m[lower]; !ok {
				m[lower] = field.Name
			}
			var err error
			anonType := interfaceToConcreteType(field.Type)
			if anonType.Kind() == reflect.Struct {
				if m, err = addFieldsToMap(m, conflicts, anonType); err != nil {
					return nil, err
				}
			}
			continue
		}
		name := field.Name
		lower := strings.ToLower(name)
		if _, ok := m[lower]; ok {
			conflicts[lower] = struct{}{}
		} else {
			if _, ok := conflicts[lower]; !ok {
				m[lower] = name
			}
		}
	}
	fieldMapCache.Put(t, m)
	return m, nil
}

// LookupCanonicalFieldName takes a type and an arbitrarily cased field name and
// returns the field name with a case that matches the actual field.
func LookupCanonicalFieldName(t reflect.Type, term string) (string, error) {
	term = strings.ToLower(term)
	m, err := fieldMap(t)
	if err != nil {
		return "", err
	}
	correctCase, found := m[term]
	if found {
		return correctCase, nil
	}
	var fields []string
	for key := range m {
		fields = append(fields, key)
	}
	return "", fmt.Errorf("%s has no field that matches %s, should be one of %v", t, term, fields)
}

func interfaceToConcreteType(i interface{}) reflect.Type {
	var t reflect.Type
	switch typed := i.(type) {
	case reflect.Type:
		t = typed
	case reflect.Value:
		t = typed.Type()
	default:
		t = reflect.TypeOf(i)
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// mapToLower converts a string slice to all lower case
func mapToLower(strs []string) []string {
	for idx, str := range strs {
		strs[idx] = strings.ToLower(str)
	}
	return strs
}

func nilPtrError(v reflect.Value) error {
	typeStr := v.Type().String()
	return fmt.Errorf("cannot dereference nil pointer of type %s", typeStr)
}

func missingFieldError(name string, v reflect.Value) error {
	return fmt.Errorf("%s has no field named %s", v.Type().String(), name)
}

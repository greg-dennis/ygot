// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ygot

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/protobuf/proto"

	"github.com/openconfig/gnmi/errdiff"
	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/testutil"
)

const (
	// TestRoot is the path to the directory within which the test runs, appended
	// to any filename that is to be loaded.
	TestRoot string = ""
)

var (
	testBinary1 = testutil.Binary("abc")
	testBinary2 = testutil.Binary("def")
)

// errToString returns an error as a string.
func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestStructTagToLibPaths(t *testing.T) {
	tests := []struct {
		name               string
		inField            reflect.StructField
		inParent           *gnmiPath
		inPreferShadowPath bool
		want               []*gnmiPath
		wantErr            bool
	}{{
		name: "invalid input path",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo"`,
		},
		inParent: &gnmiPath{
			pathElemPath:    []*gnmipb.PathElem{},
			stringSlicePath: []string{},
		},
		wantErr: true,
	}, {
		name: "simple single tag example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo"`,
		},
		inParent: &gnmiPath{
			stringSlicePath: []string{},
		},
		want: []*gnmiPath{{
			stringSlicePath: []string{"foo"},
		}},
	}, {
		name: "multi-element single tag example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo/bar"`,
		},
		inParent: &gnmiPath{
			stringSlicePath: []string{},
		},
		want: []*gnmiPath{{
			stringSlicePath: []string{"foo", "bar"},
		}},
	}, {
		name: "multi-element single tag with shadow-path example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo/bar" shadow-path:"far/boo"`,
		},
		inParent: &gnmiPath{
			stringSlicePath: []string{},
		},
		want: []*gnmiPath{{
			stringSlicePath: []string{"foo", "bar"},
		}},
	}, {
		name: "multi-element single tag with preferred shadow-path example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo/bar" shadow-path:"far/boo"`,
		},
		inParent: &gnmiPath{
			stringSlicePath: []string{},
		},
		inPreferShadowPath: true,
		want: []*gnmiPath{{
			stringSlicePath: []string{"far", "boo"},
		}},
	}, {
		name: "empty tag example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"" rootpath:""`,
		},
		inParent: &gnmiPath{
			stringSlicePath: []string{},
		},
		want: []*gnmiPath{{
			stringSlicePath: []string{},
		}},
	}, {
		name: "multiple path",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo/bar|bar"`,
		},
		inParent: &gnmiPath{
			stringSlicePath: []string{},
		},
		want: []*gnmiPath{{
			stringSlicePath: []string{"foo", "bar"},
		}, {
			stringSlicePath: []string{"bar"},
		}},
	}, {
		name: "populated parent path",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"baz|foo/baz"`,
		},
		inParent: &gnmiPath{
			stringSlicePath: []string{"existing"},
		},
		want: []*gnmiPath{{
			stringSlicePath: []string{"existing", "baz"},
		}, {
			stringSlicePath: []string{"existing", "foo", "baz"},
		}},
	}, {
		name: "simple pathelem single tag example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo"`,
		},
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "foo"}},
		}},
	}, {
		name: "simple pathelem single tag with shadow-path preferred but not found example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo"`,
		},
		inPreferShadowPath: true,
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "foo"}},
		}},
	}, {
		name: "empty tag pathelem example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"" rootpath:""`,
		},
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{},
		}},
	}, {
		name: "multi-element single tag pathelem example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo/bar"`,
		},
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "foo"}, {Name: "bar"}},
		}},
	}, {
		name: "multi-element single tag with shadow-path pathelem example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo/bar" shadow-path:"far/boo"`,
		},
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "foo"}, {Name: "bar"}},
		}},
	}, {
		name: "multi-element single tag with preferred shadow-path pathelem example",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo/bar" shadow-path:"far/boo"`,
		},
		inPreferShadowPath: true,
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "far"}, {Name: "boo"}},
		}},
	}, {
		name: "multiple pathelem path",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"foo/bar|bar"`,
		},
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "foo"}, {Name: "bar"}},
		}, {
			pathElemPath: []*gnmipb.PathElem{{Name: "bar"}},
		}},
	}, {
		name: "populated pathelem parent path",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"baz|foo/baz"`,
		},
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{{Name: "existing"}},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "existing"}, {Name: "baz"}},
		}, {
			pathElemPath: []*gnmipb.PathElem{{Name: "existing"}, {Name: "foo"}, {Name: "baz"}},
		}},
	}, {
		name: "populated pathelem parent path with shadow-path",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"baz|foo/baz" shadow-path:"far/boo"`,
		},
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{{Name: "existing"}},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "existing"}, {Name: "baz"}},
		}, {
			pathElemPath: []*gnmipb.PathElem{{Name: "existing"}, {Name: "foo"}, {Name: "baz"}},
		}},
	}, {
		name: "populated pathelem parent path with preferred shadow-path",
		inField: reflect.StructField{
			Name: "field",
			Tag:  `path:"baz|foo/baz" shadow-path:"far/boo"`,
		},
		inPreferShadowPath: true,
		inParent: &gnmiPath{
			pathElemPath: []*gnmipb.PathElem{{Name: "existing"}},
		},
		want: []*gnmiPath{{
			pathElemPath: []*gnmipb.PathElem{{Name: "existing"}, {Name: "far"}, {Name: "boo"}},
		}},
	}}

	for _, tt := range tests {
		got, err := structTagToLibPaths(tt.inField, tt.inParent, tt.inPreferShadowPath)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: structTagToLibPaths(%v, %v, %v): did not get expected error status, got: %v, want err: %v", tt.name, tt.inField, tt.inParent, tt.inPreferShadowPath, err, tt.wantErr)
		}

		if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(gnmiPath{}), cmp.Comparer(proto.Equal)); diff != "" {
			t.Errorf("%s: structTagToLibPaths(%v, %v, %v): did not get expected set of map paths, diff(-want, +got):\n%s", tt.name, tt.inField, tt.inParent, tt.inPreferShadowPath, diff)
		}
	}
}

type enumTest int64

func (enumTest) IsYANGGoEnum() {}

const (
	EUNSET enumTest = 0
	EONE   enumTest = 1
	ETWO   enumTest = 2
)

func (enumTest) ΛMap() map[string]map[int64]EnumDefinition {
	return map[string]map[int64]EnumDefinition{
		"enumTest": {
			1: EnumDefinition{Name: "VAL_ONE", DefiningModule: "valone-mod"},
			2: EnumDefinition{Name: "VAL_TWO", DefiningModule: "valtwo-mod"},
		},
	}
}

func (e enumTest) String() string {
	return EnumLogString(e, int64(e), "enumTest")
}

type badEnumTest int64

func (badEnumTest) IsYANGGoEnum() {}

const (
	BUNSET badEnumTest = 0
	BONE   badEnumTest = 1
)

func (badEnumTest) ΛMap() map[string]map[int64]EnumDefinition {
	return nil
}

func (e badEnumTest) String() string {
	return ""
}

func TestEnumFieldToString(t *testing.T) {
	// EONE must be a valid GoEnum.
	var _ GoEnum = EONE

	tests := []struct {
		name               string
		inField            reflect.Value
		inAppendModuleName bool
		wantName           string
		wantSet            bool
		wantErr            string
	}{{
		name:     "simple enum",
		inField:  reflect.ValueOf(EONE),
		wantName: "VAL_ONE",
		wantSet:  true,
	}, {
		name:     "unset enum",
		inField:  reflect.ValueOf(EUNSET),
		wantName: "",
		wantSet:  false,
	}, {
		name:               "simple enum with append module name",
		inField:            reflect.ValueOf(ETWO),
		inAppendModuleName: true,
		wantName:           "valtwo-mod:VAL_TWO",
		wantSet:            true,
	}, {
		name:    "bad enum - no mapping",
		inField: reflect.ValueOf(BONE),
		wantErr: "cannot map enumerated value as type badEnumTest was unknown",
	}}

	for _, tt := range tests {
		gotName, gotSet, err := enumFieldToString(tt.inField, tt.inAppendModuleName)
		if err != nil && err.Error() != tt.wantErr {
			t.Errorf("%s: enumFieldToString(%v, %v): did not get expected error, got: %v, want: %v", tt.name, tt.inField, tt.inAppendModuleName, err, tt.wantErr)
		}

		if gotName != tt.wantName {
			t.Errorf("%s: enumFieldToString(%v, %v): did not get expected name, got: %v, want: %v", tt.name, tt.inField, tt.inAppendModuleName, gotName, tt.wantName)
		}

		if gotSet != tt.wantSet {
			t.Errorf("%s: enumFieldToString(%v, %v): did not get expected set status, got: %v, want: %v", tt.name, tt.inField, tt.inAppendModuleName, gotSet, tt.wantSet)
		}
	}
}

func TestEnumName(t *testing.T) {
	tests := []struct {
		name             string
		in               GoEnum
		want             string
		wantErrSubstring string
	}{{
		name: "simple enumeration",
		in:   EONE,
		want: "VAL_ONE",
	}, {
		name: "unset",
		in:   EUNSET,
		want: "",
	}, {
		name:             "bad enumeration",
		in:               BONE,
		wantErrSubstring: "cannot map enumerated value as type badEnumTest was unknown",
	}}

	for _, tt := range tests {
		got, err := EnumName(tt.in)
		if diff := errdiff.Substring(err, tt.wantErrSubstring); diff != "" {
			t.Errorf("%s: EnumName(%v): did not get expected error, %s", tt.name, tt.in, diff)
		}

		if got != tt.want {
			t.Errorf("%s: EnumName(%v): did not get expected value, got: %s, want: %s", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestEnumLogString(t *testing.T) {
	tests := []struct {
		desc           string
		inEnum         GoEnum
		inVal          int64
		inEnumTypeName string
		want           string
	}{{
		desc:           "one",
		inEnum:         EONE,
		inVal:          int64(EONE),
		inEnumTypeName: "enumTest",
		want:           "VAL_ONE",
	}, {
		desc:           "two",
		inEnum:         ETWO,
		inVal:          int64(ETWO),
		inEnumTypeName: "enumTest",
		want:           "VAL_TWO",
	}, {
		desc:           "unset",
		inEnum:         EUNSET,
		inVal:          int64(EUNSET),
		inEnumTypeName: "enumTest",
		want:           "out-of-range enumTest enum value: 0",
	}, {
		desc:           "way out of range",
		inEnum:         EONE,
		inVal:          42,
		inEnumTypeName: "enumTest",
		want:           "out-of-range enumTest enum value: 42",
	}}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := EnumLogString(tt.inEnum, tt.inVal, tt.inEnumTypeName); got != tt.want {
				t.Errorf("EnumLogString: got %s, want %s", got, tt.want)
			}
		})
	}
}

// mapStructTestOne is the base struct used for the simple-schema test.
type mapStructTestOne struct {
	Child *mapStructTestOneChild `path:"child" module:"test-one"`
}

// IsYANGGoStruct makes sure that we implement the GoStruct interface.
func (*mapStructTestOne) IsYANGGoStruct() {}

func (*mapStructTestOne) ΛValidate(...ValidationOption) error {
	return nil
}

func (*mapStructTestOne) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mapStructTestOne) ΛBelongingModule() string                { return "" }

// mapStructTestOne_Child is a child structure of the mapStructTestOne test
// case.
type mapStructTestOneChild struct {
	FieldOne   *string  `path:"config/field-one" module:"test-one/test-one"`
	FieldTwo   *uint32  `path:"config/field-two" module:"test-one/test-one"`
	FieldThree Binary   `path:"config/field-three" module:"test-one/test-one"`
	FieldFour  []Binary `path:"config/field-four" module:"test-one/test-one"`
	FieldFive  *uint64  `path:"config/field-five" module:"test-five/test-five"`
}

// IsYANGGoStruct makes sure that we implement the GoStruct interface.
func (*mapStructTestOneChild) IsYANGGoStruct() {}

func (*mapStructTestOneChild) ΛValidate(...ValidationOption) error {
	return nil
}

func (*mapStructTestOneChild) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mapStructTestOneChild) ΛBelongingModule() string                { return "test-one" }

// mapStructTestFour is the top-level container used for the
// schema-with-list test.
type mapStructTestFour struct {
	C *mapStructTestFourC `path:"c"`
}

// IsYANGGoStruct makes sure that we implement the GoStruct interface.
func (*mapStructTestFour) IsYANGGoStruct() {}

func (*mapStructTestFour) ΛValidate(...ValidationOption) error {
	return nil
}

func (*mapStructTestFour) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mapStructTestFour) ΛBelongingModule() string                { return "" }

// mapStructTestFourC is the "c" container used for the schema-with-list
// test.
type mapStructTestFourC struct {
	// ACLSet is a YANG list that is keyed with a string.
	ACLSet   map[string]*mapStructTestFourCACLSet   `path:"acl-set"`
	OtherSet map[ECTest]*mapStructTestFourCOtherSet `path:"other-set"`
}

// IsYANGGoStruct makes sure that we implement the GoStruct interface.
func (*mapStructTestFourC) IsYANGGoStruct() {}

func (*mapStructTestFourC) ΛValidate(...ValidationOption) error {
	return nil
}

func (*mapStructTestFourC) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mapStructTestFourC) ΛBelongingModule() string                { return "" }

// mapStructTestFourCACLSet is the struct which represents each entry in
// the ACLSet list in the schema-with-list test.
type mapStructTestFourCACLSet struct {
	// Name explicitly maps to two leaves, as shown with the two values
	// that are pipe separated.
	Name        *string `path:"config/name|name"`
	SecondValue *string `path:"config/second-value"`
}

// IsYANGGoStruct makes sure that we implement the GoStruct interface.
func (*mapStructTestFourCACLSet) IsYANGGoStruct() {}

func (*mapStructTestFourCACLSet) ΛValidate(...ValidationOption) error {
	return nil
}

func (*mapStructTestFourCACLSet) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mapStructTestFourCACLSet) ΛBelongingModule() string                { return "" }

// mapStructTestFourOtherSet is a map entry with a
type mapStructTestFourCOtherSet struct {
	Name ECTest `path:"config/name|name"`
}

// IsYANGGoStruct implements the GoStruct interface.
func (*mapStructTestFourCOtherSet) IsYANGGoStruct() {}

func (*mapStructTestFourCOtherSet) ΛValidate(...ValidationOption) error {
	return nil
}

func (*mapStructTestFourCOtherSet) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mapStructTestFourCOtherSet) ΛBelongingModule() string                { return "" }

// ECTest is a synthesised derived type which is used to represent
// an enumeration in the YANG schema.
type ECTest int64

// IsYANGEnumeration ensures that the ECTest derived enum type implemnts
// the GoEnum interface.
func (ECTest) IsYANGGoEnum() {}

const (
	ECTestUNSET  = 0
	ECTestVALONE = 1
	ECTestVALTWO = 2
)

// ΛMap returns the enumeration dictionary associated with the mapStructTestFiveC
// struct.
func (ECTest) ΛMap() map[string]map[int64]EnumDefinition {
	return map[string]map[int64]EnumDefinition{
		"ECTest": {
			1: EnumDefinition{Name: "VAL_ONE", DefiningModule: "valone-mod"},
			2: EnumDefinition{Name: "VAL_TWO", DefiningModule: "valtwo-mod"},
		},
	}
}

func (e ECTest) String() string {
	return EnumLogString(e, int64(e), "ECTest")
}

// mapStructInvalid is a valid GoStruct whose ΛValidate() method always returns
// an error.
type mapStructInvalid struct {
	Name *string `path:"name"`
}

// IsYANGGoStruct implements the GoStruct interface.
func (*mapStructInvalid) IsYANGGoStruct() {}

// Validate implements the GoStruct interface.
func (*mapStructInvalid) ΛValidate(...ValidationOption) error {
	return fmt.Errorf("invalid")
}

func (*mapStructInvalid) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mapStructInvalid) ΛBelongingModule() string                { return "" }

// mapStructNoPaths is a valid GoStruct who does not implement path tags.
type mapStructNoPaths struct {
	Name *string
}

// IsYANGGoStruct implements the GoStruct interface.
func (*mapStructNoPaths) IsYANGGoStruct() {}

// Validate implements the GoStruct interface.
func (*mapStructNoPaths) ΛValidate(...ValidationOption) error     { return nil }
func (*mapStructNoPaths) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mapStructNoPaths) ΛBelongingModule() string                { return "" }

// TestEmitJSON validates that the EmitJSON function outputs the expected JSON
// for a set of input structs and schema definitions.
func TestEmitJSON(t *testing.T) {
	tests := []struct {
		name         string
		inStruct     GoStruct
		inConfig     *EmitJSONConfig
		wantJSONPath string
		wantErr      string
	}{{
		name: "simple schema JSON output",
		inStruct: &mapStructTestOne{
			Child: &mapStructTestOneChild{
				FieldOne: String("abc -> def"),
				FieldTwo: Uint32(42),
			},
		},
		wantJSONPath: filepath.Join(TestRoot, "testdata/emitjson_1.json-txt"),
	}, {
		name: "simple schema JSON output with safe HTML",
		inStruct: &mapStructTestOne{
			Child: &mapStructTestOneChild{
				FieldOne: String("abc -> def"),
				FieldTwo: Uint32(42),
			},
		},
		inConfig: &EmitJSONConfig{
			EscapeHTML: true,
		},
		wantJSONPath: filepath.Join(TestRoot, "testdata/emitjson_1_html_safe.json-txt"),
	}, {
		name: "schema with a list JSON output",
		inStruct: &mapStructTestFour{
			C: &mapStructTestFourC{
				ACLSet: map[string]*mapStructTestFourCACLSet{
					"n42": {Name: String("n42"), SecondValue: String("val")},
				},
			},
		},
		wantJSONPath: filepath.Join(TestRoot, "testdata/emitjson_2.json-txt"),
	}, {
		name: "simple schema IETF JSON output",
		inStruct: &mapStructTestOne{
			Child: &mapStructTestOneChild{
				FieldOne:  String("bar"),
				FieldTwo:  Uint32(84),
				FieldFive: Uint64(42),
			},
		},
		inConfig: &EmitJSONConfig{
			Format: RFC7951,
			RFC7951Config: &RFC7951JSONConfig{
				AppendModuleName: true,
			},
			Indent: "  ",
		},
		wantJSONPath: filepath.Join(TestRoot, "testdata/emitjson1_ietf.json-txt"),
	}, {
		name: "schema with list and enum IETF JSON",
		inStruct: &mapStructTestFour{
			C: &mapStructTestFourC{
				ACLSet: map[string]*mapStructTestFourCACLSet{
					"n42": {Name: String("n42"), SecondValue: String("foo")},
				},
				OtherSet: map[ECTest]*mapStructTestFourCOtherSet{
					ECTestVALONE: {Name: ECTestVALONE},
					ECTestVALTWO: {Name: ECTestVALTWO},
				},
			},
		},
		inConfig: &EmitJSONConfig{
			Format: RFC7951,
			RFC7951Config: &RFC7951JSONConfig{
				AppendModuleName: true,
			},
			Indent: "  ",
		},
		wantJSONPath: filepath.Join(TestRoot, "testdata/emitjson2_ietf.json-txt"),
	}, {
		name:     "invalid struct contents",
		inStruct: &mapStructInvalid{Name: String("aardvark")},
		wantErr:  "validation err: invalid",
	}, {
		name:     "invalid with skip validation",
		inStruct: &mapStructInvalid{Name: String("aardwolf")},
		inConfig: &EmitJSONConfig{
			SkipValidation: true,
		},
		wantJSONPath: filepath.Join(TestRoot, "testdata", "invalid-struct.json-txt"),
	}, {
		name:     "invalid internal JSON",
		inStruct: &mapStructNoPaths{Name: String("honey badger")},
		wantErr:  "ConstructInternalJSON error: Name: field did not specify a path",
	}, {
		name:     "invalid RFC7951 JSON",
		inStruct: &mapStructNoPaths{Name: String("ladybird")},
		inConfig: &EmitJSONConfig{
			Format: RFC7951,
		},
		wantErr: "ConstructIETFJSON error: Name: field did not specify a path",
	}}

	for _, tt := range tests {
		got, err := EmitJSON(tt.inStruct, tt.inConfig)
		if errToString(err) != tt.wantErr {
			t.Errorf("%s: EmitJSON(%v, nil): did not get expected error, got: %v, want: %v", tt.name, tt.inStruct, err, tt.wantErr)
			continue
		}

		if tt.wantErr != "" {
			continue
		}

		wantJSON, ioerr := ioutil.ReadFile(tt.wantJSONPath)
		if ioerr != nil {
			t.Errorf("%s: ioutil.ReadFile(%s): could not open file: %v", tt.name, tt.wantJSONPath, ioerr)
			continue
		}

		if diff := pretty.Compare(got, string(wantJSON)); diff != "" {
			if diffl, err := testutil.GenerateUnifiedDiff(string(wantJSON), got); err == nil {
				diff = diffl
			}
			t.Errorf("%s: EmitJSON(%v, nil): got invalid JSON, diff(-want, +got):\n%s", tt.name, tt.inStruct, diff)
		}
	}
}

// emptyTreeTestOne is a test case for TestBuildEmptyTree.
type emptyTreeTestOne struct {
	ValOne   *string
	ValTwo   *string
	ValThree *int32
}

// IsYANGGoStruct ensures that emptyTreeTestOne implements the GoStruct interface
func (*emptyTreeTestOne) IsYANGGoStruct() {}

// emptyTreeTestTwo is a test case for TestBuildEmptyTree
type emptyTreeTestTwo struct {
	SliceVal     []*emptyTreeTestTwoChild
	MapVal       map[string]*emptyTreeTestTwoChild
	StructVal    *emptyTreeTestTwoChild
	StructValTwo *emptyTreeTestTwoChild
}

// IsYANGGoStruct ensures that emptyTreeTestTwo implements the GoStruct interface
func (*emptyTreeTestTwo) IsYANGGoStruct() {}

// emptyTreeTestTwoChild is used in the TestBuildEmptyTree emptyTreeTestTwo structs.
type emptyTreeTestTwoChild struct {
	Val string
}

func TestBuildEmptyTree(t *testing.T) {
	tests := []struct {
		name     string
		inStruct GoStruct
		want     GoStruct
	}{{
		name:     "struct with no children",
		inStruct: &emptyTreeTestOne{},
		want:     &emptyTreeTestOne{},
	}, {
		name:     "struct with children",
		inStruct: &emptyTreeTestTwo{},
		want: &emptyTreeTestTwo{
			SliceVal:     []*emptyTreeTestTwoChild{},
			MapVal:       map[string]*emptyTreeTestTwoChild{},
			StructVal:    &emptyTreeTestTwoChild{},
			StructValTwo: &emptyTreeTestTwoChild{},
		},
	}, {
		name: "struct with already populated child",
		inStruct: &emptyTreeTestTwo{
			StructVal: &emptyTreeTestTwoChild{
				Val: "foo",
			},
		},
		want: &emptyTreeTestTwo{
			StructVal: &emptyTreeTestTwoChild{
				Val: "foo",
			},
			StructValTwo: &emptyTreeTestTwoChild{},
		},
	}}

	for _, tt := range tests {
		BuildEmptyTree(tt.inStruct)
		if diff := pretty.Compare(tt.inStruct, tt.want); diff != "" {
			t.Errorf("%s: did not get expected output, diff(-got,+want):\n%s", tt.name, diff)
		}
	}
}

type emptyBranchTestOne struct {
	String    *string                             `path:"string"`
	Struct    *emptyBranchTestOneChild            `path:"child"`
	StructMap map[string]*emptyBranchTestOneChild `path:"maps/map"`
}

func (*emptyBranchTestOne) IsYANGGoStruct() {}

type emptyBranchTestOneChild struct {
	String     *string                       `path:"string"`
	Enumerated int64                         `path:"enum"`
	Struct     *emptyBranchTestOneGrandchild `path:"grand-child"`
}

func (*emptyBranchTestOneChild) IsYANGGoStruct() {}

type emptyBranchTestOneGrandchild struct {
	String *string                            `path:"string"`
	Slice  []string                           `path:"slice"`
	Struct *emptyBranchTestOneGreatGrandchild `path:"great-grand-child"`
}

func (*emptyBranchTestOneGrandchild) IsYANGGoStruct() {}

type emptyBranchTestOneGreatGrandchild struct {
	String *string `path:"string"`
}

func (*emptyBranchTestOneGreatGrandchild) IsYANGGoStruct() {}

func TestPruneEmptyBranches(t *testing.T) {
	tests := []struct {
		name     string
		inStruct GoStruct
		want     GoStruct
	}{{
		name:     "struct with no children",
		inStruct: &emptyBranchTestOne{},
		want:     &emptyBranchTestOne{},
	}, {
		name: "struct with empty child",
		inStruct: &emptyBranchTestOne{
			String: String("hello"),
			Struct: &emptyBranchTestOneChild{},
		},
		want: &emptyBranchTestOne{
			String: String("hello"),
		},
	}, {
		name: "struct with populated child",
		inStruct: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				String: String("foo"),
			},
		},
		want: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				String: String("foo"),
			},
		},
	}, {
		name: "struct with populated child with unpopulated grandchild",
		inStruct: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				String: String("bar"),
				Struct: &emptyBranchTestOneGrandchild{},
			},
		},
		want: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				String: String("bar"),
			},
		},
	}, {
		name: "struct with populated grandchild",
		inStruct: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				String: String("bar"),
				Struct: &emptyBranchTestOneGrandchild{
					String: String("baz"),
				},
			},
		},
		want: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				String: String("bar"),
				Struct: &emptyBranchTestOneGrandchild{
					String: String("baz"),
				},
			},
		},
	}, {
		name: "struct with unpopulated child and grandchild",
		inStruct: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				Struct: &emptyBranchTestOneGrandchild{},
			},
		},
		want: &emptyBranchTestOne{},
	}, {
		name: "struct with map with unpopulated children",
		inStruct: &emptyBranchTestOne{
			StructMap: map[string]*emptyBranchTestOneChild{
				"value": {
					String: String("value"),
					Struct: &emptyBranchTestOneGrandchild{
						Struct: &emptyBranchTestOneGreatGrandchild{},
					},
				},
			},
		},
		want: &emptyBranchTestOne{
			StructMap: map[string]*emptyBranchTestOneChild{
				"value": {
					String: String("value"),
				},
			},
		},
	}, {
		name: "struct with slice, and enumerated value",
		inStruct: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				Enumerated: 42,
				Struct: &emptyBranchTestOneGrandchild{
					Slice:  []string{"one", "two"},
					Struct: &emptyBranchTestOneGreatGrandchild{},
				},
			},
		},
		want: &emptyBranchTestOne{
			Struct: &emptyBranchTestOneChild{
				Enumerated: 42,
				Struct: &emptyBranchTestOneGrandchild{
					Slice: []string{"one", "two"},
				},
			},
		},
	}}

	for _, tt := range tests {
		PruneEmptyBranches(tt.inStruct)
		if diff := pretty.Compare(tt.inStruct, tt.want); diff != "" {
			t.Errorf("%s: PruneEmptyBranches(%#v): did not get expected output, diff(-got,+want):\n%s", tt.name, tt.inStruct, diff)
		}
	}
}

// initContainerTest is a synthesised GoStruct for use in
// testing InitContainer.
type initContainerTest struct {
	StringVal    *string
	ContainerVal *initContainerTestChild
}

// IsYANGGoStruct ensures that the GoStruct interface is implemented
// for initContainerTest.
func (*initContainerTest) IsYANGGoStruct() {}

// initContainerTestChild is a synthesised GoStruct for use
// as a child of initContainerTest, and used in testing
// InitContainer.
type initContainerTestChild struct {
	Val *string
}

// IsYANGGoStruct ensures that the GoStruct interface is implemented
// for initContainerTestChild.
func (*initContainerTestChild) IsYANGGoStruct() {}

func TestInitContainer(t *testing.T) {
	tests := []struct {
		name            string
		inStruct        GoStruct
		inContainerName string
		want            GoStruct
		wantErr         bool
	}{{
		name:            "initialise existing field",
		inStruct:        &initContainerTest{},
		inContainerName: "ContainerVal",
		want:            &initContainerTest{ContainerVal: &initContainerTestChild{}},
	}, {
		name:            "initialise non-container field",
		inStruct:        &initContainerTest{},
		inContainerName: "StringVal",
		wantErr:         true,
	}, {
		name:            "initialise non-existent field",
		inStruct:        &initContainerTest{},
		inContainerName: "Fish",
		wantErr:         true,
	}}

	for _, tt := range tests {
		if err := InitContainer(tt.inStruct, tt.inContainerName); err != nil {
			if !tt.wantErr {
				t.Errorf("%s: InitContainer(%v): got unexpected error: %v", tt.name, tt.inStruct, err)
			}
			continue
		}

		if diff := pretty.Compare(tt.inStruct, tt.want); diff != "" {
			t.Errorf("%s: InitContainer(...): did not get expected output, diff(-got,+want):\n%s", tt.name, diff)
		}
	}
}

func TestMergeJSON(t *testing.T) {
	tests := []struct {
		name    string
		inA     map[string]interface{}
		inB     map[string]interface{}
		want    map[string]interface{}
		wantErr bool
	}{{
		name: "simple maps",
		inA:  map[string]interface{}{"a": 1},
		inB:  map[string]interface{}{"b": 2},
		want: map[string]interface{}{"a": 1, "b": 2},
	}, {
		name: "non-overlapping multi-layer tree",
		inA: map[string]interface{}{
			"a": map[string]interface{}{
				"a1": 42,
			},
			"aa": map[string]interface{}{
				"aa2": 84,
			},
		},
		inB: map[string]interface{}{
			"b": map[string]interface{}{
				"b1": 42,
			},
			"bb": map[string]interface{}{
				"bb2": 84,
			},
		},
		want: map[string]interface{}{
			"a": map[string]interface{}{
				"a1": 42,
			},
			"aa": map[string]interface{}{
				"aa2": 84,
			},
			"b": map[string]interface{}{
				"b1": 42,
			},
			"bb": map[string]interface{}{
				"bb2": 84,
			},
		},
	}, {
		name: "overlapping trees",
		inA: map[string]interface{}{
			"a": map[string]interface{}{
				"b": "c",
			},
		},
		inB: map[string]interface{}{
			"a": map[string]interface{}{
				"c": "d",
			},
		},
		want: map[string]interface{}{
			"a": map[string]interface{}{
				"b": "c",
				"c": "d",
			},
		},
	}, {
		name: "slice within json",
		inA: map[string]interface{}{
			"a": []interface{}{
				map[string]interface{}{"a": "a"},
			},
		},
		inB: map[string]interface{}{
			"a": []interface{}{
				map[string]interface{}{"b": "b"},
			},
		},
		want: map[string]interface{}{
			"a": []interface{}{
				map[string]interface{}{"a": "a"},
				map[string]interface{}{"b": "b"},
			},
		},
	}, {
		name: "slice value",
		inA: map[string]interface{}{
			"a": []interface{}{"a"},
		},
		inB: map[string]interface{}{
			"a": []interface{}{"b"},
		},
		want: map[string]interface{}{
			"a": []interface{}{"a", "b"},
		},
	}, {
		name: "scalar value",
		inA: map[string]interface{}{
			"a": "a",
		},
		inB: map[string]interface{}{
			"a": "b",
		},
		wantErr: true,
	}, {
		name: "different depth trees",
		inA: map[string]interface{}{
			"a": map[string]interface{}{
				"a1": map[string]interface{}{
					"a2": map[string]interface{}{
						"a3": 42,
					},
				},
			},
			"b": map[string]interface{}{
				"a1": map[string]interface{}{
					"a2": 42,
				},
			},
		},
		inB: map[string]interface{}{
			"a": map[string]interface{}{
				"b1": true,
			},
			"b": map[string]interface{}{
				"b2": 84,
				"b3": map[string]interface{}{
					"b4": map[string]interface{}{
						"b5": true,
					},
				},
			},
		},
		want: map[string]interface{}{
			"a": map[string]interface{}{
				"a1": map[string]interface{}{
					"a2": map[string]interface{}{
						"a3": 42,
					},
				},
				"b1": true,
			},
			"b": map[string]interface{}{
				"a1": map[string]interface{}{
					"a2": 42,
				},
				"b2": 84,
				"b3": map[string]interface{}{
					"b4": map[string]interface{}{
						"b5": true,
					},
				},
			},
		},
	}}

	for _, tt := range tests {
		got, err := MergeJSON(tt.inA, tt.inB)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: MergeJSON(%v, %v): did not get expected error, got: %v, want: %v", tt.name, tt.inA, tt.inB, err, tt.wantErr)
			continue
		}

		if diff := pretty.Compare(got, tt.want); diff != "" {
			t.Errorf("%s: MergeJSON(%v, %v): did not get expected merged JSON, diff(-got,+want):\n%s", tt.name, tt.inA, tt.inB, diff)
		}
	}
}

type mergeTest struct {
	FieldOne    *string                        `path:"field-one" module:"mod"`
	FieldTwo    *uint8                         `path:"field-two" module:"mod"`
	LeafList    []string                       `path:"leaf-list" module:"leaflist"`
	UnkeyedList []*mergeTestListChild          `path:"unkeyed-list" module:"bar"`
	List        map[string]*mergeTestListChild `path:"list" module:"bar"`
}

func (*mergeTest) IsYANGGoStruct()                         {}
func (*mergeTest) ΛValidate(...ValidationOption) error     { return nil }
func (*mergeTest) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mergeTest) ΛBelongingModule() string                { return "" }

type mergeTestListChild struct {
	Val *string `path:"val" module:"mod"`
}

func (*mergeTestListChild) IsYANGGoStruct()                         {}
func (*mergeTestListChild) ΛValidate(...ValidationOption) error     { return nil }
func (*mergeTestListChild) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*mergeTestListChild) ΛBelongingModule() string                { return "bar" }

func TestMergeStructJSON(t *testing.T) {
	tests := []struct {
		name     string
		inStruct GoStruct
		inJSON   map[string]interface{}
		inOpts   *EmitJSONConfig
		wantJSON map[string]interface{}
		wantErr  bool
	}{{
		name:     "single field merge test, internal format",
		inStruct: &mergeTest{FieldOne: String("hello")},
		inJSON: map[string]interface{}{
			"field-two": "world",
		},
		wantJSON: map[string]interface{}{
			"field-one": "hello",
			"field-two": "world",
		},
	}, {
		name:     "single field merge test, RFC7951 format",
		inStruct: &mergeTest{FieldOne: String("hello")},
		inJSON: map[string]interface{}{
			"mod:field-two": "world",
		},
		inOpts: &EmitJSONConfig{
			Format: RFC7951,
			RFC7951Config: &RFC7951JSONConfig{
				AppendModuleName: true,
			},
		},
		wantJSON: map[string]interface{}{
			"mod:field-one": "hello",
			"mod:field-two": "world",
		},
	}, {
		name: "leaf-list field, present in only one message, internal JSON",
		inStruct: &mergeTest{
			FieldOne: String("hello"),
			LeafList: []string{"me", "you're", "looking", "for"},
		},
		inJSON: map[string]interface{}{
			"leaf-list": []interface{}{"is", "it"},
		},
		wantJSON: map[string]interface{}{
			"field-one": "hello",
			"leaf-list": []interface{}{"is", "it", "me", "you're", "looking", "for"},
		},
	}, {
		name: "unkeyed list merge",
		inStruct: &mergeTest{
			UnkeyedList: []*mergeTestListChild{{String("in")}, {String("the")}, {String("jar")}},
		},
		inJSON: map[string]interface{}{
			"unkeyed-list": []interface{}{
				map[string]interface{}{"val": "whisky"},
			},
		},
		inOpts: &EmitJSONConfig{
			Format: RFC7951,
		},
		wantJSON: map[string]interface{}{
			"unkeyed-list": []interface{}{
				map[string]interface{}{"val": "whisky"},
				map[string]interface{}{"val": "in"},
				map[string]interface{}{"val": "the"},
				map[string]interface{}{"val": "jar"},
			},
		},
	}, {
		name: "keyed list, RFC7951 JSON",
		inStruct: &mergeTest{
			List: map[string]*mergeTestListChild{
				"anjou":  {String("anjou")},
				"chinon": {String("chinon")},
			},
		},
		inJSON: map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"val": "sancerre"},
			},
		},
		inOpts: &EmitJSONConfig{
			Format: RFC7951,
		},
		wantJSON: map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"val": "sancerre"},
				map[string]interface{}{"val": "anjou"},
				map[string]interface{}{"val": "chinon"},
			},
		},
	}, {
		name: "keyed list, internal JSON",
		inStruct: &mergeTest{
			List: map[string]*mergeTestListChild{
				"bandol": {String("bandol")},
			},
		},
		inJSON: map[string]interface{}{
			"list": map[string]interface{}{
				"bellet": map[string]interface{}{
					"val": "bellet",
				},
			},
		},
		wantJSON: map[string]interface{}{
			"list": map[string]interface{}{
				"bellet": map[string]interface{}{"val": "bellet"},
				"bandol": map[string]interface{}{"val": "bandol"},
			},
		},
	}, {
		name:     "overlapping trees",
		inStruct: &mergeTest{FieldOne: String("foo")},
		inJSON:   map[string]interface{}{"field-one": "bar"},
		wantErr:  true,
	}}

	for _, tt := range tests {
		got, err := MergeStructJSON(tt.inStruct, tt.inJSON, tt.inOpts)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: MergeStructJSON(%v, %v, %v): did not get expected error status, got: %v, want: %v", tt.name, tt.inStruct, tt.inJSON, tt.inOpts, err, tt.wantErr)
		}

		if diff := pretty.Compare(got, tt.wantJSON); diff != "" {
			t.Errorf("%s: MergeStrucTJSON(%v, %v, %v): did not get expected error status, diff(-got,+want):\n%s", tt.name, tt.inStruct, tt.inJSON, tt.inOpts, diff)
		}
	}
}

// Types for testing copyStruct.
type enumType int64

const (
	EnumTypeValue    enumType = 1
	EnumTypeValueTwo enumType = 2
)

func (enumType) IsYANGGoEnum() {}

func (e enumType) String() string {
	return EnumLogString(e, int64(e), "enumType")
}

func (enumType) ΛMap() map[string]map[int64]EnumDefinition {
	return map[string]map[int64]EnumDefinition{
		"enumType": {
			1: EnumDefinition{Name: "Value", DefiningModule: "valone-mod"},
			2: EnumDefinition{Name: "Value_Two", DefiningModule: "valtwo-mod"},
		},
	}
}

type copyUnion interface {
	IsUnion()
}

type copyUnionS struct {
	S string
}

func (*copyUnionS) IsUnion() {}

type copyUnionI struct {
	I int64
}

func (*copyUnionI) IsUnion() {}

func (enumType) IsUnion() {}

type copyMapKey struct {
	A string
}

type copyTest struct {
	StringField   *string
	Uint32Field   *uint32
	Uint16Field   *uint16
	Float64Field  *float64
	StructPointer *copyTest
	EnumValue     enumType
	UnionField    copyUnion
	StringSlice   []string
	StringMap     map[string]*copyTest
	StructMap     map[copyMapKey]*copyTest
	StructSlice   []*copyTest
}

func (*copyTest) IsYANGGoStruct()                         {}
func (*copyTest) ΛValidate(...ValidationOption) error     { return nil }
func (*copyTest) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*copyTest) ΛBelongingModule() string                { return "" }

type errorCopyTest struct {
	I interface{}
	S *string
	M map[string]errorCopyTest
	N map[string]*errorCopyTest
	E *errorCopyTest
	L []*errorCopyTest
}

func (*errorCopyTest) IsYANGGoStruct()                         {}
func (*errorCopyTest) ΛValidate(...ValidationOption) error     { return nil }
func (*errorCopyTest) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*errorCopyTest) ΛBelongingModule() string                { return "" }

func TestCopyStructError(t *testing.T) {
	// Checks specifically for bad reflect.Values being provided.
	tests := []struct {
		name string
		inA  reflect.Value
		inB  reflect.Value
	}{{
		name: "non-struct pointer",
		inA:  reflect.ValueOf(String("little-creatures-pale-ale")),
		inB:  reflect.ValueOf(String("4-pines-brewing-kolsch")),
	}, {
		name: "non-pointer",
		inA:  reflect.ValueOf("4-pines-indian-summer-ale"),
		inB:  reflect.ValueOf("james-squire-150-lashes"),
	}}

	for _, tt := range tests {
		if err := copyStruct(tt.inA, tt.inB); err == nil {
			t.Errorf("%s: copyStruct(%v, %v): did not get nil error, got: %v, want: nil", tt.name, tt.inA, tt.inB, err)
		}
	}
}

func TestCopyStruct(t *testing.T) {
	tests := []struct {
		name    string
		inSrc   GoStruct
		inDst   GoStruct
		inOpts  []MergeOpt
		wantDst GoStruct
		wantErr bool
	}{{
		name:    "simple string pointer",
		inSrc:   &copyTest{StringField: String("anchor-steam")},
		inDst:   &copyTest{},
		wantDst: &copyTest{StringField: String("anchor-steam")},
	}, {
		name:    "error simple string pointer different value",
		inSrc:   &copyTest{StringField: String("anchor-steam")},
		inDst:   &copyTest{StringField: String("bira")},
		wantErr: true,
	}, {
		name:  "overwrite simple string pointer different value",
		inSrc: &copyTest{StringField: String("bira")},
		inDst: &copyTest{StringField: String("anchor-steam")},
		inOpts: []MergeOpt{
			&MergeOverwriteExistingFields{},
		},
		wantDst: &copyTest{StringField: String("bira")},
	}, {
		name: "uint and string pointer",
		inSrc: &copyTest{
			StringField: String("fourpure-juicebox"),
			Uint32Field: Uint32(42),
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			StringField: String("fourpure-juicebox"),
			Uint32Field: Uint32(42),
		},
	}, {
		name: "struct pointer with single field",
		inSrc: &copyTest{
			StringField: String("lagunitas-aunt-sally"),
			StructPointer: &copyTest{
				StringField: String("deschutes-pinedrops"),
			},
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			StringField: String("lagunitas-aunt-sally"),
			StructPointer: &copyTest{
				StringField: String("deschutes-pinedrops"),
			},
		},
	}, {
		name: "struct pointer with multiple fields",
		inSrc: &copyTest{
			StringField: String("allagash-brett"),
			Uint32Field: Uint32(84),
			StructPointer: &copyTest{
				StringField: String("brooklyn-summer-ale"),
				Uint32Field: Uint32(128),
			},
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			StringField: String("allagash-brett"),
			Uint32Field: Uint32(84),
			StructPointer: &copyTest{
				StringField: String("brooklyn-summer-ale"),
				Uint32Field: Uint32(128),
			},
		},
	}, {
		name: "enum value",
		inSrc: &copyTest{
			EnumValue: EnumTypeValue,
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			EnumValue: EnumTypeValue,
		},
	}, {
		name: "union field (wrapper union)",
		inSrc: &copyTest{
			UnionField: &copyUnionS{"new-belgium-fat-tire"},
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			UnionField: &copyUnionS{"new-belgium-fat-tire"},
		},
	}, {
		name: "union field: string",
		inSrc: &copyTest{
			UnionField: testutil.UnionString("new-belgium-fat-tire"),
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			UnionField: testutil.UnionString("new-belgium-fat-tire"),
		},
	}, {
		name: "union field: int64",
		inSrc: &copyTest{
			UnionField: testutil.UnionInt64(42),
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			UnionField: testutil.UnionInt64(42),
		},
	}, {
		name:    "union field: empty",
		inSrc:   &copyTest{},
		inDst:   &copyTest{},
		wantDst: &copyTest{},
	}, {
		name: "union field: enum",
		inSrc: &copyTest{
			UnionField: EnumTypeValue,
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			UnionField: EnumTypeValue,
		},
	}, {
		name: "union field: binary",
		inSrc: &copyTest{
			UnionField: testBinary1,
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			UnionField: testBinary1,
		},
	}, {
		name: "string slice",
		inSrc: &copyTest{
			StringSlice: []string{"sierra-nevada-pale-ale", "stone-ipa"},
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			StringSlice: []string{"sierra-nevada-pale-ale", "stone-ipa"},
		},
	}, {
		name: "unimplemented string slice with existing members",
		inSrc: &copyTest{
			StringSlice: []string{"stone-and-wood-pacific", "pirate-life-brewing-iipa"},
		},
		inDst: &copyTest{
			StringSlice: []string{"feral-brewing-co-hop-hog", "balter-brewing-xpa"},
		},
		wantDst: &copyTest{
			StringSlice: []string{"feral-brewing-co-hop-hog", "balter-brewing-xpa", "stone-and-wood-pacific", "pirate-life-brewing-iipa"},
		},
	}, {
		name: "string map",
		inSrc: &copyTest{
			StringMap: map[string]*copyTest{
				"ballast-point": {StringField: String("sculpin")},
				"upslope":       {StringSlice: []string{"amber-ale", "brown"}},
			},
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			StringMap: map[string]*copyTest{
				"ballast-point": {StringField: String("sculpin")},
				"upslope":       {StringSlice: []string{"amber-ale", "brown"}},
			},
		},
	}, {
		name: "string map with existing members",
		inSrc: &copyTest{
			StringMap: map[string]*copyTest{
				"bentspoke-brewing": {StringField: String("crankshaft")},
			},
		},
		inDst: &copyTest{
			StringMap: map[string]*copyTest{
				"modus-operandi-brewing-co": {StringField: String("former-tenant")},
			},
		},
		wantDst: &copyTest{
			StringMap: map[string]*copyTest{
				"bentspoke-brewing":         {StringField: String("crankshaft")},
				"modus-operandi-brewing-co": {StringField: String("former-tenant")},
			},
		},
	}, {
		name: "overwrite, string map with overlapping members",
		inSrc: &copyTest{
			StringMap: map[string]*copyTest{
				"wild-beer-co": {StringField: String("wild-goose-chase")},
			},
		},
		inDst: &copyTest{
			StringMap: map[string]*copyTest{
				"wild-beer-co": {StringField: String("wildebeest")},
			},
		},
		inOpts: []MergeOpt{
			&MergeOverwriteExistingFields{},
		},
		wantDst: &copyTest{
			StringMap: map[string]*copyTest{
				"wild-beer-co": {StringField: String("wild-goose-chase")},
			},
		},
	}, {
		name: "error, string map with overlapping members",
		inSrc: &copyTest{
			StringMap: map[string]*copyTest{
				"wild-beer-co": {StringField: String("wild-goose-chase")},
			},
		},
		inDst: &copyTest{
			StringMap: map[string]*copyTest{
				"wild-beer-co": {StringField: String("wildebeest")},
			},
		},
		wantErr: true,
	}, {
		name: "struct map",
		inSrc: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"saint-arnold"}: {StringField: String("fancy-lawnmower")},
				{"green-flash"}:  {StringField: String("hop-head-red")},
			},
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"saint-arnold"}: {StringField: String("fancy-lawnmower")},
				{"green-flash"}:  {StringField: String("hop-head-red")},
			},
		},
	}, {
		name: "struct map with non-overlapping contents",
		inSrc: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"brewdog"}: {StringField: String("kingpin")},
			},
		},
		inDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"cheshire-brewhouse"}: {StringField: String("dane'ish")},
			},
		},
		wantDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"brewdog"}:            {StringField: String("kingpin")},
				{"cheshire-brewhouse"}: {StringField: String("dane'ish")},
			},
		},
	}, {
		name: "struct map with overlapping contents",
		inSrc: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"fourpure"}: {StringField: String("session-ipa")},
			},
		},
		inDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"fourpure"}: {
					Uint32Field:  Uint32(42),
					Uint16Field:  Uint16(16),
					Float64Field: Float64(42.42),
				},
			},
		},
		wantDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"fourpure"}: {
					StringField:  String("session-ipa"),
					Uint32Field:  Uint32(42),
					Uint16Field:  Uint16(16),
					Float64Field: Float64(42.42),
				},
			},
		},
	}, {
		name: "struct map with overlapping fields within the same key",
		inSrc: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"new-belgium"}: {StringField: String("mysterious-ranger")},
			},
		},
		inDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"new-belgium"}: {StringField: String("fat-tire")},
			},
		},
		wantErr: true,
	}, {
		name: "struct map with overlapping fields within the same key",
		inSrc: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"new-belgium"}: {StringField: String("mysterious-ranger")},
			},
		},
		inDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"new-belgium"}: {StringField: String("fat-tire")},
			},
		},
		inOpts: []MergeOpt{
			&MergeOverwriteExistingFields{},
		},
		wantDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{
				{"new-belgium"}: {StringField: String("mysterious-ranger")},
			},
		},
	}, {
		name: "struct slice",
		inSrc: &copyTest{
			StructSlice: []*copyTest{{
				StringField: String("russian-river-pliny-the-elder"),
			}, {
				StringField: String("lagunitas-brown-shugga"),
			}},
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			StructSlice: []*copyTest{{
				StringField: String("russian-river-pliny-the-elder"),
			}, {
				StringField: String("lagunitas-brown-shugga"),
			}},
		},
	}, {
		name: "struct slice with overlapping contents",
		inSrc: &copyTest{
			StructSlice: []*copyTest{{
				StringField: String("pirate-life-brewing-ipa"),
			}},
		},
		inDst: &copyTest{
			StructSlice: []*copyTest{{
				StringField: String("gage-roads-little-dove"),
			}},
		},
		wantDst: &copyTest{
			StructSlice: []*copyTest{{
				StringField: String("gage-roads-little-dove"),
			}, {
				StringField: String("pirate-life-brewing-ipa"),
			}},
		},
	}, {
		name:    "error, integer in interface",
		inSrc:   &errorCopyTest{I: 42},
		inDst:   &errorCopyTest{},
		wantErr: true,
	}, {
		name:    "error, integer pointer in interface",
		inSrc:   &errorCopyTest{I: Uint32(42)},
		inDst:   &errorCopyTest{},
		wantErr: true,
	}, {
		name:    "error, invalid interface in struct within interface",
		inSrc:   &errorCopyTest{I: &errorCopyTest{I: "founders-kbs"}},
		inDst:   &errorCopyTest{},
		wantErr: true,
	}, {
		name: "error, invalid struct in map",
		inSrc: &errorCopyTest{M: map[string]errorCopyTest{
			"beaver-town-gamma-ray": {S: String("beaver-town-black-betty-ipa")},
		}},
		inDst:   &errorCopyTest{},
		wantErr: true,
	}, {
		name: "error, invalid field in struct in map",
		inSrc: &errorCopyTest{N: map[string]*errorCopyTest{
			"brewdog-punk-ipa": {I: "harbour-amber-ale"},
		}},
		inDst:   &errorCopyTest{},
		wantErr: true,
	}, {
		name:    "error, invalid field in struct in struct ptr",
		inSrc:   &errorCopyTest{E: &errorCopyTest{I: "meantime-wheat"}},
		inDst:   &errorCopyTest{},
		wantErr: true,
	}, {
		name:    "error, invalid struct in struct ptr slice",
		inSrc:   &errorCopyTest{L: []*errorCopyTest{{I: "wild-beer-co-somerset-wild"}}},
		inDst:   &errorCopyTest{},
		wantErr: true,
	}, {
		name:    "error, mismatched types",
		inSrc:   &copyTest{StringField: String("camden-hells")},
		inDst:   &errorCopyTest{S: String("kernel-table-beer")},
		wantErr: true,
	}, {
		name:    "error, slice fields not unique",
		inSrc:   &copyTest{StringSlice: []string{"mikkeler-draft-bear"}},
		inDst:   &copyTest{StringSlice: []string{"mikkeler-draft-bear"}},
		wantDst: &copyTest{StringSlice: []string{"mikkeler-draft-bear"}},
	}, {
		name:  "overwrite, slice fields not unique",
		inSrc: &copyTest{StringSlice: []string{"mikkeler-draft-bear"}},
		inDst: &copyTest{StringSlice: []string{"kingfisher"}},
		inOpts: []MergeOpt{
			&MergeOverwriteExistingFields{},
		},
		wantDst: &copyTest{StringSlice: []string{"kingfisher", "mikkeler-draft-bear"}},
	}, {
		name: "dst struct pointer with no populated field",
		inSrc: &copyTest{
			StringField: String("lagunitas-aunt-sally"),
			StructPointer: &copyTest{
				StructPointer: &copyTest{},
			},
		},
		inDst: &copyTest{},
		wantDst: &copyTest{
			StringField: String("lagunitas-aunt-sally"),
			StructPointer: &copyTest{
				StructPointer: &copyTest{},
			},
		},
	}, {
		name:  "src struct pointer with no populated field",
		inSrc: &copyTest{},
		inDst: &copyTest{
			StringField: String("lagunitas-aunt-sally"),
			StructPointer: &copyTest{
				StructPointer: &copyTest{},
			},
		},
		wantDst: &copyTest{
			StringField: String("lagunitas-aunt-sally"),
			StructPointer: &copyTest{
				StructPointer: &copyTest{},
			},
		},
	}, {
		name: "dst single-key map with no elements",
		inSrc: &copyTest{
			StringMap: map[string]*copyTest{},
		},
		inDst: &copyTest{},
		inOpts: []MergeOpt{
			&MergeEmptyMaps{},
		},
		wantDst: &copyTest{
			StringMap: map[string]*copyTest{},
		},
	}, {
		name:  "dst single-key map with no elements",
		inSrc: &copyTest{},
		inDst: &copyTest{
			StringMap: map[string]*copyTest{},
		},
		inOpts: []MergeOpt{
			&MergeEmptyMaps{},
		},
		wantDst: &copyTest{
			StringMap: map[string]*copyTest{},
		},
	}, {
		name: "dst struct map with no elements",
		inSrc: &copyTest{
			StructMap: map[copyMapKey]*copyTest{},
		},
		inDst: &copyTest{},
		inOpts: []MergeOpt{
			&MergeEmptyMaps{},
		},
		wantDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{},
		},
	}, {
		name:  "src struct map with no elements",
		inSrc: &copyTest{},
		inDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{},
		},
		inOpts: []MergeOpt{
			&MergeEmptyMaps{},
		},
		wantDst: &copyTest{
			StructMap: map[copyMapKey]*copyTest{},
		},
	}}

	for _, tt := range tests {
		dst, src := reflect.ValueOf(tt.inDst).Elem(), reflect.ValueOf(tt.inSrc).Elem()
		var wantDst reflect.Value
		if tt.wantDst != nil {
			wantDst = reflect.ValueOf(tt.wantDst).Elem()
		}

		err := copyStruct(dst, src, tt.inOpts...)
		if (err != nil) != tt.wantErr {
			t.Fatalf("%s: copyStruct(%v, %v): did not get expected error, got: %v, wantErr: %v", tt.name, tt.inSrc, tt.inDst, err, tt.wantErr)
		}

		if err != nil {
			continue
		}

		if diff := cmp.Diff(dst.Interface(), wantDst.Interface()); diff != "" {
			t.Errorf("%s: copyStruct(%v, %v): did not get expected copied struct, diff(-got,+want):\n%s", tt.name, tt.inSrc, tt.inDst, diff)
		}
	}
}

type validatedMergeTest struct {
	String      *string
	StringTwo   *string
	Uint32Field *uint32
	EnumValue   enumType
	UnionField  copyUnion
}

func (*validatedMergeTest) ΛValidate(...ValidationOption) error     { return nil }
func (*validatedMergeTest) IsYANGGoStruct()                         {}
func (*validatedMergeTest) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*validatedMergeTest) ΛBelongingModule() string                { return "" }

type validatedMergeTestTwo struct {
	String *string
	I      interface{}
}

func (*validatedMergeTestTwo) ΛValidate(...ValidationOption) error     { return nil }
func (*validatedMergeTestTwo) IsYANGGoStruct()                         {}
func (*validatedMergeTestTwo) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*validatedMergeTestTwo) ΛBelongingModule() string                { return "" }

type validatedMergeTestWithSlice struct {
	SliceField []*validatedMergeTestSliceField
}

func (*validatedMergeTestWithSlice) ΛValidate(...ValidationOption) error     { return nil }
func (*validatedMergeTestWithSlice) IsYANGGoStruct()                         {}
func (*validatedMergeTestWithSlice) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*validatedMergeTestWithSlice) ΛBelongingModule() string                { return "" }

type validatedMergeTestSliceField struct {
	String *string
}

type validatedMergeTestWithAnnotationSlice struct {
	SliceField []Annotation `ygotAnnotation:"true"`
}

func (*validatedMergeTestWithAnnotationSlice) ΛValidate(...ValidationOption) error     { return nil }
func (*validatedMergeTestWithAnnotationSlice) IsYANGGoStruct()                         {}
func (*validatedMergeTestWithAnnotationSlice) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*validatedMergeTestWithAnnotationSlice) ΛBelongingModule() string                { return "" }

// ExampleAnnotation is used to test MergeStructs with Annotation slices.
type ExampleAnnotation struct {
	ConfigSource string `json:"cfg-source"`
}

// MarshalJSON marshals the ExampleAnnotation receiver to JSON.
func (e *ExampleAnnotation) MarshalJSON() ([]byte, error) {
	return json.Marshal(*e)
}

// UnmarshalJSON ensures that ExampleAnnotation implements the ygot.Annotation
// interface. It is stubbed out and unimplemented.
func (e *ExampleAnnotation) UnmarshalJSON([]byte) error {
	return fmt.Errorf("unimplemented")
}

// mergeStructTests are shared test cases for both MergeStructs and
// MergeStructInto. Used to capture the common cases between the two functions.
var mergeStructTests = []struct {
	name    string
	inA     GoStruct
	inB     GoStruct
	inOpts  []MergeOpt
	want    GoStruct
	wantErr string
}{{
	name: "simple struct merge, a empty",
	inA:  &validatedMergeTest{},
	inB:  &validatedMergeTest{String: String("odell-90-shilling")},
	want: &validatedMergeTest{String: String("odell-90-shilling")},
}, {
	name: "simple struct merge, a populated",
	inA:  &validatedMergeTest{String: String("left-hand-milk-stout-nitro"), Uint32Field: Uint32(42)},
	inB:  &validatedMergeTest{StringTwo: String("new-belgium-lips-of-faith-la-folie")},
	want: &validatedMergeTest{
		String:      String("left-hand-milk-stout-nitro"),
		StringTwo:   String("new-belgium-lips-of-faith-la-folie"),
		Uint32Field: Uint32(42),
	},
}, {
	name: "enum merge: set in a, and not b",
	inA: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
	inB: &validatedMergeTest{},
	want: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
}, {
	name: "enum merge: set in b and not a",
	inA:  &validatedMergeTest{},
	inB: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
	want: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
}, {
	name: "enum merge: set to same value in both",
	inA: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
	inB: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
	want: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
}, {
	name: "enum merge: set to different values in both",
	inA: &validatedMergeTest{
		EnumValue: EnumTypeValueTwo,
	},
	inB: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
	wantErr: "destination and source values were set when merging enum field",
}, {
	name: "overwrite enum merge: set to different values in both",
	inA: &validatedMergeTest{
		EnumValue: EnumTypeValueTwo,
	},
	inB: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
	inOpts: []MergeOpt{
		&MergeOverwriteExistingFields{},
	},
	want: &validatedMergeTest{
		EnumValue: EnumTypeValue,
	},
}, {
	name:    "error, differing types",
	inA:     &validatedMergeTest{String: String("great-divide-yeti")},
	inB:     &validatedMergeTestTwo{String: String("north-coast-old-rasputin")},
	wantErr: "cannot merge structs that are not of matching types, *ygot.validatedMergeTest != *ygot.validatedMergeTestTwo",
}, {
	name:    "error, bad data in B",
	inA:     &validatedMergeTestTwo{String: String("weird-beard-sorachi-faceplant")},
	inB:     &validatedMergeTestTwo{I: "fourpure-southern-latitude"},
	wantErr: "invalid interface type received: string",
}, {
	name:    "error, field set in both structs",
	inA:     &validatedMergeTest{String: String("karbach-hopadillo")},
	inB:     &validatedMergeTest{String: String("blackwater-draw-brewing-co-border-town")},
	wantErr: "destination value was set, but was not equal to source value when merging ptr field",
}, {
	name: "overwrite, field set in both structs",
	inA:  &validatedMergeTest{String: String("karbach-hopadillo")},
	inB:  &validatedMergeTest{String: String("blackwater-draw-brewing-co-border-town")},
	inOpts: []MergeOpt{
		&MergeOverwriteExistingFields{},
	},
	want: &validatedMergeTest{
		String: String("blackwater-draw-brewing-co-border-town"),
	},
}, {
	name: "allow leaf overwrite if equal",
	inA:  &validatedMergeTest{String: String("new-belgium-sour-saison")},
	inB:  &validatedMergeTest{String: String("new-belgium-sour-saison")},
	want: &validatedMergeTest{String: String("new-belgium-sour-saison")},
}, {
	name:    "error - merge leaf overwrite but not equal",
	inA:     &validatedMergeTest{String: String("schneider-weisse-hopfenweisse")},
	inB:     &validatedMergeTest{String: String("deschutes-jubelale")},
	wantErr: "destination value was set, but was not equal to source value when merging ptr field",
}, {
	name: "merge fields with slice of structs",
	inA: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}},
	},
	inB: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("citrus-dream")}},
	},
	want: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}, {String("citrus-dream")}},
	},
}, {
	name: "merge fields with duplicate slices of annotations",
	inA: &validatedMergeTestWithAnnotationSlice{
		SliceField: []Annotation{&ExampleAnnotation{ConfigSource: "devicedemo"}},
	},
	inB: &validatedMergeTestWithAnnotationSlice{
		SliceField: []Annotation{&ExampleAnnotation{ConfigSource: "devicedemo"}},
	},
	want: &validatedMergeTestWithAnnotationSlice{
		SliceField: []Annotation{
			&ExampleAnnotation{ConfigSource: "devicedemo"},
			&ExampleAnnotation{ConfigSource: "devicedemo"},
		},
	},
}, {
	name: "error - merge fields with slice with duplicate strings",
	inA: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}},
	},
	inB: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}, {String("citrus-dream")}},
	},
	wantErr: "source and destination lists must be unique",
}, {
	name: "error - merge fields with slice with duplicate strings, with dst and src reversed",
	inA: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}, {String("citrus-dream")}},
	},
	inB: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}},
	},
	wantErr: "source and destination lists must be unique",
}, {
	name: "merge fields with identical slices",
	inA: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}},
	},
	inB: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}},
	},
	want: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}},
	},
}, {
	name: "merge fields with identical slices with length 2",
	inA: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}, {String("citrus-dream")}},
	},
	inB: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}, {String("citrus-dream")}},
	},
	want: &validatedMergeTestWithSlice{
		SliceField: []*validatedMergeTestSliceField{{String("chinook-single-hop")}, {String("citrus-dream")}},
	},
}, {
	name: "merge union: string values not equal",
	inA: &validatedMergeTest{
		UnionField: testutil.UnionString("glutenberg-ipa"),
	},
	inB: &validatedMergeTest{
		UnionField: testutil.UnionString("mikkeler-pale-peter-and-mary"),
	},
	wantErr: "interface field was set in both src and dst and was not equal",
}, {
	name: "overwrite merge union: string values not equal",
	inA: &validatedMergeTest{
		UnionField: testutil.UnionString("glutenberg-ipa"),
	},
	inB: &validatedMergeTest{
		UnionField: testutil.UnionString("mikkeler-pale-peter-and-mary"),
	},
	inOpts: []MergeOpt{
		&MergeOverwriteExistingFields{},
	},
	want: &validatedMergeTest{
		UnionField: testutil.UnionString("mikkeler-pale-peter-and-mary"),
	},
}, {
	name: "merge union: string values equal",
	inA: &validatedMergeTest{
		UnionField: testutil.UnionString("ipswich-ale-celia-saison"),
	},
	inB: &validatedMergeTest{
		UnionField: testutil.UnionString("ipswich-ale-celia-saison"),
	},
	want: &validatedMergeTest{
		UnionField: testutil.UnionString("ipswich-ale-celia-saison"),
	},
}, {
	name: "merge union: string value set in src and not dst",
	inA: &validatedMergeTest{
		UnionField: testutil.UnionString("estrella-damn-daura"),
	},
	inB: &validatedMergeTest{},
	want: &validatedMergeTest{
		UnionField: testutil.UnionString("estrella-damn-daura"),
	},
}, {
	name: "merge union: string value set in dst and not src",
	inA:  &validatedMergeTest{},
	inB: &validatedMergeTest{
		UnionField: testutil.UnionString("two-brothers-prairie-path-golden-ale"),
	},
	want: &validatedMergeTest{
		UnionField: testutil.UnionString("two-brothers-prairie-path-golden-ale"),
	},
}, {
	name: "merge union: values not equal, and different types",
	inA: &validatedMergeTest{
		UnionField: testutil.UnionString("greens-amber"),
	},
	inB: &validatedMergeTest{
		UnionField: testutil.UnionInt64(42),
	},
	wantErr: "interface field was set in both src and dst and was not equal",
}, {
	name: "overwrite merge: values not equal, and different types",
	inA: &validatedMergeTest{
		UnionField: testutil.UnionString("greens-amber"),
	},
	inB: &validatedMergeTest{
		UnionField: testutil.UnionInt64(42),
	},
	inOpts: []MergeOpt{
		&MergeOverwriteExistingFields{},
	},
	want: &validatedMergeTest{
		UnionField: testutil.UnionInt64(42),
	},
}, {
	name: "merge union: enum values not equal",
	inA: &validatedMergeTest{
		UnionField: EnumTypeValue,
	},
	inB: &validatedMergeTest{
		UnionField: EnumTypeValueTwo,
	},
	wantErr: "interface field was set in both src and dst and was not equal",
}, {
	name: "merge union: binary values not equal",
	inA: &validatedMergeTest{
		UnionField: testBinary1,
	},
	inB: &validatedMergeTest{
		UnionField: testBinary2,
	},
	wantErr: "interface field was set in both src and dst and was not equal",
}, {
	name: "overwrite merge union: binary values not equal",
	inA: &validatedMergeTest{
		UnionField: testBinary1,
	},
	inB: &validatedMergeTest{
		UnionField: testBinary2,
	},
	inOpts: []MergeOpt{
		&MergeOverwriteExistingFields{},
	},
	want: &validatedMergeTest{
		UnionField: testBinary2,
	},
}, {
	name: "merge union: int values equal",
	inA: &validatedMergeTest{
		UnionField: testutil.UnionInt64(42),
	},
	inB: &validatedMergeTest{
		UnionField: testutil.UnionInt64(42),
	},
	want: &validatedMergeTest{
		UnionField: testutil.UnionInt64(42),
	},
}, {
	name: "merge union: binary values equal",
	inA: &validatedMergeTest{
		UnionField: testBinary1,
	},
	inB: &validatedMergeTest{
		UnionField: testBinary1,
	},
	want: &validatedMergeTest{
		UnionField: testBinary1,
	},
}, {
	name: "merge union: binary value set in src and not dst",
	inA: &validatedMergeTest{
		UnionField: testBinary1,
	},
	inB: &validatedMergeTest{},
	want: &validatedMergeTest{
		UnionField: testBinary1,
	},
}, {
	name: "merge union: binary value set in dst and not src",
	inA:  &validatedMergeTest{},
	inB: &validatedMergeTest{
		UnionField: testBinary1,
	},
	want: &validatedMergeTest{
		UnionField: testBinary1,
	},
}, {
	name: "merge union (wrapper union): values not equal",
	inA: &validatedMergeTest{
		UnionField: &copyUnionS{"glutenberg-ipa"},
	},
	inB: &validatedMergeTest{
		UnionField: &copyUnionS{"mikkeler-pale-peter-and-mary"},
	},
	wantErr: "interface field was set in both src and dst and was not equal",
}, {
	name: "overwrite merge union (wrapper union): values not equal",
	inA: &validatedMergeTest{
		UnionField: &copyUnionS{"glutenberg-ipa"},
	},
	inB: &validatedMergeTest{
		UnionField: &copyUnionS{"mikkeler-pale-peter-and-mary"},
	},
	inOpts: []MergeOpt{
		&MergeOverwriteExistingFields{},
	},
	want: &validatedMergeTest{
		UnionField: &copyUnionS{"mikkeler-pale-peter-and-mary"},
	},
}, {
	name: "merge union (wrapper union): values equal",
	inA: &validatedMergeTest{
		UnionField: &copyUnionS{"ipswich-ale-celia-saison"},
	},
	inB: &validatedMergeTest{
		UnionField: &copyUnionS{"ipswich-ale-celia-saison"},
	},
	want: &validatedMergeTest{
		UnionField: &copyUnionS{"ipswich-ale-celia-saison"},
	},
}, {
	name: "merge union (wrapper union): set in src and not dst",
	inA: &validatedMergeTest{
		UnionField: &copyUnionS{"estrella-damn-daura"},
	},
	inB: &validatedMergeTest{},
	want: &validatedMergeTest{
		UnionField: &copyUnionS{"estrella-damn-daura"},
	},
}, {
	name: "merge union (wrapper union): set in dst and not src",
	inA:  &validatedMergeTest{},
	inB: &validatedMergeTest{
		UnionField: &copyUnionS{"two-brothers-prairie-path-golden-ale"},
	},
	want: &validatedMergeTest{
		UnionField: &copyUnionS{"two-brothers-prairie-path-golden-ale"},
	},
}, {
	name: "merge union (wrapper union): values not equal, and different types",
	inA: &validatedMergeTest{
		UnionField: &copyUnionS{"greens-amber"},
	},
	inB: &validatedMergeTest{
		UnionField: &copyUnionI{42},
	},
	wantErr: "interface field was set in both src and dst and was not equal",
}, {
	name: "overwrite merge union (wrapper union): values not equal, and different types",
	inA: &validatedMergeTest{
		UnionField: &copyUnionS{"greens-amber"},
	},
	inB: &validatedMergeTest{
		UnionField: &copyUnionI{42},
	},
	inOpts: []MergeOpt{
		&MergeOverwriteExistingFields{},
	},
	want: &validatedMergeTest{
		UnionField: &copyUnionI{42},
	},
}}

func TestMergeStructs(t *testing.T) {
	// Tests that only apply to the extra copy steps performed in MergeStructs as
	// it does not mutate any inputs.
	tests := append(mergeStructTests, struct {
		name    string
		inA     GoStruct
		inB     GoStruct
		inOpts  []MergeOpt
		want    GoStruct
		wantErr string
	}{
		name:    "error, bad data in A",
		inA:     &validatedMergeTestTwo{I: "belleville-thames-surfer"},
		inB:     &validatedMergeTestTwo{String: String("fourpure-beartooth")},
		wantErr: "cannot DeepCopy struct: invalid interface type received: string",
	})

	for _, tt := range tests {
		got, err := MergeStructs(tt.inA, tt.inB, tt.inOpts...)
		if diff := errdiff.Substring(err, tt.wantErr); diff != "" {
			t.Errorf("%s: MergeStructs(%v, %v): did not get expected error status, %s", tt.name, tt.inA, tt.inB, diff)
		}

		if diff := pretty.Compare(got, tt.want); diff != "" {
			t.Errorf("%s: MergeStructs(%v, %v): did not get expected returned struct, diff(-got,+want):\n%s", tt.name, tt.inA, tt.inB, diff)
		}
	}
}

func TestMergeStructInto(t *testing.T) {
	for _, tt := range mergeStructTests {
		// Make a copy of inA here since it will get mutated.
		got, err := DeepCopy(tt.inA)
		if err != nil {
			t.Errorf("%s: DeepCopy(%v): unexpected error with testdata, %v", tt.name, tt.inA, err)
			continue
		}
		err = MergeStructInto(got, tt.inB, tt.inOpts...)
		if diff := errdiff.Substring(err, tt.wantErr); diff != "" {
			t.Errorf("%s: MergeStructInto(%v, %v): did not get expected error status, %s", tt.name, tt.inA, tt.inB, diff)
		}
		if err != nil {
			continue
		}

		if diff := pretty.Compare(got, tt.want); diff != "" {
			t.Errorf("%s: MergeStructInto(%v, %v): did not mutate inA struct correctly, diff(-got,+want):\n%s", tt.name, tt.inA, tt.inB, diff)
		}
	}
}

func TestValidateMap(t *testing.T) {
	tests := []struct {
		name        string
		inSrc       reflect.Value
		inDst       reflect.Value
		wantMapType *mapType
		wantErr     string
	}{{
		name:  "valid maps",
		inSrc: reflect.ValueOf(map[string]*copyTest{}),
		inDst: reflect.ValueOf(map[string]*copyTest{}),
		wantMapType: &mapType{
			key:   reflect.TypeOf(""),
			value: reflect.TypeOf(&copyTest{}),
		},
	}, {
		name:    "invalid src field, not a map",
		inSrc:   reflect.ValueOf(""),
		inDst:   reflect.ValueOf(map[string]*copyTest{}),
		wantErr: "invalid src field, was not a map, was: string",
	}, {
		name:    "invalid dst field, not a map",
		inSrc:   reflect.ValueOf(map[string]*copyTest{}),
		inDst:   reflect.ValueOf(uint32(42)),
		wantErr: "invalid dst field, was not a map, was: uint32",
	}, {
		name:    "invalid src and dst fields, do not have the same value type",
		inSrc:   reflect.ValueOf(map[string]string{}),
		inDst:   reflect.ValueOf(map[string]uint32{}),
		wantErr: "invalid maps, src and dst value types are different, string != uint32",
	}, {
		name:    "invalid src and dst field, not a struct ptr",
		inSrc:   reflect.ValueOf(map[string]copyTest{}),
		inDst:   reflect.ValueOf(map[string]copyTest{}),
		wantErr: "invalid maps, src or dst does not have a struct ptr element, src: struct, dst: struct",
	}, {
		name:    "invalid maps, src and dst key types differ",
		inSrc:   reflect.ValueOf(map[string]*copyTest{}),
		inDst:   reflect.ValueOf(map[uint32]*copyTest{}),
		wantErr: "invalid maps, src and dst key types are different, string != uint32",
	}}

	for _, tt := range tests {
		got, err := validateMap(tt.inSrc, tt.inDst)
		if err != nil {
			if err.Error() != tt.wantErr {
				t.Errorf("%s: validateMap(%v, %v): did not get expected error status, got: %v, wantErr: %v", tt.name, tt.inSrc, tt.inDst, err, tt.wantErr)
			}
			continue
		}

		if diff := pretty.Compare(got, tt.wantMapType); diff != "" {
			t.Errorf("%s: validateMap(%v, %v): did not get expected return mapType, diff(-got,+want):\n%s", tt.name, tt.inSrc, tt.inDst, diff)
		}
	}
}

func TestCopyErrorCases(t *testing.T) {
	type errorTest struct {
		name    string
		inSrc   reflect.Value
		inDst   reflect.Value
		wantErr string
	}

	mapErrs := []errorTest{
		{"bad src", reflect.ValueOf(""), reflect.ValueOf(map[string]string{}), "received a non-map type in src map field: string"},
		{"bad dst", reflect.ValueOf(map[string]string{}), reflect.ValueOf(uint32(42)), "received a non-map type in dst map field: uint32"},
	}
	for _, tt := range mapErrs {
		if err := copyMapField(tt.inDst, tt.inSrc); err == nil || err.Error() != tt.wantErr {
			t.Errorf("%s: copyMapField(%v, %v): did not get expected error, got: %v, want: %v", tt.name, tt.inSrc, tt.inDst, err, tt.wantErr)
		}
	}

	ptrErrs := []errorTest{
		{"non-ptr", reflect.ValueOf(""), reflect.ValueOf(""), "received non-ptr type: string"},
	}
	for _, tt := range ptrErrs {
		if err := copyPtrField(tt.inDst, tt.inSrc); err == nil || err.Error() != tt.wantErr {
			t.Errorf("%s: copyPtrField(%v, %v): did not get expected error, got: %v, want: %v", tt.name, tt.inSrc, tt.inDst, err, tt.wantErr)
		}
	}

	badDeepCopy := &errorCopyTest{I: "foobar"}
	wantBDCErr := "cannot DeepCopy struct: invalid interface type received: string"
	if _, err := DeepCopy(badDeepCopy); err == nil || err.Error() != wantBDCErr {
		t.Errorf("badDeepCopy: DeepCopy(%v): did not get expected error, got: %v, want: %v", badDeepCopy, err, wantBDCErr)
	}
}

func TestDeepCopy(t *testing.T) {
	tests := []struct {
		name             string
		in               *copyTest
		inKey            string
		wantErrSubstring string
	}{{
		name: "simple copy",
		in:   &copyTest{StringField: String("zaphod")},
	}, {
		name: "copy with map",
		in: &copyTest{
			StringMap: map[string]*copyTest{
				"just": {StringField: String("this guy")},
			},
		},
		inKey: "just",
	}, {
		name: "copy with slice",
		in: &copyTest{
			StringSlice: []string{"one"},
		},
	}, {
		name:             "nil inputs",
		wantErrSubstring: "got nil value",
	}}

	for _, tt := range tests {
		got, err := DeepCopy(tt.in)

		if err != nil {
			if diff := errdiff.Substring(err, tt.wantErrSubstring); diff != "" {
				t.Errorf("%s: DeepCopy(%#v): did not get expected error, %s", tt.name, tt.in, diff)
			}
			continue
		}

		if diff := pretty.Compare(got, tt.in); diff != "" {
			t.Errorf("%s: DeepCopy(%#v): did not get identical copy, diff(-got,+want):\n%s", tt.name, tt.in, diff)
		}

		// Check we got a copy that doesn't modify the original.
		gotC, ok := got.(*copyTest)
		if !ok {
			t.Errorf("%s: DeepCopy(%#v): did not get back the same type, got: %T, want: %T", tt.name, tt.in, got, tt.in)
		}

		if &gotC == &tt.in {
			t.Errorf("%s: DeepCopy(%#v): after copy, input and copy have same memory address: %v", tt.name, tt.in, &gotC)
		}

		if len(tt.in.StringMap) != 0 && tt.inKey != "" {
			if &tt.in.StringMap == &gotC.StringMap {
				t.Errorf("%s: DeepCopy(%#v): after copy, input map and copied map have the same address: %v", tt.name, tt.in, &gotC.StringMap)
			}

			if v, ok := tt.in.StringMap[tt.inKey]; ok {
				cv, cok := gotC.StringMap[tt.inKey]
				if !cok {
					t.Errorf("%s: DeepCopy(%#v): after copy, received map did not have correct key, want key: %v, got: %v", tt.name, tt.in, tt.inKey, gotC.StringMap)
				}

				if &v == &cv {
					t.Errorf("%s: DeepCopy(%#v): after copy, input map element and copied map element have the same address: %v", tt.name, tt.in, &cv)
				}
			}
		}

		if len(tt.in.StringSlice) != 0 {
			if &tt.in.StringSlice == &gotC.StringSlice {
				t.Errorf("%s: DeepCopy(%#v): after copy, input slice and copied slice have the same address: %v", tt.name, tt.in, &gotC.StringSlice)
			}
		}
	}
}

type buildEmptyTreeMergeTest struct {
	Son      *buildEmptyTreeMergeTestChild
	Daughter *buildEmptyTreeMergeTestChild
	String   *string
}

func (*buildEmptyTreeMergeTest) ΛValidate(...ValidationOption) error     { return nil }
func (*buildEmptyTreeMergeTest) IsYANGGoStruct()                         {}
func (*buildEmptyTreeMergeTest) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*buildEmptyTreeMergeTest) ΛBelongingModule() string                { return "" }

type buildEmptyTreeMergeTestChild struct {
	Grandson      *buildEmptyTreeMergeTestGrandchild
	Granddaughter *buildEmptyTreeMergeTestGrandchild
	String        *string
}

func (*buildEmptyTreeMergeTestChild) ΛValidate(...ValidationOption) error     { return nil }
func (*buildEmptyTreeMergeTestChild) IsYANGGoStruct()                         {}
func (*buildEmptyTreeMergeTestChild) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*buildEmptyTreeMergeTestChild) ΛBelongingModule() string                { return "" }

type buildEmptyTreeMergeTestGrandchild struct {
	String *string
}

func (*buildEmptyTreeMergeTestGrandchild) ΛValidate(...ValidationOption) error     { return nil }
func (*buildEmptyTreeMergeTestGrandchild) IsYANGGoStruct()                         {}
func (*buildEmptyTreeMergeTestGrandchild) ΛEnumTypeMap() map[string][]reflect.Type { return nil }
func (*buildEmptyTreeMergeTestGrandchild) ΛBelongingModule() string                { return "" }

func TestBuildEmptyTreeMerge(t *testing.T) {
	tests := []struct {
		name        string
		inStructA   *buildEmptyTreeMergeTest
		inStructB   *buildEmptyTreeMergeTest
		inBuildSonA bool
		inBuildSonB bool
		want        GoStruct
		wantErr     bool
	}{{
		name: "check with no build empty",
		inStructA: &buildEmptyTreeMergeTest{
			Son: &buildEmptyTreeMergeTestChild{
				String: String("blackwater-draw-brewing-co-contract-killer"),
			},
		},
		inStructB: &buildEmptyTreeMergeTest{
			Daughter: &buildEmptyTreeMergeTestChild{
				String: String("brazos-valley-brewing-7-spanish-angels"),
			},
		},
		want: &buildEmptyTreeMergeTest{
			Son: &buildEmptyTreeMergeTestChild{
				String: String("blackwater-draw-brewing-co-contract-killer"),
			},
			Daughter: &buildEmptyTreeMergeTestChild{
				String: String("brazos-valley-brewing-7-spanish-angels"),
			},
		},
	}, {
		name: "check with build empty on B",
		inStructA: &buildEmptyTreeMergeTest{
			Son: &buildEmptyTreeMergeTestChild{
				String: String("brazos-valley-brewing-mama-tried-ipa"),
				Grandson: &buildEmptyTreeMergeTestGrandchild{
					String: String("brazos-valley-brewing-killin'-time-blonde"),
				},
			},
		},
		inStructB: &buildEmptyTreeMergeTest{
			Daughter: &buildEmptyTreeMergeTestChild{
				String: String("brazos-valley-brewing-13th-can"),
				Granddaughter: &buildEmptyTreeMergeTestGrandchild{
					String: String("brazos-valley-brewing-silt-brown-ale"),
				},
			},
			Son: &buildEmptyTreeMergeTestChild{},
		},
		inBuildSonB: true,
		want: &buildEmptyTreeMergeTest{
			Son: &buildEmptyTreeMergeTestChild{
				String: String("brazos-valley-brewing-mama-tried-ipa"),
				Grandson: &buildEmptyTreeMergeTestGrandchild{
					String: String("brazos-valley-brewing-killin'-time-blonde"),
				},
				Granddaughter: &buildEmptyTreeMergeTestGrandchild{},
			},
			Daughter: &buildEmptyTreeMergeTestChild{
				String: String("brazos-valley-brewing-13th-can"),
				Granddaughter: &buildEmptyTreeMergeTestGrandchild{
					String: String("brazos-valley-brewing-silt-brown-ale"),
				},
			},
		},
	}, {
		name: "check with build empty on A",
		inStructA: &buildEmptyTreeMergeTest{
			Son: &buildEmptyTreeMergeTestChild{},
			Daughter: &buildEmptyTreeMergeTestChild{
				String: String("huff-brewing-orrange-blossom-saison"),
			},
		},
		inStructB: &buildEmptyTreeMergeTest{
			Son: &buildEmptyTreeMergeTestChild{
				String: String("brazos-valley-brewing-suma-babushka"),
				Grandson: &buildEmptyTreeMergeTestGrandchild{
					String: String("brazos-valley-brewing-big-spoon"),
				},
			},
		},
		inBuildSonA: true,
		want: &buildEmptyTreeMergeTest{
			Son: &buildEmptyTreeMergeTestChild{
				String: String("brazos-valley-brewing-suma-babushka"),
				Grandson: &buildEmptyTreeMergeTestGrandchild{
					String: String("brazos-valley-brewing-big-spoon"),
				},
				Granddaughter: &buildEmptyTreeMergeTestGrandchild{},
			},
			Daughter: &buildEmptyTreeMergeTestChild{
				String: String("huff-brewing-orrange-blossom-saison"),
			},
		},
	}}

	for _, tt := range tests {
		if tt.inBuildSonA {
			BuildEmptyTree(tt.inStructA.Son)
		}

		if tt.inBuildSonB {
			BuildEmptyTree(tt.inStructB.Son)
		}

		got, err := MergeStructs(tt.inStructA, tt.inStructB)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: MergeStructs(%v, %v): got unexpected error status, got: %v, wantErr: %v", tt.name, tt.inStructA, tt.inStructB, err, tt.wantErr)
		}
		if diff := pretty.Compare(got, tt.want); diff != "" {
			t.Errorf("%s: MergeStructs(%v, %v): did not get expected merge result, diff(-got,+want):\n%s", tt.name, tt.inStructA, tt.inStructB, diff)
		}

	}
}

func TestUniqueSlices(t *testing.T) {
	type stringPtrStruct struct {
		Foo *string
	}

	type sliceStruct struct {
		Bar []string
	}

	tests := []struct {
		name             string
		inA              reflect.Value
		inB              reflect.Value
		wantUnique       bool
		wantErrSubstring string
	}{{
		name:       "unique strings",
		inA:        reflect.ValueOf([]string{"zest-please"}),
		inB:        reflect.ValueOf([]string{"amarillo-single-hop-ipa"}),
		wantUnique: true,
	}, {
		name:       "unique integers",
		inA:        reflect.ValueOf([]int{1, 2, 3}),
		inB:        reflect.ValueOf([]int{4, 5, 6}),
		wantUnique: true,
	}, {
		name:             "error: mismatched types",
		inA:              reflect.ValueOf([]string{"american-dream"}),
		inB:              reflect.ValueOf([]int{42}),
		wantErrSubstring: "a and b do not contain the same type",
	}, {
		name:             "error: not slices",
		inA:              reflect.ValueOf("beer-geek-breakfast"),
		inB:              reflect.ValueOf([]string{"beer-mile"}),
		wantErrSubstring: "a and b must both be slices",
	}, {
		name:       "not unique, strings",
		inA:        reflect.ValueOf([]string{"beobrew-ipa", "berliner-weisse"}),
		inB:        reflect.ValueOf([]string{"beobrew-ipa", "big-worse"}),
		wantUnique: false,
	}, {
		name:       "not unique, integers",
		inA:        reflect.ValueOf([]int{42, 84, 96}),
		inB:        reflect.ValueOf([]int{128, 256, 42}),
		wantUnique: false,
	}, {
		name:       "unique, string ptr struct",
		inA:        reflect.ValueOf([]*stringPtrStruct{{String("belgian-tripel")}}),
		inB:        reflect.ValueOf([]*stringPtrStruct{{String("black-bear")}}),
		wantUnique: true,
	}, {
		name:       "not unique, string ptr struct",
		inA:        reflect.ValueOf([]*stringPtrStruct{{String("black-hole")}}),
		inB:        reflect.ValueOf([]*stringPtrStruct{{String("black-hole")}}),
		wantUnique: false,
	}, {
		name:       "unique, slice ptr struct",
		inA:        reflect.ValueOf([]*sliceStruct{{[]string{"california-dream"}}}),
		inB:        reflect.ValueOf([]*sliceStruct{{[]string{"caretaker"}}}),
		wantUnique: true,
	}, {
		name:       "not unique, slice ptr struct",
		inA:        reflect.ValueOf([]*sliceStruct{{[]string{"chill-pils"}}}),
		inB:        reflect.ValueOf([]*sliceStruct{{[]string{"chill-pils"}}}),
		wantUnique: false,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := uniqueSlices(tt.inA, tt.inB)
			if diff := errdiff.Substring(err, tt.wantErrSubstring); diff != "" {
				t.Fatalf("%s: uniqueSlices(%v, %v): did not get expected error, %s", tt.name, tt.inA, tt.inB, diff)
			}

			if want := tt.wantUnique; got != want {
				t.Fatalf("%s: uniqueSlices(%v, %v): did not get expected unique status, got: %v, want: %v", tt.name, tt.inA, tt.inB, got, want)
			}
		})
	}
}

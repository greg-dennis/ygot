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

package ygen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kylelemons/godebug/pretty"
	"github.com/openconfig/gnmi/errdiff"
	"github.com/openconfig/goyang/pkg/yang"
	"github.com/openconfig/ygot/genutil"
	"github.com/openconfig/ygot/testutil"
)

const (
	// TestRoot is the root of the test directory such that this is not
	// repeated when referencing files.
	TestRoot string = ""
	// deflakeRuns specifies the number of runs of code generation that
	// should be performed to check for flakes.
	deflakeRuns int = 10
)

// datapath is the path to common YANG test modules.
const datapath = "../testdata/modules"

// TestFindMappableEntities tests the extraction of elements that are to be mapped
// into Go code from a YANG schema.
func TestFindMappableEntities(t *testing.T) {
	tests := []struct {
		name          string        // name is an identifier for the test.
		in            *yang.Entry   // in is the yang.Entry corresponding to the YANG root element.
		inSkipModules []string      // inSkipModules is a slice of strings indicating modules to be skipped.
		inModules     []*yang.Entry // inModules is the set of modules that the code generation is for.
		// wantCompressed is a map keyed by the string "structs" or "enums" which contains a slice
		// of the YANG identifiers for the corresponding mappable entities that should be
		// found. wantCompressed is the set that are expected when compression is enabled.
		wantCompressed map[string][]string
		// wantUncompressed is a map of the same form as wantCompressed. It is the expected
		// result when compression is disabled.
		wantUncompressed map[string][]string
	}{{
		name: "base-test",
		in: &yang.Entry{
			Name: "module",
			Kind: yang.DirectoryEntry,
			Dir: map[string]*yang.Entry{
				"base": {
					Name: "base",
					Kind: yang.DirectoryEntry,
					Dir: map[string]*yang.Entry{
						"config": {
							Name: "config",
							Kind: yang.DirectoryEntry,
							Dir:  map[string]*yang.Entry{},
						},
						"state": {
							Name: "state",
							Kind: yang.DirectoryEntry,
							Dir:  map[string]*yang.Entry{},
						},
					},
				},
			},
		},
		wantCompressed: map[string][]string{
			"structs": {"base"},
			"enums":   {},
		},
		wantUncompressed: map[string][]string{
			"structs": {"base", "config", "state"},
			"enums":   {},
		},
	}, {
		name: "enum-test",
		in: &yang.Entry{
			Name: "module",
			Dir: map[string]*yang.Entry{
				"base": {
					Name: "base",
					Kind: yang.DirectoryEntry,
					Dir: map[string]*yang.Entry{
						"config": {
							Name: "config",
							Kind: yang.DirectoryEntry,
							Dir: map[string]*yang.Entry{
								"enumleaf": {
									Name: "enumleaf",
									Type: &yang.YangType{
										Kind: yang.Yenum,
									},
								},
							},
						},
						"state": {
							Name: "state",
							Kind: yang.DirectoryEntry,
							Dir: map[string]*yang.Entry{
								"enumleaf": {
									Name: "enumleaf",
									Type: &yang.YangType{
										Kind: yang.Yenum,
									},
								},
							},
						},
					},
				},
			},
		},
		wantCompressed: map[string][]string{
			"structs": {"base"},
			"enums":   {"enumleaf"},
		},
		wantUncompressed: map[string][]string{
			"structs": {"base", "config", "state"},
			"enums":   {"enumleaf"},
		},
	}, {
		name: "skip module",
		in: &yang.Entry{
			Name: "module",
			Kind: yang.DirectoryEntry,
			Dir: map[string]*yang.Entry{
				"ignored-container": {
					Name: "ignored-container",
					Kind: yang.DirectoryEntry,
					Dir:  map[string]*yang.Entry{},
					Node: &yang.Container{
						Name: "ignored-container",
						Parent: &yang.Module{
							Namespace: &yang.Value{
								Name: "module-namespace",
							},
						},
					},
				},
			},
			Node: &yang.Module{
				Namespace: &yang.Value{
					Name: "module-namespace",
				},
			},
		},
		inSkipModules: []string{"module"},
		inModules: []*yang.Entry{{
			Name: "module",
			Node: &yang.Module{
				Namespace: &yang.Value{
					Name: "module-namespace",
				},
			},
		}},
		wantCompressed: map[string][]string{
			"structs": {},
			"enums":   {},
		},
		wantUncompressed: map[string][]string{
			"structs": {},
			"enums":   {},
		},
	}, {
		name: "surrounding container for list at root",
		in: &yang.Entry{
			Name: "module",
			Kind: yang.DirectoryEntry,
			Dir: map[string]*yang.Entry{
				"surrounding-container": {
					Name: "surrounding-container",
					Kind: yang.DirectoryEntry,
					Dir: map[string]*yang.Entry{
						"child-list": {
							Name:     "child-list",
							Kind:     yang.DirectoryEntry,
							Dir:      map[string]*yang.Entry{},
							ListAttr: &yang.ListAttr{},
						},
					},
				},
			},
		},
		wantCompressed: map[string][]string{
			"structs": {"child-list"},
		},
		wantUncompressed: map[string][]string{
			"structs": {"surrounding-container", "child-list"},
		},
	}, {
		name: "choice/case at root",
		in: &yang.Entry{
			Name: "module",
			Dir: map[string]*yang.Entry{
				"choice": {
					Name: "choice",
					Kind: yang.ChoiceEntry,
					Dir: map[string]*yang.Entry{
						"case": {
							Name: "case",
							Kind: yang.CaseEntry,
							Dir: map[string]*yang.Entry{
								"container": {
									Name: "container",
									Kind: yang.DirectoryEntry,
									Dir:  map[string]*yang.Entry{},
								},
							},
						},
					},
				},
			},
		},
		wantCompressed: map[string][]string{
			"structs": {"container"},
		},
		wantUncompressed: map[string][]string{
			"structs": {"container"},
		},
	}, {
		name: "enumerated value within a union leaf",
		in: &yang.Entry{
			Name: "module",
			Kind: yang.DirectoryEntry,
			Dir: map[string]*yang.Entry{
				"leaf": {
					Name: "leaf",
					Type: &yang.YangType{
						Kind: yang.Yunion,
						Type: []*yang.YangType{
							{Kind: yang.Yenum},
						},
					},
				},
			},
		},
		wantCompressed:   map[string][]string{"enums": {"leaf"}},
		wantUncompressed: map[string][]string{"enums": {"leaf"}},
	}, {
		name: "identityref value within a union leaf",
		in: &yang.Entry{
			Name: "module",
			Kind: yang.DirectoryEntry,
			Dir: map[string]*yang.Entry{
				"leaf": {
					Name: "leaf",
					Type: &yang.YangType{
						Name: "union",
						Kind: yang.Yunion,
						Type: []*yang.YangType{
							{Kind: yang.Yidentityref},
							{Kind: yang.Yenum},
						},
					},
				},
			},
		},
		wantCompressed:   map[string][]string{"enums": {"leaf"}},
		wantUncompressed: map[string][]string{"enums": {"leaf"}},
	}, {
		name: "enumeration within a typedef which is a union",
		in: &yang.Entry{
			Name: "module",
			Kind: yang.DirectoryEntry,
			Dir: map[string]*yang.Entry{
				"leaf": {
					Name: "leaf",
					Type: &yang.YangType{
						Name: "newtype",
						Kind: yang.Yunion,
						Type: []*yang.YangType{
							{Kind: yang.Yenum},
							{Kind: yang.Yenum},
						},
					},
				},
			},
		},
		wantCompressed:   map[string][]string{"enums": {"leaf"}},
		wantUncompressed: map[string][]string{"enums": {"leaf"}},
	}, {
		name: "enumerated value within a choice that has a child",
		in: &yang.Entry{
			Name: "module",
			Kind: yang.DirectoryEntry,
			Dir: map[string]*yang.Entry{
				"choice": {
					Name: "choice",
					Kind: yang.ChoiceEntry,
					Dir: map[string]*yang.Entry{
						"case": {
							Name: "case",
							Kind: yang.CaseEntry,
							Dir: map[string]*yang.Entry{
								"container": {
									Name: "container",
									Kind: yang.DirectoryEntry,
									Dir: map[string]*yang.Entry{
										"choice-case-container-leaf": {
											Name: "choice-case-container-leaf",
											Type: &yang.YangType{Kind: yang.Yenum},
										},
									},
								},
							},
						},
						"case2": {
							Name: "case2",
							Kind: yang.CaseEntry,
							Dir: map[string]*yang.Entry{
								"choice-case2-leaf": {
									Name: "choice-case2-leaf",
									Type: &yang.YangType{Kind: yang.Yenum},
								},
							},
						},
						"direct": {
							Name: "direct",
							Type: &yang.YangType{Kind: yang.Yenum},
						},
					},
				},
			},
		},
		wantCompressed: map[string][]string{
			"structs": {"container"},
			"enums":   {"choice-case-container-leaf", "choice-case2-leaf", "direct"}},
		wantUncompressed: map[string][]string{
			"structs": {"container"},
			"enums":   {"choice-case-container-leaf", "choice-case2-leaf", "direct"}},
	}}

	for _, tt := range tests {
		testSpec := map[bool]map[string][]string{
			true:  tt.wantCompressed,
			false: tt.wantUncompressed,
		}

		for compress, expected := range testSpec {
			structs := make(map[string]*yang.Entry)
			enums := make(map[string]*yang.Entry)

			errs := findMappableEntities(tt.in, structs, enums, tt.inSkipModules, compress, tt.inModules)
			if errs != nil {
				t.Errorf("%s: findMappableEntities(compressEnabled: %v): got unexpected error, got: %v, want: nil", tt.name, compress, errs)
			}

			entityNames := func(m map[string]bool) []string {
				o := []string{}
				for k := range m {
					o = append(o, k)
				}
				return o
			}

			structOut := make(map[string]bool)
			enumOut := make(map[string]bool)
			for _, o := range structs {
				structOut[o.Name] = true
			}
			for _, e := range enums {
				enumOut[e.Name] = true
			}

			if len(expected["structs"]) != len(structOut) {
				t.Errorf("%s: findMappableEntities(compressEnabled: %v): did not get expected number of structs, got: %v, want: %v", tt.name, compress, entityNames(structOut), expected["structs"])
			}

			for _, e := range expected["structs"] {
				if !structOut[e] {
					t.Errorf("%s: findMappableEntities(compressEnabled: %v): struct %s was not found in %v\n", tt.name, compress, e, structOut)
				}
			}

			if len(expected["enums"]) != len(enumOut) {
				t.Errorf("%s: findMappableEntities(compressEnabled: %v): did not get expected number of enums, got: %v, want: %v", tt.name, compress, entityNames(enumOut), expected["enums"])
			}

			for _, e := range expected["enums"] {
				if !enumOut[e] {
					t.Errorf("%s: findMappableEntities(compressEnabled: %v): enum %s was not found in %v\n", tt.name, compress, e, enumOut)
				}
			}
		}
	}
}

// yangTestCase describs a test case for which code generation is performed
// through Goyang's API, it provides the input set of parameters in a way that
// can be reused across tests.
type yangTestCase struct {
	name                string          // Name is the identifier for the test.
	inFiles             []string        // inFiles is the set of inputFiles for the test.
	inIncludePaths      []string        // inIncludePaths is the set of paths that should be searched for imports.
	inExcludeModules    []string        // inExcludeModules is the set of modules that should be excluded from code generation.
	inConfig            GeneratorConfig // inConfig specifies the configuration that should be used for the generator test case.
	wantStructsCodeFile string          // wantsStructsCodeFile is the path of the generated Go code that the output of the test should be compared to.
	wantErrSubstring    string          // wantErrSubstring specifies whether the test should expect an error.
	wantSchemaFile      string          // wantSchemaFile is the path to the schema JSON that the output of the test should be compared to.
}

// TestSimpleStructs tests the processModules, GenerateGoCode and writeGoCode
// functions. It takes the set of YANG modules described in the slice of
// yangTestCases and generates the struct code for them, comparing the output
// to the wantStructsCodeFile.  In order to simplify the files that are used,
// the GenerateGoCode structs are concatenated before comparison with the
// expected output. If the generated code matches the expected output, it is
// run against the Go parser to ensure that the code is valid Go - this is
// expected, but it ensures that the input file does not contain Go which is
// invalid.
func TestSimpleStructs(t *testing.T) {
	tests := []yangTestCase{{
		name:    "simple openconfig test, with compression, with (useless) enum org name trimming",
		inFiles: []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions:    true,
				GenerateLeafGetters:     true,
				GeneratePopulateDefault: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				EnumOrgPrefixesToTrim:                []string{"openconfig"},
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-simple.formatted-txt"),
	}, {
		name:    "simple openconfig test, with excluded state, with compression, with enum org name trimming",
		inFiles: []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
				GenerateLeafGetters:  true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.ExcludeDerivedState,
				ShortenEnumLeafNames:                 true,
				EnumOrgPrefixesToTrim:                []string{"openconfig"},
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-simple-excludestate.formatted-txt"),
	}, {
		name:    "simple openconfig test, with no compression",
		inFiles: []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions:    true,
				GenerateLeafGetters:     true,
				GeneratePopulateDefault: true,
			},
			TransformationOptions: TransformationOpts{
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-simple-no-compress.formatted-txt"),
	}, {
		name:    "simple openconfig test, with compression, without shortened enum leaf names, with enum org name trimming",
		inFiles: []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				EnumOrgPrefixesToTrim:                []string{"openconfig"},
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-simple.long-enum-names.trimmed-enum.formatted-txt"),
	}, {
		name:    "simple openconfig test, with no compression, with enum org name trimming",
		inFiles: []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				ShortenEnumLeafNames:                 true,
				EnumOrgPrefixesToTrim:                []string{"openconfig"},
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-simple-no-compress.trimmed-enum.formatted-txt"),
	}, {
		name:    "OpenConfig leaf-list defaults test, with compression",
		inFiles: []string{filepath.Join(datapath, "openconfig-leaflist-default.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions:    true,
				GenerateLeafGetters:     true,
				GeneratePopulateDefault: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-leaflist-default.formatted-txt"),
	}, {
		name:    "OpenConfig schema test - with annotations",
		inFiles: []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				AddAnnotationFields:  true,
				AnnotationPrefix:     "ᗩ",
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "openconfig-simple-annotations.formatted-txt"),
	}, {
		name:    "OpenConfig schema test - list and associated method (rename, new)",
		inFiles: []string{filepath.Join(datapath, "openconfig-withlist.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GoOptions: GoOpts{
				GenerateRenameMethod: true,
				GenerateSimpleUnions: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-withlist.formatted-txt"),
	}, {
		name:    "OpenConfig schema test - list and associated method (rename, new) - using operational state",
		inFiles: []string{filepath.Join(datapath, "openconfig-withlist.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferOperationalState,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GoOptions: GoOpts{
				GenerateRenameMethod: true,
				GenerateSimpleUnions: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-withlist-opstate.formatted-txt"),
	}, {
		name:    "OpenConfig schema test - multi-keyed list key struct name conflict and associated method (rename, new)",
		inFiles: []string{filepath.Join(datapath, "openconfig-multikey-list-name-conflict.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GoOptions: GoOpts{
				GenerateRenameMethod: true,
				GenerateSimpleUnions: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-multikey-list-name-conflict.formatted-txt"),
	}, {
		name:    "simple openconfig test, with a list that has an enumeration key",
		inFiles: []string{filepath.Join(datapath, "openconfig-list-enum-key.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				IgnoreShadowSchemaPaths:              true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-list-enum-key.formatted-txt"),
	}, {
		name:    "simple openconfig test, with a list that has an enumeration key, with enum org name trimming",
		inFiles: []string{filepath.Join(datapath, "openconfig-list-enum-key.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				EnumOrgPrefixesToTrim:                []string{"openconfig"},
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-list-enum-key.trimmed-enum.formatted-txt"),
	}, {
		name:    "openconfig test with a identityref union",
		inFiles: []string{filepath.Join(datapath, "openconfig-unione.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-unione.formatted-txt"),
	}, {
		name:    "openconfig test with a identityref union (wrapper unions)",
		inFiles: []string{filepath.Join(datapath, "openconfig-unione.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-unione.wrapper-unions.formatted-txt"),
	}, {
		name:    "openconfig tests with fakeroot",
		inFiles: []string{filepath.Join(datapath, "openconfig-fakeroot.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-fakeroot.formatted-txt"),
	}, {
		name:    "openconfig noncompressed tests with fakeroot",
		inFiles: []string{filepath.Join(datapath, "openconfig-fakeroot.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-fakeroot-nc.formatted-txt"),
	}, {
		name:    "schema test with compression",
		inFiles: []string{filepath.Join(TestRoot, "testdata/schema/openconfig-options.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GenerateJSONSchema: true,
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/schema/openconfig-options-compress.formatted-txt"),
		wantSchemaFile:      filepath.Join(TestRoot, "testdata/schema/openconfig-options-compress-schema.json"),
	}, {
		name:    "schema test without compression",
		inFiles: []string{filepath.Join(TestRoot, "testdata/schema/openconfig-options.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				EnumerationsUseUnderscores: true,
			},
			GenerateJSONSchema: true,
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/schema/openconfig-options-nocompress.formatted-txt"),
		wantSchemaFile:      filepath.Join(TestRoot, "testdata/schema/openconfig-options-nocompress-schema.json"),
	}, {
		name:    "schema test with fakeroot",
		inFiles: []string{filepath.Join(TestRoot, "testdata/schema/openconfig-options.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GenerateJSONSchema: true,
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/schema/openconfig-options-compress-fakeroot.formatted-txt"),
		wantSchemaFile:      filepath.Join(TestRoot, "testdata/schema/openconfig-options-compress-fakeroot-schema.json"),
	}, {
		name:    "schema test with fakeroot and no compression",
		inFiles: []string{filepath.Join(TestRoot, "testdata/schema/openconfig-options.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:                     true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GenerateJSONSchema: true,
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/schema/openconfig-options-nocompress-fakeroot.formatted-txt"),
		wantSchemaFile:      filepath.Join(TestRoot, "testdata/schema/openconfig-options-nocompress-fakeroot-schema.json"),
	}, {
		name:    "schema test with camelcase annotations",
		inFiles: []string{filepath.Join(datapath, "openconfig-camelcase.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-camelcase-compress.formatted-txt"),
	}, {
		name:    "structs test with camelcase annotations",
		inFiles: []string{filepath.Join(datapath, "openconfig-enumcamelcase.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-enumcamelcase-compress.formatted-txt"),
	}, {
		name:                "structs test with choices and cases",
		inFiles:             []string{filepath.Join(datapath, "choice-case-example.yang")},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/choice-case-example.formatted-txt"),
	}, {
		name: "module with augments",
		inFiles: []string{
			filepath.Join(datapath, "openconfig-simple-target.yang"),
			filepath.Join(datapath, "openconfig-simple-augment.yang"),
		},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:          genutil.PreferIntendedConfig,
				GenerateFakeRoot:           true,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/openconfig-augmented.formatted-txt"),
	}, {
		name:    "variable and import explicitly specified",
		inFiles: []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				FakeRootName:                         "fakeroot",
				EnumerationsUseUnderscores:           true,
			},
			Caller:             "testcase",
			StoreRawSchema:     true,
			GenerateJSONSchema: true,
			GoOptions: GoOpts{
				SchemaVarName:        "YANGSchema",
				GoyangImportPath:     "foo/goyang",
				YgotImportPath:       "bar/ygot",
				YtypesImportPath:     "baz/ytypes",
				GenerateSimpleUnions: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/schema/openconfig-options-explicit.formatted-txt"),
		wantSchemaFile:      filepath.Join(TestRoot, "testdata/schema/openconfig-options-explicit-schema.json"),
	}, {
		name:    "module with entities at the root",
		inFiles: []string{filepath.Join(datapath, "root-entities.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				FakeRootName:               "fakeroot",
				GenerateFakeRoot:           true,
				EnumerationsUseUnderscores: true,
			},
			Caller: "testcase",
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/root-entities.formatted-txt"),
	}, {
		name:                "module with empty leaf",
		inFiles:             []string{filepath.Join(datapath, "empty.yang")},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/empty.formatted-txt"),
	}, {
		name:             "module with excluded modules",
		inFiles:          []string{filepath.Join(datapath, "excluded-module.yang")},
		inExcludeModules: []string{"excluded-module-two"},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				FakeRootName:               "office",
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/excluded-module.formatted-txt"),
	}, {
		name:    "module with excluded config false",
		inFiles: []string{filepath.Join(datapath, "", "openconfig-config-false.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:          genutil.UncompressedExcludeDerivedState,
				GenerateFakeRoot:           true,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "openconfig-config-false-uncompressed.formatted-txt"),
	}, {
		name:    "module with excluded config false - with compression",
		inFiles: []string{filepath.Join(datapath, "", "openconfig-config-false.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				CompressBehaviour:          genutil.ExcludeDerivedState,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "openconfig-config-false-compressed.formatted-txt"),
	}, {
		name:    "module with getters, delete and append methods",
		inFiles: []string{filepath.Join(datapath, "", "openconfig-list-enum-key.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				EnumerationsUseUnderscores: true,
			},
			GoOptions: GoOpts{
				GenerateAppendMethod: true,
				GenerateGetters:      true,
				GenerateDeleteMethod: true,
				GenerateSimpleUnions: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "openconfig-list-enum-key.getters-append.formatted-txt"),
	}, {
		name:    "module with excluded state, with RO list, path compression on",
		inFiles: []string{filepath.Join(datapath, "", "exclude-state-ro-list.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				CompressBehaviour:          genutil.ExcludeDerivedState,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "exclude-state-ro-list.formatted-txt"),
	}, {
		name:           "different union enumeration types",
		inFiles:        []string{filepath.Join(datapath, "", "enum-union.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
				GenerateLeafGetters:  true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-union.formatted-txt"),
	}, {
		name:           "different union enumeration types with consistent naming for union-inlined enums",
		inFiles:        []string{filepath.Join(datapath, "", "enum-union.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions:                true,
				GenerateLeafGetters:                 true,
				AppendEnumSuffixForSimpleUnionEnums: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-union.consistent.formatted-txt"),
	}, {
		name:           "different union enumeration types with default enum values",
		inFiles:        []string{filepath.Join(datapath, "", "enum-union-with-enum-defaults.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions:                true,
				GenerateLeafGetters:                 true,
				GeneratePopulateDefault:             true,
				AppendEnumSuffixForSimpleUnionEnums: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-union-with-enum-defaults.formatted-txt"),
	}, {
		name:           "different union enumeration types with default enum values (wrapper union)",
		inFiles:        []string{filepath.Join(datapath, "", "enum-union-with-enum-defaults.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateLeafGetters:                 true,
				GeneratePopulateDefault:             true,
				AppendEnumSuffixForSimpleUnionEnums: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantErrSubstring: "default value not supported for wrapper union values, please generate using simplified union leaves",
	}, {
		name:           "enumeration behaviour - resolution across submodules and grouping re-use within union",
		inFiles:        []string{filepath.Join(datapath, "", "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
				GenerateLeafGetters:  true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-module.formatted-txt"),
	}, {
		name:           "enumeration behaviour (wrapper unions) - resolution across submodules and grouping re-use within union",
		inFiles:        []string{filepath.Join(datapath, "", "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateLeafGetters: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-module.wrapper-unions.formatted-txt"),
	}, {
		name:           "enumeration behaviour - resolution across submodules and grouping re-use within union, with enumeration leaf names not shortened",
		inFiles:        []string{filepath.Join(datapath, "", "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-module.long-enum-names.formatted-txt"),
	}, {
		name:           "enumeration behaviour - resolution across submodules and grouping re-use within union, with typedef enum names being prefixed by the module of their use/residence rather than of their definition",
		inFiles:        []string{filepath.Join(datapath, "", "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:          genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:       true,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-module.residing-module-typedef-enum-name.formatted-txt"),
	}, {
		name:           "enumeration behaviour - resolution across submodules and grouping re-use within union, with typedef enum names being prefixed by the module of their use/residence rather than of their definition, and enumeration leaf names not shortened",
		inFiles:        []string{filepath.Join(datapath, "", "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:          genutil.PreferIntendedConfig,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-module.long-enum-names.residing-module-typedef-enum-name.formatted-txt"),
	}, {
		name:           "enumeration behaviour - resolution across submodules and grouping re-use within union, with typedef enum names being prefixed by the module of their use/residence rather than of their definition, and enumeration leaf names not shortened",
		inFiles:        []string{filepath.Join(datapath, "", "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:          genutil.PreferIntendedConfig,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-module.long-enum-names.residing-module-typedef-enum-name.wrapper-unions.formatted-txt"),
	}, {
		name:           "enumeration behaviour - multiple enumerations within a union",
		inFiles:        []string{filepath.Join(datapath, "", "enum-multi-module.yang")},
		inIncludePaths: []string{filepath.Join(datapath, "modules")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GenerateJSONSchema: true,
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-multi-module.formatted-txt"),
	}, {
		name:    "module with leaf getters",
		inFiles: []string{filepath.Join(datapath, "", "openconfig-list-enum-key.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				EnumerationsUseUnderscores:           true,
			},
			GoOptions: GoOpts{
				GenerateLeafGetters:     true,
				GeneratePopulateDefault: true,
				GenerateSimpleUnions:    true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "openconfig-list-enum-key.leaf-getters.formatted-txt"),
	}, {
		name:    "uncompressed module with two different enums",
		inFiles: []string{filepath.Join(datapath, "", "enum-list-uncompressed.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-list-uncompressed.formatted-txt"),
	}, {
		name:    "uncompressed module with two different enums (wrapper unions)",
		inFiles: []string{filepath.Join(datapath, "", "enum-list-uncompressed.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-list-uncompressed.wrapper-unions.formatted-txt"),
	}, {
		name:    "with model data",
		inFiles: []string{filepath.Join(datapath, "", "openconfig-versioned-mod.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				CompressBehaviour:          genutil.PreferIntendedConfig,
				EnumerationsUseUnderscores: true,
			},
			GoOptions: GoOpts{
				IncludeModelData:     true,
				GenerateSimpleUnions: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "openconfig-versioned-mod.formatted-txt"),
	}, {
		name:    "model with deduplicated enums",
		inFiles: []string{filepath.Join(datapath, "enum-duplication.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				EnumerationsUseUnderscores: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-duplication-dedup.formatted-txt"),
	}, {
		name:    "model with enums that are in the same grouping duplicated",
		inFiles: []string{filepath.Join(datapath, "enum-duplication.yang")},
		inConfig: GeneratorConfig{
			GoOptions: GoOpts{
				GenerateSimpleUnions: true,
			},
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				EnumerationsUseUnderscores: true,
			},
			ParseOptions: ParseOpts{
				SkipEnumDeduplication: true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata", "structs", "enum-duplication-dup.formatted-txt"),
	}, {
		name:    "OpenConfig schema test - list with binary key",
		inFiles: []string{filepath.Join(datapath, "openconfig-binary-list.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GoOptions: GoOpts{
				GenerateRenameMethod: true,
				GenerateSimpleUnions: true,
			},
		},
		wantErrSubstring: "has a binary key",
	}, {
		name:    "OpenConfig schema test - multi-keyed list with binary key",
		inFiles: []string{filepath.Join(datapath, "openconfig-binary-multi-list.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GoOptions: GoOpts{
				GenerateRenameMethod: true,
				GenerateSimpleUnions: true,
			},
		},
		wantErrSubstring: "has a binary key",
	}, {
		name:    "OpenConfig schema test - list with union key containing binary",
		inFiles: []string{filepath.Join(datapath, "openconfig-union-binary-list.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			GoOptions: GoOpts{
				GenerateRenameMethod: true,
				GenerateSimpleUnions: true,
			},
		},
		wantErrSubstring: "has a union key containing a binary",
	}, {
		name:    "module with presence containers",
		inFiles: []string{filepath.Join(datapath, "presence-container-example.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:           true,
				FakeRootName:               "device",
				EnumerationsUseUnderscores: true,
			},
			GoOptions: GoOpts{
				GenerateSimpleUnions:    true,
				GenerateLeafGetters:     true,
				GeneratePopulateDefault: true,
				AddYangPresence:         true,
			},
		},
		wantStructsCodeFile: filepath.Join(TestRoot, "testdata/structs/presence-container-example.formatted-txt"),
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			genCode := func() (*GeneratedGoCode, string, map[string]interface{}, error) {
				// Set defaults within the supplied configuration for these tests.
				if tt.inConfig.Caller == "" {
					// Set the name of the caller explicitly to avoid issues when
					// the unit tests are called by external test entities.
					tt.inConfig.Caller = "codegen-tests"
				}
				tt.inConfig.StoreRawSchema = true
				tt.inConfig.ParseOptions.ExcludeModules = tt.inExcludeModules

				cg := NewYANGCodeGenerator(&tt.inConfig)

				gotGeneratedCode, errs := cg.GenerateGoCode(tt.inFiles, tt.inIncludePaths)
				var err error
				if len(errs) > 0 {
					err = fmt.Errorf("%w", errs)
				}
				if diff := errdiff.Substring(err, tt.wantErrSubstring); diff != "" {
					t.Fatalf("%s: cg.GenerateCode(%v, %v): Config: %+v, Did not get expected error: %s", tt.name, tt.inFiles, tt.inIncludePaths, tt.inConfig, diff)
				}
				if err != nil {
					return nil, "", nil, err
				}

				// Write all the received structs into a single file such that
				// it can be compared to the received file.
				var gotCode bytes.Buffer
				fmt.Fprint(&gotCode, gotGeneratedCode.CommonHeader)
				fmt.Fprint(&gotCode, gotGeneratedCode.OneOffHeader)
				for _, gotStruct := range gotGeneratedCode.Structs {
					fmt.Fprint(&gotCode, gotStruct.String())
				}

				for _, gotEnum := range gotGeneratedCode.Enums {
					fmt.Fprint(&gotCode, gotEnum)
				}

				// Write generated enumeration map out.
				fmt.Fprint(&gotCode, gotGeneratedCode.EnumMap)

				var gotJSON map[string]interface{}
				if tt.inConfig.GenerateJSONSchema {
					// Write the schema byte array out.
					fmt.Fprint(&gotCode, gotGeneratedCode.JSONSchemaCode)
					fmt.Fprint(&gotCode, gotGeneratedCode.EnumTypeMap)

					if err := json.Unmarshal(gotGeneratedCode.RawJSONSchema, &gotJSON); err != nil {
						t.Fatalf("%s: json.Unmarshal(..., %v), could not unmarshal received JSON: %v", tt.name, gotGeneratedCode.RawJSONSchema, err)
					}
				}
				return gotGeneratedCode, gotCode.String(), gotJSON, nil
			}

			gotGeneratedCode, gotCode, gotJSON, err := genCode()
			if err != nil {
				return
			}

			if tt.wantSchemaFile != "" {
				wantSchema, rferr := ioutil.ReadFile(tt.wantSchemaFile)
				if rferr != nil {
					t.Fatalf("%s: ioutil.ReadFile(%q) error: %v", tt.name, tt.wantSchemaFile, rferr)
				}

				var wantJSON map[string]interface{}
				if err := json.Unmarshal(wantSchema, &wantJSON); err != nil {
					t.Fatalf("%s: json.Unmarshal(..., [contents of %s]), could not unmarshal golden JSON file: %v", tt.name, tt.wantSchemaFile, err)
				}

				if !cmp.Equal(gotJSON, wantJSON) {
					diff, _ := testutil.GenerateUnifiedDiff(string(wantSchema), string(gotGeneratedCode.RawJSONSchema))
					t.Fatalf("%s: GenerateGoCode(%v, %v), Config: %+v, did not return correct JSON (file: %v), diff: \n%s", tt.name, tt.inFiles, tt.inIncludePaths, tt.inConfig, tt.wantSchemaFile, diff)
				}
			}

			wantCodeBytes, rferr := ioutil.ReadFile(tt.wantStructsCodeFile)
			if rferr != nil {
				t.Fatalf("%s: ioutil.ReadFile(%q) error: %v", tt.name, tt.wantStructsCodeFile, rferr)
			}

			wantCode := string(wantCodeBytes)

			if gotCode != wantCode {
				// Use difflib to generate a unified diff between the
				// two code snippets such that this is simpler to debug
				// in the test output.
				diff, _ := testutil.GenerateUnifiedDiff(wantCode, gotCode)
				t.Errorf("%s: GenerateGoCode(%v, %v), Config: %+v, did not return correct code (file: %v), diff:\n%s",
					tt.name, tt.inFiles, tt.inIncludePaths, tt.inConfig, tt.wantStructsCodeFile, diff)
			}

			for i := 0; i < deflakeRuns; i++ {
				_, gotAttempt, _, _ := genCode()
				if gotAttempt != gotCode {
					diff, _ := testutil.GenerateUnifiedDiff(gotAttempt, gotCode)
					t.Fatalf("flaky code generation, diff:\n%s", diff)
				}
			}
		})
	}
}

func TestGenerateErrs(t *testing.T) {
	tests := []struct {
		name                  string
		inFiles               []string
		inPath                []string
		inConfig              GeneratorConfig
		wantGoOK              bool
		wantGoErrSubstring    string
		wantProtoOK           bool
		wantProtoErrSubstring string
		wantSameErrSubstring  bool
	}{{
		name:                 "missing YANG file",
		inFiles:              []string{filepath.Join(TestRoot, "testdata", "errors", "doesnt-exist.yang")},
		wantGoErrSubstring:   "no such file",
		wantSameErrSubstring: true,
	}, {
		name:                 "bad YANG file",
		inFiles:              []string{filepath.Join(TestRoot, "testdata", "errors", "bad-module.yang")},
		wantGoErrSubstring:   "syntax error",
		wantSameErrSubstring: true,
	}, {
		name:                 "missing import due to path",
		inFiles:              []string{filepath.Join(TestRoot, "testdata", "errors", "missing-import.yang")},
		wantGoErrSubstring:   "no such module",
		wantSameErrSubstring: true,
	}, {
		name:        "import satisfied due to path",
		inFiles:     []string{filepath.Join(TestRoot, "testdata", "errors", "missing-import.yang")},
		inPath:      []string{filepath.Join(TestRoot, "testdata", "errors", "subdir")},
		wantGoOK:    true,
		wantProtoOK: true,
	}}

	for _, tt := range tests {
		cg := NewYANGCodeGenerator(&tt.inConfig)

		_, goErr := cg.GenerateGoCode(tt.inFiles, tt.inPath)
		switch {
		case tt.wantGoOK && goErr != nil:
			t.Errorf("%s: cg.GenerateGoCode(%v, %v): got unexpected error, got: %v, want: nil", tt.name, tt.inFiles, tt.inPath, goErr)
		case tt.wantGoOK:
		default:
			if diff := errdiff.Substring(goErr, tt.wantGoErrSubstring); diff != "" {
				t.Errorf("%s: cg.GenerateGoCode(%v, %v): %v", tt.name, tt.inFiles, tt.inPath, diff)
			}
		}

		if tt.wantSameErrSubstring {
			tt.wantProtoErrSubstring = tt.wantGoErrSubstring
		}

		_, protoErr := cg.GenerateProto3(tt.inFiles, tt.inPath)
		switch {
		case tt.wantProtoOK && protoErr != nil:
			t.Errorf("%s: cg.GenerateProto3(%v, %v): got unexpected error, got: %v, want: nil", tt.name, tt.inFiles, tt.inPath, protoErr)
		case tt.wantProtoOK:
		default:
			if diff := errdiff.Substring(protoErr, tt.wantProtoErrSubstring); diff != "" {
				t.Errorf("%s: cg.GenerateProto3(%v, %v): %v", tt.name, tt.inFiles, tt.inPath, diff)
			}
		}

	}
}

func TestGetDirectoriesAndLeafTypes(t *testing.T) {
	tests := []struct {
		name           string
		inFiles        []string
		inIncludePaths []string
		inConfig       *DirectoryGenConfig
		wantDirMap     map[string]*Directory
		wantFieldPath  map[string]map[string]string
		wantTypeMap    map[string]map[string]*MappedType
	}{{
		name:           "simple openconfig test",
		inFiles:        []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inIncludePaths: []string{filepath.Join(TestRoot, "testdata", "structs")},
		inConfig: &DirectoryGenConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			ParseOptions: ParseOpts{
				ExcludeModules: []string{},
			},
		},
		wantDirMap: map[string]*Directory{
			"/openconfig-simple/parent": {
				Name: "Parent",
				Fields: map[string]*yang.Entry{
					"child": {Name: "child", Type: nil},
				},
				Path: []string{"", "openconfig-simple", "parent"},
			},
			"/openconfig-simple/parent/child": {
				Name: "Parent_Child",
				Fields: map[string]*yang.Entry{
					"one":   {Name: "one", Type: &yang.YangType{Kind: yang.Ystring}},
					"two":   {Name: "two", Type: &yang.YangType{Kind: yang.Ystring}},
					"three": {Name: "three", Type: &yang.YangType{Kind: yang.Yenum}},
					"four":  {Name: "four", Type: &yang.YangType{Kind: yang.Ybinary}},
				},
				Path: []string{"", "openconfig-simple", "parent", "child"},
			},
			"/openconfig-simple/remote-container": {
				Name: "RemoteContainer",
				Fields: map[string]*yang.Entry{
					"a-leaf": {Name: "a-leaf", Type: &yang.YangType{Kind: yang.Ystring}},
				},
				Path: []string{"", "openconfig-simple", "remote-container"},
			},
		},
		wantFieldPath: map[string]map[string]string{
			"/openconfig-simple/parent": {
				"child": "/openconfig-simple/parent/child",
			},
			"/openconfig-simple/parent/child": {
				"one":   "/openconfig-simple/parent/child/config/one",
				"two":   "/openconfig-simple/parent/child/state/two",
				"three": "/openconfig-simple/parent/child/config/three",
				"four":  "/openconfig-simple/parent/child/config/four",
			},
			"/openconfig-simple/remote-container": {
				"a-leaf": "/openconfig-simple/remote-container/config/a-leaf",
			},
		},
		wantTypeMap: map[string]map[string]*MappedType{
			"/openconfig-simple/parent": {
				"child": nil,
			},
			"/openconfig-simple/parent/child": {
				"one":   {NativeType: "string"},
				"two":   {NativeType: "string"},
				"three": {NativeType: "E_Child_Three", IsEnumeratedValue: true},
				"four":  {NativeType: "Binary"},
			},
			"/openconfig-simple/remote-container": {
				"a-leaf": {NativeType: "string"},
			},
		},
	}, {
		name:           "simple openconfig test with state prioritized",
		inFiles:        []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inIncludePaths: []string{filepath.Join(TestRoot, "testdata", "structs")},
		inConfig: &DirectoryGenConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferOperationalState,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			ParseOptions: ParseOpts{
				ExcludeModules: []string{},
			},
		},
		wantDirMap: map[string]*Directory{
			"/openconfig-simple/parent": {
				Name: "Parent",
				Fields: map[string]*yang.Entry{
					"child": {Name: "child", Type: nil},
				},
				Path: []string{"", "openconfig-simple", "parent"},
			},
			"/openconfig-simple/parent/child": {
				Name: "Parent_Child",
				Fields: map[string]*yang.Entry{
					"one":   {Name: "one", Type: &yang.YangType{Kind: yang.Ystring}},
					"two":   {Name: "two", Type: &yang.YangType{Kind: yang.Ystring}},
					"three": {Name: "three", Type: &yang.YangType{Kind: yang.Yenum}},
					"four":  {Name: "four", Type: &yang.YangType{Kind: yang.Ybinary}},
				},
				Path: []string{"", "openconfig-simple", "parent", "child"},
			},
			"/openconfig-simple/remote-container": {
				Name: "RemoteContainer",
				Fields: map[string]*yang.Entry{
					"a-leaf": {Name: "a-leaf", Type: &yang.YangType{Kind: yang.Ystring}},
				},
				Path: []string{"", "openconfig-simple", "remote-container"},
			},
		},
		wantFieldPath: map[string]map[string]string{
			"/openconfig-simple/parent": {
				"child": "/openconfig-simple/parent/child",
			},
			"/openconfig-simple/parent/child": {
				"one":   "/openconfig-simple/parent/child/state/one",
				"two":   "/openconfig-simple/parent/child/state/two",
				"three": "/openconfig-simple/parent/child/state/three",
				"four":  "/openconfig-simple/parent/child/state/four",
			},
			"/openconfig-simple/remote-container": {
				"a-leaf": "/openconfig-simple/remote-container/state/a-leaf",
			},
		},
		wantTypeMap: map[string]map[string]*MappedType{
			"/openconfig-simple/parent": {
				"child": nil,
			},
			"/openconfig-simple/parent/child": {
				"one":   {NativeType: "string"},
				"two":   {NativeType: "string"},
				"three": {NativeType: "E_Child_Three", IsEnumeratedValue: true},
				"four":  {NativeType: "Binary"},
			},
			"/openconfig-simple/remote-container": {
				"a-leaf": {NativeType: "string"},
			},
		},
	}, {
		name:           "enum openconfig test with enum-types module excluded",
		inFiles:        []string{filepath.Join(datapath, "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(TestRoot, "testdata", "structs")},
		inConfig: &DirectoryGenConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			ParseOptions: ParseOpts{
				ExcludeModules: []string{"enum-types"},
			},
		},
		wantDirMap: map[string]*Directory{
			"/enum-module/parent": {
				Name: "Parent",
				Fields: map[string]*yang.Entry{
					"child": {Name: "child", Type: nil},
				},
				Path: []string{"", "enum-module", "parent"},
			},
			"/enum-module/c": {
				Name: "C",
				Fields: map[string]*yang.Entry{
					"cl": {Name: "cl", Type: &yang.YangType{Kind: yang.Yenum}},
				},
				Path: []string{"", "enum-module", "c"},
			},
			"/enum-module/parent/child": {
				Name: "Parent_Child",
				Fields: map[string]*yang.Entry{
					"id":          {Name: "id", Type: &yang.YangType{Kind: yang.Yidentityref}},
					"enum":        {Name: "enum", Type: &yang.YangType{Kind: yang.Yenum}},
					"id2":         {Name: "id2", Type: &yang.YangType{Kind: yang.Yidentityref}},
					"inline-enum": {Name: "inline-enum", Type: &yang.YangType{Kind: yang.Yenum}},
				},
				Path: []string{"", "enum-module", "parent", "child"},
			},
			"/enum-module/a-lists/a-list": {
				Name: "AList",
				Fields: map[string]*yang.Entry{
					"value": {Name: "value", Type: &yang.YangType{Kind: yang.Yunion}},
				},
				Path: []string{"", "enum-module", "a-lists", "a-list"},
			},
			"/enum-module/b-lists/b-list": {
				Name: "BList",
				Fields: map[string]*yang.Entry{
					"value": {Name: "value", Type: &yang.YangType{Kind: yang.Yunion}},
				},
				Path: []string{"", "enum-module", "b-lists", "b-list"},
			},
		},
		wantTypeMap: map[string]map[string]*MappedType{
			"/enum-module/parent": {
				"child": nil,
			},
			"/enum-module/c": {
				"cl": {NativeType: "E_EnumModule_Cl", IsEnumeratedValue: true},
			},
			"/enum-module/parent/child": {
				"id":          {NativeType: "E_EnumTypes_ID", IsEnumeratedValue: true},
				"enum":        {NativeType: "E_EnumTypes_TdEnum", IsEnumeratedValue: true},
				"id2":         {NativeType: "E_EnumTypes_ID", IsEnumeratedValue: true},
				"inline-enum": {NativeType: "E_Child_InlineEnum", IsEnumeratedValue: true},
			},
			"/enum-module/a-lists/a-list": {
				"value": {NativeType: "AList_Value_Union"},
			},
			"/enum-module/b-lists/b-list": {
				"value": {NativeType: "BList_Value_Union"},
			},
		},
	}, {
		name:           "enum openconfig test with enum-types module and state excluded",
		inFiles:        []string{filepath.Join(datapath, "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(TestRoot, "testdata", "structs")},
		inConfig: &DirectoryGenConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.ExcludeDerivedState,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			ParseOptions: ParseOpts{
				ExcludeModules: []string{"enum-types"},
			},
		},
		wantDirMap: map[string]*Directory{
			"/enum-module/parent": {
				Name: "Parent",
				Fields: map[string]*yang.Entry{
					"child": {Name: "child", Type: nil},
				},
				Path: []string{"", "enum-module", "parent"},
			},
			"/enum-module/c": {
				Name: "C",
				Fields: map[string]*yang.Entry{
					"cl": {Name: "cl", Type: &yang.YangType{Kind: yang.Yenum}},
				},
				Path: []string{"", "enum-module", "c"},
			},
			"/enum-module/parent/child": {
				Name: "Parent_Child",
				Fields: map[string]*yang.Entry{
					"id":          {Name: "id", Type: &yang.YangType{Kind: yang.Yidentityref}},
					"id2":         {Name: "id2", Type: &yang.YangType{Kind: yang.Yidentityref}},
					"inline-enum": {Name: "inline-enum", Type: &yang.YangType{Kind: yang.Yenum}},
				},
				Path: []string{"", "enum-module", "parent", "child"},
			},
			"/enum-module/a-lists/a-list": {
				Name:   "AList",
				Fields: map[string]*yang.Entry{}, // Key is only part of state and thus is excluded.
				Path:   []string{"", "enum-module", "a-lists", "a-list"},
			},
			"/enum-module/b-lists/b-list": {
				Name:   "BList",
				Fields: map[string]*yang.Entry{},
				Path:   []string{"", "enum-module", "b-lists", "b-list"},
			},
		},
		wantTypeMap: map[string]map[string]*MappedType{
			"/enum-module/parent": {
				"child": nil,
			},
			"/enum-module/c": {
				"cl": {NativeType: "E_EnumModule_Cl", IsEnumeratedValue: true},
			},
			"/enum-module/parent/child": {
				"id":          {NativeType: "E_EnumTypes_ID", IsEnumeratedValue: true},
				"id2":         {NativeType: "E_EnumTypes_ID", IsEnumeratedValue: true},
				"inline-enum": {NativeType: "E_Child_InlineEnum", IsEnumeratedValue: true},
			},
			"/enum-module/a-lists/a-list": {},
			"/enum-module/b-lists/b-list": {},
		},
	}, {
		name:           "simple openconfig test with openconfig-simple module excluded",
		inFiles:        []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inIncludePaths: []string{filepath.Join(TestRoot, "testdata", "structs")},
		inConfig: &DirectoryGenConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			ParseOptions: ParseOpts{
				ExcludeModules: []string{"openconfig-simple"},
			},
		},
		wantDirMap:  map[string]*Directory{},
		wantTypeMap: map[string]map[string]*MappedType{},
	}, {
		name:           "simple openconfig test with fakeroot",
		inFiles:        []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inIncludePaths: []string{filepath.Join(TestRoot, "testdata", "structs")},
		inConfig: &DirectoryGenConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			ParseOptions: ParseOpts{
				ExcludeModules: []string{},
			},
		},
		wantDirMap: map[string]*Directory{
			"/device": {
				Name: "Device",
				Fields: map[string]*yang.Entry{
					"parent":           {Name: "parent", Type: nil},
					"remote-container": {Name: "remote-container", Type: nil},
				},
				Path: []string{"", "device"},
			},
			"/openconfig-simple/parent": {
				Name: "Parent",
				Fields: map[string]*yang.Entry{
					"child": {Name: "child", Type: nil},
				},
				Path: []string{"", "openconfig-simple", "parent"},
			},
			"/openconfig-simple/parent/child": {
				Name: "Parent_Child",
				Fields: map[string]*yang.Entry{
					"one":   {Name: "one", Type: &yang.YangType{Kind: yang.Ystring}},
					"two":   {Name: "two", Type: &yang.YangType{Kind: yang.Ystring}},
					"three": {Name: "three", Type: &yang.YangType{Kind: yang.Yenum}},
					"four":  {Name: "four", Type: &yang.YangType{Kind: yang.Ybinary}},
				},
				Path: []string{"", "openconfig-simple", "parent", "child"},
			},
			"/openconfig-simple/remote-container": {
				Name: "RemoteContainer",
				Fields: map[string]*yang.Entry{
					"a-leaf": {Name: "a-leaf", Type: &yang.YangType{Kind: yang.Ystring}},
				},
				Path: []string{"", "openconfig-simple", "remote-container"},
			},
		},
		wantTypeMap: map[string]map[string]*MappedType{
			"/device": {
				"parent":           nil,
				"remote-container": nil,
			},
			"/openconfig-simple/parent": {
				"child": nil,
			},
			"/openconfig-simple/parent/child": {
				"one":   {NativeType: "string"},
				"two":   {NativeType: "string"},
				"three": {NativeType: "E_Child_Three", IsEnumeratedValue: true},
				"four":  {NativeType: "Binary"},
			},
			"/openconfig-simple/remote-container": {
				"a-leaf": {NativeType: "string"},
			},
		},
	}, {
		name:           "enum openconfig test with enum-types module excluded with fakeroot",
		inFiles:        []string{filepath.Join(datapath, "enum-module.yang")},
		inIncludePaths: []string{filepath.Join(TestRoot, "testdata", "structs")},
		inConfig: &DirectoryGenConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			ParseOptions: ParseOpts{
				ExcludeModules: []string{"enum-types"},
			},
		},
		wantDirMap: map[string]*Directory{
			"/device": {
				Name: "Device",
				Fields: map[string]*yang.Entry{
					"parent": {Name: "parent", Type: nil},
					"c":      {Name: "c", Type: nil},
					"a-list": {Name: "a-list", Type: nil},
					"b-list": {Name: "b-list", Type: nil},
				},
				Path: []string{"", "device"},
			},
			"/enum-module/parent": {
				Name: "Parent",
				Fields: map[string]*yang.Entry{
					"child": {Name: "child", Type: nil},
				},
				Path: []string{"", "enum-module", "parent"},
			},
			"/enum-module/c": {
				Name: "C",
				Fields: map[string]*yang.Entry{
					"cl": {Name: "cl", Type: &yang.YangType{Kind: yang.Yenum}},
				},
				Path: []string{"", "enum-module", "c"},
			},
			"/enum-module/parent/child": {
				Name: "Parent_Child",
				Fields: map[string]*yang.Entry{
					"id":          {Name: "id", Type: &yang.YangType{Kind: yang.Yidentityref}},
					"enum":        {Name: "enum", Type: &yang.YangType{Kind: yang.Yenum}},
					"id2":         {Name: "id2", Type: &yang.YangType{Kind: yang.Yidentityref}},
					"inline-enum": {Name: "inline-enum", Type: &yang.YangType{Kind: yang.Yenum}},
				},
				Path: []string{"", "enum-module", "parent", "child"},
			},
			"/enum-module/a-lists/a-list": {
				Name: "AList",
				Fields: map[string]*yang.Entry{
					"value": {Name: "value", Type: &yang.YangType{Kind: yang.Yunion}},
				},
				Path: []string{"", "enum-module", "a-lists", "a-list"},
			},
			"/enum-module/b-lists/b-list": {
				Name: "BList",
				Fields: map[string]*yang.Entry{
					"value": {Name: "value", Type: &yang.YangType{Kind: yang.Yunion}},
				},
				Path: []string{"", "enum-module", "b-lists", "b-list"},
			},
		},
		wantTypeMap: map[string]map[string]*MappedType{
			"/device": {
				"parent": nil,
				"c":      nil,
				"a-list": nil,
				"b-list": nil,
			},
			"/enum-module/parent": {
				"child": nil,
			},
			"/enum-module/c": {
				"cl": {NativeType: "E_EnumModule_Cl", IsEnumeratedValue: true},
			},
			"/enum-module/parent/child": {
				"id":          {NativeType: "E_EnumTypes_ID", IsEnumeratedValue: true},
				"enum":        {NativeType: "E_EnumTypes_TdEnum", IsEnumeratedValue: true},
				"id2":         {NativeType: "E_EnumTypes_ID", IsEnumeratedValue: true},
				"inline-enum": {NativeType: "E_Child_InlineEnum", IsEnumeratedValue: true},
			},
			"/enum-module/a-lists/a-list": {
				"value": {NativeType: "AList_Value_Union"},
			},
			"/enum-module/b-lists/b-list": {
				"value": {NativeType: "BList_Value_Union"},
			},
		},
	}, {
		name:           "simple openconfig test with openconfig-simple module excluded with fakeroot",
		inFiles:        []string{filepath.Join(datapath, "openconfig-simple.yang")},
		inIncludePaths: []string{filepath.Join(TestRoot, "testdata", "structs")},
		inConfig: &DirectoryGenConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				GenerateFakeRoot:                     true,
				ShortenEnumLeafNames:                 true,
				UseDefiningModuleForTypedefEnumNames: true,
				EnumerationsUseUnderscores:           true,
			},
			ParseOptions: ParseOpts{
				ExcludeModules: []string{"openconfig-simple"},
			},
		},
		wantDirMap: map[string]*Directory{
			"/device": {
				Name:   "Device",
				Fields: map[string]*yang.Entry{},
				Path:   []string{"", "device"},
			},
		},
		wantTypeMap: map[string]map[string]*MappedType{
			"/device": {},
		},
	}}

	// Simple helper function for error messages
	fieldNames := func(dir *Directory) []string {
		names := []string{}
		for k := range dir.Fields {
			names = append(names, k)
		}
		return names
	}

	for _, tt := range tests {
		c := tt.inConfig
		t.Run(fmt.Sprintf("%s:GetDirectoriesAndLeafTypes(compressBehaviour:%v,GenerateFakeRoot:%v)", tt.name, c.TransformationOptions.CompressBehaviour, c.TransformationOptions.GenerateFakeRoot), func(t *testing.T) {
			gotDirMap, gotTypeMap, errs := c.GetDirectoriesAndLeafTypes(tt.inFiles, tt.inIncludePaths)
			if errs != nil {
				t.Fatal(errs)
			}

			// This checks the "Name" and "Path" attributes of the output Directories.
			if diff := cmp.Diff(tt.wantDirMap, gotDirMap, cmpopts.IgnoreFields(Directory{}, "Entry", "Fields", "ShadowedFields", "ListAttr", "IsFakeRoot"), cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("(-want +got):\n%s", diff)
			}

			// Verify certain fields of the "Fields" attribute -- there are too many fields to ignore to use cmp.Diff for comparison.
			for gotDirName, gotDir := range gotDirMap {
				// Note that any missing or extra Directories would've been caught with the previous check.
				wantDir := tt.wantDirMap[gotDirName]
				if len(gotDir.Fields) != len(wantDir.Fields) {
					t.Fatalf("director %q: Did not get expected set of fields, got: %v, want: %v", gotDirName, fieldNames(gotDir), fieldNames(wantDir))
				}
				for fieldk, wantField := range wantDir.Fields {
					gotField, ok := gotDir.Fields[fieldk]
					if !ok {
						t.Errorf("Could not find expected field %q in %q, gotDir.Fields: %v", fieldk, gotDirName, gotDir.Fields)
						continue // Fatal error for this field only.
					}

					if gotField.Name != wantField.Name {
						t.Errorf("Field %q of %q did not have expected name, got: %v, want: %v", fieldk, gotDirName, gotField.Name, wantField.Name)
					}

					if gotField.Type != nil && wantField.Type != nil && gotField.Type.Kind != wantField.Type.Kind {
						t.Errorf("Field %q of %q did not have expected type, got: %v, want: %v", fieldk, gotDirName, gotField.Type.Kind, wantField.Type.Kind)
					}

					if tt.wantFieldPath != nil && gotField.Path() != tt.wantFieldPath[gotDirName][fieldk] {
						t.Errorf("Field %q of %q did not have expected path, got: %v, want: %v", fieldk, gotDirName, gotField.Path(), tt.wantFieldPath[gotDirName][fieldk])
					}
				}
			}
			// The other attributes for wantDir are not tested, as
			// most of the work is passed to mappedDefinitions()
			// and buildDirectoryDefinitions(), making a good
			// quick check here sufficient.

			// This checks the "NativeType" and "IsEnumeratedValue" attributes of the output leaf types.
			// Since this is an integration test, many lower-level detail checks are omitted.
			if diff := cmp.Diff(tt.wantTypeMap, gotTypeMap, cmpopts.IgnoreFields(MappedType{}, "UnionTypes", "ZeroValue", "DefaultValue")); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestFindRootEntries(t *testing.T) {
	tests := []struct {
		name                       string
		inStructs                  map[string]*yang.Entry
		inRootElems                []*yang.Entry
		inRootName                 string
		wantCompressRootChildren   []string
		wantUncompressRootChildren []string
	}{{
		name: "directory at root",
		inStructs: map[string]*yang.Entry{
			"/foo": {
				Name: "foo",
				Dir:  map[string]*yang.Entry{},
				Kind: yang.DirectoryEntry,
				Parent: &yang.Entry{
					Name: "module",
				},
			},
			"/foo/bar": {
				Name: "bar",
				Dir:  map[string]*yang.Entry{},
				Kind: yang.DirectoryEntry,
				Parent: &yang.Entry{
					Name: "foo",
					Parent: &yang.Entry{
						Name: "module",
					},
				},
			},
		},
		inRootName:                 "fakeroot",
		wantCompressRootChildren:   []string{"foo"},
		wantUncompressRootChildren: []string{"foo"},
	}, {
		name: "directory and leaf at root",
		inStructs: map[string]*yang.Entry{
			"/foo": {
				Name: "foo",
				Dir:  map[string]*yang.Entry{},
				Kind: yang.DirectoryEntry,
				Parent: &yang.Entry{
					Name: "module",
				},
			},
		},
		inRootElems: []*yang.Entry{{
			Name: "foo",
			Dir:  map[string]*yang.Entry{},
			Kind: yang.DirectoryEntry,
			Parent: &yang.Entry{
				Name: "module",
			},
		}, {
			Name: "leaf",
			Type: &yang.YangType{
				Kind: yang.Ystring,
			},
			Parent: &yang.Entry{
				Name: "module",
			},
		}},
		inRootName:                 "fakeroot",
		wantCompressRootChildren:   []string{"foo", "leaf"},
		wantUncompressRootChildren: []string{"foo", "leaf"},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for compress, wantChildren := range map[bool][]string{true: tt.wantCompressRootChildren, false: tt.wantUncompressRootChildren} {
				if err := createFakeRoot(tt.inStructs, tt.inRootElems, tt.inRootName, compress); err != nil {
					t.Errorf("cg.createFakeRoot(%v), compressEnabled: %v, got unexpected error: %v", tt.inStructs, compress, err)
					continue
				}

				rootElem, ok := tt.inStructs["/"]
				if !ok {
					t.Errorf("cg.createFakeRoot(%v), compressEnabled: %v, could not find root element", tt.inStructs, compress)
					continue
				}

				gotChildren := map[string]bool{}
				for n := range rootElem.Dir {
					gotChildren[n] = true
				}

				for _, ch := range wantChildren {
					if _, ok := rootElem.Dir[ch]; !ok {
						t.Errorf("cg.createFakeRoot(%v), compressEnabled: %v, could not find child %v in %v", tt.inStructs, compress, ch, rootElem.Dir)
					}
					gotChildren[ch] = false
				}

				for ch, ok := range gotChildren {
					if ok == true {
						t.Errorf("cg.findRootentries(%v), compressEnabled: %v, did not expect child %v", tt.inStructs, compress, ch)
					}
				}
			}
		})
	}
}

func TestGenerateProto3(t *testing.T) {
	tests := []struct {
		name           string
		inFiles        []string
		inIncludePaths []string
		inConfig       GeneratorConfig
		// wantOutputFiles is a map keyed on protobuf package name with a path
		// to the file that is expected for each package.
		wantOutputFiles map[string]string
		wantErr         bool
	}{{
		name:    "simple protobuf test with compression",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-test-a.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour: genutil.PreferIntendedConfig,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig":        filepath.Join(TestRoot, "testdata", "proto", "proto-test-a.compress.parent.formatted-txt"),
			"openconfig.parent": filepath.Join(TestRoot, "testdata", "proto", "proto-test-a.compress.parent.child.formatted-txt"),
		},
	}, {
		name:    "simple protobuf test without compression",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-test-a.yang")},
		wantOutputFiles: map[string]string{
			"openconfig.proto_test_a":              filepath.Join(TestRoot, "testdata", "proto", "proto-test-a.nocompress.formatted-txt"),
			"openconfig.proto_test_a.parent":       filepath.Join(TestRoot, "testdata", "proto", "proto-test-a.nocompress.parent.formatted-txt"),
			"openconfig.proto_test_a.parent.child": filepath.Join(TestRoot, "testdata", "proto", "proto-test-a.nocompress.parent.child.formatted-txt"),
		},
	}, {
		name:    "enumeration under unions test with compression",
		inFiles: []string{filepath.Join(datapath, "enum-union.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				GenerateFakeRoot:                     true,
				UseDefiningModuleForTypedefEnumNames: true,
			},
			ProtoOptions: ProtoOpts{
				AnnotateEnumNames: true,
				NestedMessages:    true,
				GoPackageBase:     "github.com/foo/bar",
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig":       filepath.Join(TestRoot, "testdata", "proto", "enum-union.compress.formatted-txt"),
			"openconfig.enums": filepath.Join(TestRoot, "testdata", "proto", "enum-union.compress.enums.formatted-txt"),
		},
	}, {
		name:     "yang schema with a list",
		inFiles:  []string{filepath.Join(TestRoot, "testdata", "proto", "proto-test-b.yang")},
		inConfig: GeneratorConfig{TransformationOptions: TransformationOpts{CompressBehaviour: genutil.PreferIntendedConfig}},
		wantOutputFiles: map[string]string{
			"openconfig":        filepath.Join(TestRoot, "testdata", "proto", "proto-test-b.compress.formatted-txt"),
			"openconfig.device": filepath.Join(TestRoot, "testdata", "proto", "proto-test-b.compress.device.formatted-txt"),
		},
	}, {
		name:    "yang schema with simple enumerations",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-test-c.yang")},
		inConfig: GeneratorConfig{
			ProtoOptions: ProtoOpts{
				GoPackageBase: "github.com/foo/baz",
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.proto_test_c":              filepath.Join(TestRoot, "testdata", "proto", "proto-test-c.proto-test-c.formatted-txt"),
			"openconfig.proto_test_c.entity":       filepath.Join(TestRoot, "testdata", "proto", "proto-test-c.proto-test-c.entity.formatted-txt"),
			"openconfig.proto_test_c.elists":       filepath.Join(TestRoot, "testdata", "proto", "proto-test-c.proto-test-c.elists.formatted-txt"),
			"openconfig.proto_test_c.elists.elist": filepath.Join(TestRoot, "testdata", "proto", "proto-test-c.proto-test-c.elists.elist.formatted-txt"),
			"openconfig.enums":                     filepath.Join(TestRoot, "testdata", "proto", "proto-test-c.enums.formatted-txt"),
		},
	}, {
		name:    "yang schema with identityref and enumerated typedef, compression off",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-test-d.yang")},
		wantOutputFiles: map[string]string{
			"openconfig.proto_test_d":      filepath.Join(TestRoot, "testdata", "proto", "proto-test-d.uncompressed.proto-test-d.formatted-txt"),
			"openconfig.proto_test_d.test": filepath.Join(TestRoot, "testdata", "proto", "proto-test-d.uncompressed.proto-test-d.test.formatted-txt"),
			"openconfig.enums":             filepath.Join(TestRoot, "testdata", "proto", "proto-test-d.uncompressed.enums.formatted-txt"),
		},
	}, {
		name:    "yang schema with unions",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				UseDefiningModuleForTypedefEnumNames: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.proto_test_e":                filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.uncompressed.proto-test-e.formatted-txt"),
			"openconfig.proto_test_e.test":           filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.uncompressed.proto-test-e.test.formatted-txt"),
			"openconfig.proto_test_e.foos":           filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.uncompressed.proto-test-e.foos.formatted-txt"),
			"openconfig.proto_test_e.foos.foo":       filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.uncompressed.proto-test-e.foos.foo.formatted-txt"),
			"openconfig.proto_test_e.bars":           filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.uncompressed.proto-test-e.bars.formatted-txt"),
			"openconfig.enums":                       filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.uncompressed.enums.formatted-txt"),
			"openconfig.proto_test_e.animals":        filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.uncompressed.proto-test-e.animals.formatted-txt"),
			"openconfig.proto_test_e.animals.animal": filepath.Join(TestRoot, "testdata", "proto", "proto-test-e.uncompressed.proto-test-e.animals.animal.formatted-txt"),
		},
	}, {
		name:    "yang schema with anydata",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-anydata-test.yang")},
		wantOutputFiles: map[string]string{
			"openconfig.proto_anydata_test":   filepath.Join(TestRoot, "testdata", "proto", "proto_anydata_test.formatted-txt"),
			"openconfig.proto_anydata_test.e": filepath.Join(TestRoot, "testdata", "proto", "proto_anydata_test.e.formatted-txt"),
		},
	}, {
		name:    "yang schema with path annotations",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-test-f.yang")},
		inConfig: GeneratorConfig{
			ProtoOptions: ProtoOpts{
				AnnotateSchemaPaths: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.proto_test_f":     filepath.Join(TestRoot, "testdata", "proto", "proto_test_f.uncompressed.proto_test_f.formatted-txt"),
			"openconfig.proto_test_f.a":   filepath.Join(TestRoot, "testdata", "proto", "proto_test_f.uncompressed.proto_test_f.a.formatted-txt"),
			"openconfig.proto_test_f.a.c": filepath.Join(TestRoot, "testdata", "proto", "proto_test_f.uncompressed.proto_test_f.a.c.formatted-txt"),
		},
	}, {
		name:    "yang schema with leafrefs that point to the same path",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-test-g.yang")},
		inConfig: GeneratorConfig{
			ProtoOptions: ProtoOpts{
				GoPackageBase:  "github.com/foo/baz",
				NestedMessages: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.proto_test_g": filepath.Join(TestRoot, "testdata", "proto", "proto-test-g.proto-test-g.formatted-txt"),
		},
	}, {
		name:    "yang schema with fake root, path compression and union list key",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour: genutil.PreferIntendedConfig,
				GenerateFakeRoot:  true,
			},
			ProtoOptions: ProtoOpts{
				AnnotateSchemaPaths: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig":                filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.compressed.openconfig.formatted-txt"),
			"openconfig.routing_policy": filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.compressed.openconfig.routing_policy.formatted-txt"),
		},
	}, {
		name:    "yang schema with fakeroot, and union list key",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot: true,
			},
			ProtoOptions: ProtoOpts{
				AnnotateSchemaPaths: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig":                                                     filepath.Join(TestRoot, "testdata", "proto", "proto-union-list_key.uncompressed.openconfig.formatted-txt"),
			"openconfig.proto_union_list_key":                                filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.uncompressed.openconfig.proto_union_list_key.formatted-txt"),
			"openconfig.proto_union_list_key.routing_policy":                 filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.uncompressed.openconfig.proto_union_list_key.routing_policy.formatted-txt"),
			"openconfig.proto_union_list_key.routing_policy.policies":        filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.uncompressed.openconfig.proto_union_list_key.routing_policy.policies.formatted-txt"),
			"openconfig.proto_union_list_key.routing_policy.policies.policy": filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.uncompressed.openconfig.proto_union_list_key.routing_policy.policies.policy.formatted-txt"),
			"openconfig.proto_union_list_key.routing_policy.sets":            filepath.Join(TestRoot, "testdata", "proto", "proto-union-list-key.uncompressed.openconfig.proto_union_list_key.routing_policy.sets.formatted-txt"),
		},
	}, {
		name:    "enums: yang schema with various types of enums with underscores",
		inFiles: []string{filepath.Join(TestRoot, "testdata", "proto", "proto-enums.yang")},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				UseDefiningModuleForTypedefEnumNames: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.enums":       filepath.Join(TestRoot, "testdata", "proto", "proto-enums.enums.formatted-txt"),
			"openconfig.proto_enums": filepath.Join(TestRoot, "testdata", "proto", "proto-enums.formatted-txt"),
		},
	}, {
		name: "enums: yang schema with identity that adds to previous module",
		inFiles: []string{
			filepath.Join(TestRoot, "testdata", "proto", "proto-enums.yang"),
			filepath.Join(TestRoot, "testdata", "proto", "proto-enums-addid.yang"),
		},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				UseDefiningModuleForTypedefEnumNames: true,
			},
			ProtoOptions: ProtoOpts{
				AnnotateEnumNames: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.enums":       filepath.Join(TestRoot, "testdata", "proto", "proto-enums-addid.enums.formatted-txt"),
			"openconfig.proto_enums": filepath.Join(TestRoot, "testdata", "proto", "proto-enums-addid.formatted-txt"),
		},
	}, {
		name: "yang schema with nested messages requested - uncompressed with fakeroot",
		inFiles: []string{
			filepath.Join(TestRoot, "testdata", "proto", "nested-messages.yang"),
		},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:                     true,
				UseDefiningModuleForTypedefEnumNames: true,
			},
			ProtoOptions: ProtoOpts{
				AnnotateEnumNames:   true,
				AnnotateSchemaPaths: true,
				NestedMessages:      true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig":                 filepath.Join(TestRoot, "testdata", "proto", "nested-messages.openconfig.formatted-txt"),
			"openconfig.enums":           filepath.Join(TestRoot, "testdata", "proto", "nested-messages.enums.formatted-txt"),
			"openconfig.nested_messages": filepath.Join(TestRoot, "testdata", "proto", "nested-messages.nested_messages.formatted-txt"),
		},
	}, {
		name: "yang schema with nested messages - compressed with fakeroot",
		inFiles: []string{
			filepath.Join(TestRoot, "testdata", "proto", "nested-messages.yang"),
		},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				CompressBehaviour:                    genutil.PreferIntendedConfig,
				IgnoreShadowSchemaPaths:              true,
				GenerateFakeRoot:                     true,
				UseDefiningModuleForTypedefEnumNames: true,
			},
			ProtoOptions: ProtoOpts{
				AnnotateEnumNames:   true,
				AnnotateSchemaPaths: true,
				NestedMessages:      true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.enums": filepath.Join(TestRoot, "testdata", "proto", "nested-messages.compressed.enums.formatted-txt"),
			"openconfig":       filepath.Join(TestRoot, "testdata", "proto", "nested-messages.compressed.nested_messages.formatted-txt"),
		},
	}, {
		name: "yang schema with a leafref key to a union with enumeration",
		inFiles: []string{
			filepath.Join(TestRoot, "testdata", "proto", "union-list-key.yang"),
		},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:                     true,
				UseDefiningModuleForTypedefEnumNames: true,
			},
			ProtoOptions: ProtoOpts{
				AnnotateEnumNames:   true,
				AnnotateSchemaPaths: true,
				NestedMessages:      true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.enums":          filepath.Join(TestRoot, "testdata", "proto", "union-list-key.enums.formatted-txt"),
			"openconfig.union_list_key": filepath.Join(TestRoot, "testdata", "proto", "union-list-key.union_list_key.formatted-txt"),
			"openconfig":                filepath.Join(TestRoot, "testdata", "proto", "union-list-key.formatted-txt"),
		},
	}, {
		name: "protobuf generation with excluded read only fields - compressed",
		inFiles: []string{
			filepath.Join(datapath, "openconfig-config-false.yang"),
		},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:  true,
				CompressBehaviour: genutil.UncompressedExcludeDerivedState,
			},
			ProtoOptions: ProtoOpts{
				AnnotateEnumNames:   true,
				AnnotateSchemaPaths: true,
				NestedMessages:      true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig":                         filepath.Join(TestRoot, "testdata", "proto", "excluded-config-false.compressed.formatted-txt"),
			"openconfig.openconfig_config_false": filepath.Join(TestRoot, "testdata", "proto", "excluded-config-false.config_false.compressed.formatted-txt"),
		},
	}, {
		name: "protobuf generation with excluded read only fields - compressed",
		inFiles: []string{
			filepath.Join(datapath, "openconfig-config-false.yang"),
		},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:  true,
				CompressBehaviour: genutil.ExcludeDerivedState,
			},
			ProtoOptions: ProtoOpts{
				AnnotateEnumNames:   true,
				AnnotateSchemaPaths: true,
				NestedMessages:      true,
				GoPackageBase:       "github.com/openconfig/a/package",
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig": filepath.Join(TestRoot, "testdata", "proto", "excluded-config-false.uncompressed.formatted-txt"),
		},
	}, {
		name: "protobuf generation with leafref to a module excluded by the test",
		inFiles: []string{
			filepath.Join(TestRoot, "testdata", "proto", "cross-ref-target.yang"),
			filepath.Join(TestRoot, "testdata", "proto", "cross-ref-src.yang"),
		},
		inConfig: GeneratorConfig{
			ParseOptions: ParseOpts{
				ExcludeModules: []string{"cross-ref-target"},
			},
			ProtoOptions: ProtoOpts{
				NestedMessages: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig.cross_ref_src": filepath.Join(TestRoot, "testdata", "proto", "cross-ref-src.formatted-txt"),
		},
	}, {
		name: "multimod with fakeroot and nested",
		inFiles: []string{
			filepath.Join(TestRoot, "testdata", "proto", "fakeroot-multimod-one.yang"),
			filepath.Join(TestRoot, "testdata", "proto", "fakeroot-multimod-two.yang"),
		},
		inConfig: GeneratorConfig{
			TransformationOptions: TransformationOpts{
				GenerateFakeRoot:  true,
				CompressBehaviour: genutil.PreferIntendedConfig,
			},
			ProtoOptions: ProtoOpts{
				NestedMessages:      true,
				AnnotateEnumNames:   true,
				AnnotateSchemaPaths: true,
			},
		},
		wantOutputFiles: map[string]string{
			"openconfig": filepath.Join(TestRoot, "testdata", "proto", "fakeroot-multimod.formatted-txt"),
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			sortedPkgNames := func(pkgs map[string]string) []string {
				wantPkgs := []string{}
				for k := range tt.wantOutputFiles {
					wantPkgs = append(wantPkgs, k)
				}
				sort.Strings(wantPkgs)
				return wantPkgs
			}

			genCode := func() *GeneratedProto3 {
				if tt.inConfig.Caller == "" {
					// Override the caller if it is not set, to ensure that test
					// output is deterministic.
					tt.inConfig.Caller = "codegen-tests"
				}

				cg := NewYANGCodeGenerator(&tt.inConfig)
				gotProto, err := cg.GenerateProto3(tt.inFiles, tt.inIncludePaths)
				if (err != nil) != tt.wantErr {
					t.Fatalf("cg.GenerateProto3(%v, %v), config: %v: got unexpected error: %v", tt.inFiles, tt.inIncludePaths, tt.inConfig, err)
				}

				if tt.wantErr || err != nil {
					return nil
				}

				return gotProto
			}

			gotProto := genCode()

			allCode := bytes.Buffer{}

			seenPkg := map[string]bool{}
			for n := range gotProto.Packages {
				seenPkg[n] = false
			}

			protoPkgs := func(m map[string]Proto3Package) []string {
				a := []string{}
				for k := range m {
					a = append(a, k)
				}
				return a
			}

			wantPkgs := sortedPkgNames(tt.wantOutputFiles)
			for _, pkg := range wantPkgs {
				wantFile := tt.wantOutputFiles[pkg]
				wantCodeBytes, err := ioutil.ReadFile(wantFile)
				if err != nil {
					t.Errorf("%s: ioutil.ReadFile(%v): could not read file for package %s", tt.name, wantFile, pkg)
					return
				}

				gotPkg, ok := gotProto.Packages[pkg]
				if !ok {
					t.Fatalf("%s: cg.GenerateProto3(%v, %v): did not find expected package %s in output, got: %#v, want key: %v", tt.name, tt.inFiles, tt.inIncludePaths, pkg, protoPkgs(gotProto.Packages), pkg)
				}

				// Mark this package as having been seen.
				seenPkg[pkg] = true

				// Write the returned struct out to a buffer to compare with the
				// testdata file.
				var gotCodeBuf bytes.Buffer
				fmt.Fprintf(&gotCodeBuf, gotPkg.Header)

				for _, gotMsg := range gotPkg.Messages {
					fmt.Fprintf(&gotCodeBuf, "%s\n", gotMsg)
				}

				for _, gotEnum := range gotPkg.Enums {
					fmt.Fprintf(&gotCodeBuf, "%s", gotEnum)
				}

				wantCode := string(wantCodeBytes)

				allCode.WriteString(gotCodeBuf.String())

				if diff := pretty.Compare(gotCodeBuf.String(), wantCode); diff != "" {
					if diffl, _ := testutil.GenerateUnifiedDiff(wantCode, gotCodeBuf.String()); diffl != "" {
						diff = diffl
					}
					t.Errorf("%s: cg.GenerateProto3(%v, %v) for package %s, did not get expected code (code file: %v), diff(-want, +got):\n%s", tt.name, tt.inFiles, tt.inIncludePaths, pkg, wantFile, diff)
				}
			}

			for pkg, seen := range seenPkg {
				if !seen {
					t.Errorf("%s: cg.GenerateProto3(%v, %v) did not test received package %v", tt.name, tt.inFiles, tt.inIncludePaths, pkg)
				}
			}

			for i := 0; i < deflakeRuns; i++ {
				got := genCode()
				var gotCodeBuf bytes.Buffer

				wantPkgs := sortedPkgNames(tt.wantOutputFiles)
				for _, pkg := range wantPkgs {
					gotPkg, ok := got.Packages[pkg]
					if !ok {
						t.Fatalf("%s: cg.GenerateProto3(%v, %v): did not find expected package %s in output, got: %#v, want key: %v", tt.name, tt.inFiles, tt.inIncludePaths, pkg, protoPkgs(gotProto.Packages), pkg)
					}
					fmt.Fprintf(&gotCodeBuf, gotPkg.Header)
					for _, gotMsg := range gotPkg.Messages {
						fmt.Fprintf(&gotCodeBuf, "%s\n", gotMsg)
					}
					for _, gotEnum := range gotPkg.Enums {
						fmt.Fprintf(&gotCodeBuf, "%s", gotEnum)
					}
				}

				if diff := pretty.Compare(gotCodeBuf.String(), allCode.String()); diff != "" {
					diff, _ = testutil.GenerateUnifiedDiff(allCode.String(), gotCodeBuf.String())
					t.Fatalf("flaky code generation iter: %d, diff(-want, +got):\n%s", i, diff)
				}
			}
		})
	}
}

func TestMakeFakeRoot(t *testing.T) {
	tests := []struct {
		name       string
		inRootName string
		want       *yang.Entry
	}{{
		name:       "simple empty root named device",
		inRootName: "device",
		want: &yang.Entry{
			Name: "device",
			Kind: yang.DirectoryEntry,
			Dir:  map[string]*yang.Entry{},
			Node: &yang.Value{
				Name: rootElementNodeName,
			},
		},
	}, {
		name:       "simple root named !@#$",
		inRootName: "!@#$",
		want: &yang.Entry{
			Name: "!@#$",
			Kind: yang.DirectoryEntry,
			Dir:  map[string]*yang.Entry{},
			Node: &yang.Value{
				Name: rootElementNodeName,
			},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeFakeRoot(tt.inRootName)
			if diff := pretty.Compare(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
			if !IsFakeRoot(got) {
				t.Errorf("IsFakeRoot returned false for entry %v", got)
			}
		})
	}
}

func TestCreateFakeRoot(t *testing.T) {
	tests := []struct {
		name            string
		inStructs       map[string]*yang.Entry
		inRootElems     []*yang.Entry
		inRootName      string
		inCompressPaths bool
		wantRoot        *yang.Entry
		wantErr         bool
	}{{
		name: "simple root",
		inStructs: map[string]*yang.Entry{
			"/module/foo": {
				Name: "foo",
				Kind: yang.DirectoryEntry,
				Parent: &yang.Entry{
					Name: "module",
				},
			},
		},
		inRootElems: []*yang.Entry{{
			Name: "foo",
			Kind: yang.DirectoryEntry,
			Parent: &yang.Entry{
				Name: "module",
			},
		}, {
			Name: "bar",
			Parent: &yang.Entry{
				Name: "module",
			},
			Type: &yang.YangType{Kind: yang.Ystring},
		}},
		inRootName:      "",
		inCompressPaths: false,
		wantRoot: &yang.Entry{
			Name: defaultRootName,
			Kind: yang.DirectoryEntry,
			Dir: map[string]*yang.Entry{
				"foo": {
					Name: "foo",
					Kind: yang.DirectoryEntry,
					Parent: &yang.Entry{
						Name: "module",
					},
				},
				"bar": {
					Name: "bar",
					Parent: &yang.Entry{
						Name: "module",
					},
					Type: &yang.YangType{Kind: yang.Ystring},
				},
			},
			Node: &yang.Value{
				Name: rootElementNodeName,
			},
		},
	}, {
		name: "overlapping root entries",
		inStructs: map[string]*yang.Entry{
			"/module1/foo": {
				Name: "foo",
				Kind: yang.DirectoryEntry,
				Parent: &yang.Entry{
					Name: "module1",
				},
			},
			"/module2/foo": {
				Name: "foo",
				Kind: yang.DirectoryEntry,
				Parent: &yang.Entry{
					Name: "module2",
				},
			},
		},
		inRootName: "name",
		wantErr:    true,
	}}

	for _, tt := range tests {
		err := createFakeRoot(tt.inStructs, tt.inRootElems, tt.inRootName, tt.inCompressPaths)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: createFakeRoot(%v, %v, %s, %v): did not get expected error, got: %s, wantErr: %v", tt.name, tt.inStructs, tt.inRootElems, tt.inRootName, tt.inCompressPaths, err, tt.wantErr)
			continue
		}

		if err != nil {
			continue
		}

		if diff := pretty.Compare(tt.inStructs["/"], tt.wantRoot); diff != "" {
			t.Errorf("%s: createFakeRoot(%v, %v, %s, %v): did not get expected root struct, diff(-got,+want):\n%s", tt.name, tt.inStructs, tt.inRootElems, tt.inRootName, tt.inCompressPaths, diff)
		}

		if !IsFakeRoot(tt.inStructs["/"]) {
			t.Errorf("IsFakeRoot returned false for entry %v", tt.inStructs["/"])
		}
	}
}

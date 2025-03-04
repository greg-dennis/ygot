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
	"fmt"
	"sort"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/openconfig/ygot/genutil"
	"github.com/openconfig/ygot/util"
)

// ProtoLangMapper contains the functionality and state for generating proto
// names for the generated code.
type ProtoLangMapper struct {
	// enumSet contains the generated enum names which can be queried.
	enumSet *enumSet
	// schematree is a copy of the YANG schema tree, containing only leaf
	// entries, such that schema paths can be referenced.
	schematree *schemaTree
	// definedGlobals specifies the global proto names used during code
	// generation to avoid conflicts.
	definedGlobals map[string]bool
	// uniqueDirectoryNames is a map keyed by the path of a YANG entity representing a
	// directory in the generated code whose value is the unique name that it
	// was mapped to. This allows routines to determine, based on a particular YANG
	// entry, how to refer to it when generating code.
	uniqueDirectoryNames map[string]string
	// uniqueProtoMsgNames is a map, keyed by a protobuf package name, that
	// contains a map keyed by protobuf message name strings that indicates the
	// names that are used within the generated package's context. It is used
	// during code generation to ensure uniqueness of the generated names within
	// the specified package.
	uniqueProtoMsgNames map[string]map[string]bool
	// uniqueProtoPackages is a map, keyed by a YANG schema path, that allows
	// a path to be resolved into the calculated Protobuf package name that
	// is to be used for it.
	uniqueProtoPackages map[string]string

	// basePackageName is the name of the package within which all generated packages
	// are to be generated.
	basePackageName string
	// enumPackageName is the name of the package within which global enumerated values
	// are defined (i.e., typedefs that contain enumerations, or YANG identities).
	enumPackageName string
}

// NewProtoLangMapper creates a new ProtoLangMapper instance, initialised with the
// default state required for code generation.
func NewProtoLangMapper(basePackageName, enumPackageName string) *ProtoLangMapper {
	return &ProtoLangMapper{
		definedGlobals:       map[string]bool{},
		uniqueDirectoryNames: map[string]string{},
		uniqueProtoMsgNames:  map[string]map[string]bool{},
		uniqueProtoPackages:  map[string]string{},
		basePackageName:      basePackageName,
		enumPackageName:      enumPackageName,
	}
}

// DirectoryName generates the proto message name to be used for a particular
// YANG schema element in the generated code.
// Since this conversion is lossy, a later step should resolve any naming
// conflicts between different fields.
func (s *ProtoLangMapper) DirectoryName(e *yang.Entry, cb genutil.CompressBehaviour) (string, error) {
	return s.protoMsgName(e, cb.CompressEnabled()), nil
}

// FieldName maps the input entry's name to what the proto name of the field would be.
// Since this conversion is lossy, a later step should resolve any naming
// conflicts between different fields.
func (s *ProtoLangMapper) FieldName(e *yang.Entry) (string, error) {
	return safeProtoIdentifierName(e.Name), nil
}

// LeafType maps the input leaf entry to a MappedType object containing the
// type information about the field.
func (s *ProtoLangMapper) LeafType(e *yang.Entry, opts IROptions) (*MappedType, error) {
	protoType, err := s.yangTypeToProtoType(resolveTypeArgs{
		yangType:     e.Type,
		contextEntry: e,
	}, resolveProtoTypeArgs{
		basePackageName: s.basePackageName,
		enumPackageName: s.enumPackageName,
	}, opts)
	if err != nil {
		return nil, err
	}

	return protoType, err
}

// LeafType maps the input list key entry to a MappedType object containing the
// type information about the key field.
func (s *ProtoLangMapper) KeyLeafType(e *yang.Entry, opts IROptions) (*MappedType, error) {
	scalarType, err := s.yangTypeToProtoScalarType(resolveTypeArgs{
		yangType:     e.Type,
		contextEntry: e,
	}, resolveProtoTypeArgs{
		basePackageName: s.basePackageName,
		enumPackageName: s.enumPackageName,
		// When there is a union within a list key that has a single type within it
		// e.g.,:
		// list foo {
		//   key "bar";
		//   leaf bar {
		//     type union {
		//       type string { pattern "a.*" }
		//			 type string { pattern "b.*" }
		//     }
		//   }
		// }
		// Then we want to use the scalar type rather than the wrapper type in
		// this message since all keys must be set. We therefore signal this in
		// the call to the type resolution.
		scalarTypeInSingleTypeUnion: true,
	}, opts)
	if err != nil {
		return nil, fmt.Errorf("list %s included a key %s that did not have a valid proto type: %v", e.Path(), e.Name, e.Type)
	}

	return scalarType, nil
}

// PackageName determines the package that the particular output protobuf
// should reside in. In the case that nested messages are being output, the
// package name is derived based on the top-level module that the message is
// within.
func (s *ProtoLangMapper) PackageName(e *yang.Entry, compressBehaviour genutil.CompressBehaviour, nestedMessages bool) (string, error) {
	compressPaths := compressBehaviour.CompressEnabled()
	switch {
	case IsFakeRoot(e):
		// In this case, we explicitly leave the package name as nil, which is interpeted
		// as meaning that the base package is used throughout the handling code.
		return "", nil
	case e.Parent == nil:
		return "", fmt.Errorf("YANG schema element %s does not have a parent, protobuf messages are not generated for modules", e.Path())
	}

	// If we have nested messages enabled, the protobuf package name is defined
	// based on the top-level message within the schema tree that is created -
	// we therefore need to derive the name of this message.
	if nestedMessages {
		if compressPaths {
			if e.Parent.Parent == nil {
				// In the special case that the grandparent of this entry is nil, and
				// compress paths is enabled, then we are a top-level schema element - so
				// this message should be in the root package.
				return "", nil
			}
			if e.IsList() && e.Parent.Parent.Parent == nil {
				// If this is a list, and our great-grandparent is a module, then
				// since the level above this node has been compressed out, then it
				// is at the root.
				return "", nil
			}
		}

		if e.Parent != nil && e.Parent.Parent != nil {
			var n *yang.Entry
			for n = e.Parent; n.Parent.Parent != nil; n = n.Parent {
			}
			e = n
		}
	}

	return s.protobufPackage(e, compressPaths), nil
}

// SetEnumSet is used to supply a set of enumerated values to the
// mapper such that leaves that have enumerated types can be looked up.
func (s *ProtoLangMapper) SetEnumSet(e *enumSet) {
	s.enumSet = e
}

// SetSchemaTree is used to supply a copy of the YANG schema tree to
// the mapped such that leaves of type leafref can be resolved to
// their target leaves.
func (s *ProtoLangMapper) SetSchemaTree(st *schemaTree) {
	s.schematree = st
}

// resolveProtoTypeArgs specifies input parameters required for resolving types
// from YANG to protobuf.
// TODO(robjs): Consider embedding resolveProtoTypeArgs in this struct per
// discussion in https://github.com/openconfig/ygot/pull/57.
type resolveProtoTypeArgs struct {
	// basePackageNAme is the name of the package within which all generated packages
	// are to be generated.
	basePackageName string
	// enumPackageName is the name of the package within which global enumerated values
	// are defined (i.e., typedefs that contain enumerations, or YANG identities).
	enumPackageName string
	// scalaraTypeInSingleTypeUnion specifies whether scalar types should be used
	// when a union contains only one base type, or whether the protobuf wrapper
	// types should be used.
	scalarTypeInSingleTypeUnion bool
}

// yangEnumTypeToProtoType takes an input resolveTypeArgs (containing a Yenum
// yang.YangType and a context node) and returns the protobuf type that it is
// to be represented by. The types that are used in the protobuf are wrapper
// types as described in the YANG to Protobuf translation specification.
// If the input type is not a Yenum, an error is returned.
func yangEnumTypeToProtoType(args resolveTypeArgs) (*MappedType, error) {
	if args.yangType.Kind != yang.Yenum {
		return nil, fmt.Errorf("input type to yangEnumTypeToProtoType is not a Yenum: %s", args.contextEntry.Path())
	}
	// Return any enumeration simply as the leaf's CamelCase name
	// since it will be mapped to the correct name at output file to ensure
	// that there are no collisions. Enumerations are mapped to an embedded
	// enum within the message.
	if args.contextEntry == nil {
		return nil, fmt.Errorf("cannot map enumeration without context entry: %v", args)
	}
	// However, if the enumeration is inlined within a union, then
	// we add a suffix to indicate that it is part of a larger
	// union type.
	typeName := yang.CamelCase(args.contextEntry.Name)
	definingType, err := util.DefiningType(args.yangType, args.contextEntry.Type)
	if err != nil {
		return nil, err
	}
	if definingType.Kind == yang.Yunion {
		typeName += enumeratedUnionSuffix
	}
	return &MappedType{
		NativeType:        typeName,
		IsEnumeratedValue: true,
	}, nil
}

// yangTypeToProtoType takes an input resolveTypeArgs (containing a yang.YangType
// and a context node) and returns the protobuf type that it is to be represented
// by. The types that are used in the protobuf are wrapper types as described
// in the YANG to Protobuf translation specification.
//
// The type returned is a wrapper protobuf such that in proto3 an unset field
// can be distinguished from one set to the nil value.
//
// See https://github.com/openconfig/ygot/blob/master/docs/yang-to-protobuf-transformations-spec.md
// for additional details as to the transformation from YANG to Protobuf.
func (s *ProtoLangMapper) yangTypeToProtoType(args resolveTypeArgs, pargs resolveProtoTypeArgs, opts IROptions) (*MappedType, error) {
	// Handle typedef cases.
	mtype, err := s.enumSet.enumeratedTypedefTypeName(args, fmt.Sprintf("%s.%s.", pargs.basePackageName, pargs.enumPackageName), true, true)
	if err != nil {
		return nil, err
	}
	if mtype != nil {
		// mtype is set to non-nil when this was a valid enumeration
		// within a typedef.
		return mtype, nil
	}

	switch args.yangType.Kind {
	case yang.Yint8, yang.Yint16, yang.Yint32, yang.Yint64:
		return &MappedType{NativeType: ywrapperAccessor + "IntValue"}, nil
	case yang.Yuint8, yang.Yuint16, yang.Yuint32, yang.Yuint64:
		return &MappedType{NativeType: ywrapperAccessor + "UintValue"}, nil
	case yang.Ybinary:
		return &MappedType{NativeType: ywrapperAccessor + "BytesValue"}, nil
	case yang.Ybool, yang.Yempty:
		return &MappedType{NativeType: ywrapperAccessor + "BoolValue"}, nil
	case yang.Ystring:
		return &MappedType{NativeType: ywrapperAccessor + "StringValue"}, nil
	case yang.Ydecimal64:
		return &MappedType{NativeType: ywrapperAccessor + "Decimal64Value"}, nil
	case yang.Yleafref:
		// We look up the leafref in the schema tree to be able to
		// determine what type to map to.
		target, err := s.schematree.resolveLeafrefTarget(args.yangType.Path, args.contextEntry)
		if err != nil {
			return nil, err
		}
		return s.yangTypeToProtoType(resolveTypeArgs{yangType: target.Type, contextEntry: target}, pargs, opts)
	case yang.Yenum:
		mtype, err := yangEnumTypeToProtoType(args)
		if err != nil {
			return nil, err
		}
		_, key, err := s.enumSet.enumName(args.contextEntry, opts.TransformationOptions.CompressBehaviour.CompressEnabled(), !opts.TransformationOptions.EnumerationsUseUnderscores, opts.ParseOptions.SkipEnumDeduplication, opts.TransformationOptions.ShortenEnumLeafNames, false, opts.TransformationOptions.EnumOrgPrefixesToTrim)
		if err != nil {
			return nil, err
		}
		mtype.EnumeratedYANGTypeKey = key
		return mtype, nil
	case yang.Yidentityref:
		// TODO(https://github.com/openconfig/ygot/issues/33) - refactor to allow
		// this call outside of the switch.
		if args.contextEntry == nil {
			return nil, fmt.Errorf("cannot map identityref without context entry: %v", args)
		}
		n, key, err := s.protoIdentityName(pargs, args.contextEntry.Type.IdentityBase)
		if err != nil {
			return nil, err
		}
		return &MappedType{
			NativeType:            n,
			IsEnumeratedValue:     true,
			EnumeratedYANGTypeKey: key,
		}, nil
	case yang.Yunion:
		return s.protoUnionType(args, pargs, opts)
	default:
		// TODO(robjs): Implement types that are missing within this function.
		// Missing types are:
		//  - binary
		//  - bits
		// We cannot return an interface{} in protobuf, so therefore
		// we just throw an error with types that we cannot map.
		return nil, fmt.Errorf("unimplemented type: %v", args.yangType.Kind)
	}
}

// yangTypeToProtoScalarType takes an input resolveTypeArgs and returns the protobuf
// in-built type that is used to represent it. It is used within list keys where the
// value cannot be nil/unset.
func (s *ProtoLangMapper) yangTypeToProtoScalarType(args resolveTypeArgs, pargs resolveProtoTypeArgs, opts IROptions) (*MappedType, error) {
	// Handle typedef cases.
	mtype, err := s.enumSet.enumeratedTypedefTypeName(args, fmt.Sprintf("%s.%s.", pargs.basePackageName, pargs.enumPackageName), true, true)
	if err != nil {
		return nil, err
	}
	if mtype != nil {
		// mtype is set to non-nil when this was a valid enumeration
		// within a typedef.
		return mtype, nil
	}
	switch args.yangType.Kind {
	case yang.Yint8, yang.Yint16, yang.Yint32, yang.Yint64:
		return &MappedType{NativeType: "sint64"}, nil
	case yang.Yuint8, yang.Yuint16, yang.Yuint32, yang.Yuint64:
		return &MappedType{NativeType: "uint64"}, nil
	case yang.Ybinary:
		return &MappedType{NativeType: "bytes"}, nil
	case yang.Ybool, yang.Yempty:
		return &MappedType{NativeType: "bool"}, nil
	case yang.Ystring:
		return &MappedType{NativeType: "string"}, nil
	case yang.Ydecimal64:
		// Decimal64 continues to be a message even when we are mapping scalars
		// as there is not an equivalent Protobuf type.
		return &MappedType{NativeType: ywrapperAccessor + "Decimal64Value"}, nil
	case yang.Yleafref:
		target, err := s.schematree.resolveLeafrefTarget(args.yangType.Path, args.contextEntry)
		if err != nil {
			return nil, err
		}
		return s.yangTypeToProtoScalarType(resolveTypeArgs{yangType: target.Type, contextEntry: target}, pargs, opts)
	case yang.Yenum:
		mtype, err := yangEnumTypeToProtoType(args)
		if err != nil {
			return nil, err
		}
		_, key, err := s.enumSet.enumName(args.contextEntry, opts.TransformationOptions.CompressBehaviour.CompressEnabled(), !opts.TransformationOptions.EnumerationsUseUnderscores, opts.ParseOptions.SkipEnumDeduplication, opts.TransformationOptions.ShortenEnumLeafNames, false, opts.TransformationOptions.EnumOrgPrefixesToTrim)
		if err != nil {
			return nil, err
		}
		mtype.EnumeratedYANGTypeKey = key
		return mtype, nil
	case yang.Yidentityref:
		if args.contextEntry == nil {
			return nil, fmt.Errorf("cannot map identityref without context entry: %v", args)
		}
		n, key, err := s.protoIdentityName(pargs, args.contextEntry.Type.IdentityBase)
		if err != nil {
			return nil, err
		}
		return &MappedType{
			NativeType:            n,
			IsEnumeratedValue:     true,
			EnumeratedYANGTypeKey: key,
		}, nil
	case yang.Yunion:
		return s.protoUnionType(args, pargs, opts)
	default:
		// TODO(robjs): implement missing types.
		//	- binary
		//	- bits
		return nil, fmt.Errorf("unimplemented type in scalar generation: %s", args.yangType.Kind)
	}
}

type unionSubtypeInfo struct {
	yangType *yang.YangType
	mtype    *MappedType
}

// protoUnionType resolves the types that are included within the YangType in resolveTypeArgs into the
// scalar type that can be included in a protobuf oneof. The basePackageName and enumPackageName are used
// to determine the paths that are used for enumerated types within the YANG schema. Each union is
// resolved into a oneof that contains the scalar types, for example:
//
// leaf a {
//	type union {
//		type string;
//		type int32;
//	}
// }
//
// Is represented in the output protobuf as:
//
// oneof a {
//	string a_string = NN;
//	int32 a_int32 = NN;
// }
//
// The MappedType's UnionTypes can be output through a template into the oneof.
func (s *ProtoLangMapper) protoUnionType(args resolveTypeArgs, pargs resolveProtoTypeArgs, opts IROptions) (*MappedType, error) {
	unionTypes := make(map[string]unionSubtypeInfo)
	if errs := s.protoUnionSubTypes(args.yangType, args.contextEntry, unionTypes, pargs, opts); errs != nil {
		return nil, fmt.Errorf("errors mapping element: %v", errs)
	}

	// Handle the case that there is just one protobuf type within the union.
	if len(unionTypes) == 1 {
		for st, t := range unionTypes {
			// Handle the case whereby there is an identityref and we simply
			// want to return the type that has been resolved.
			if t.yangType.Kind == yang.Yidentityref || t.yangType.Kind == yang.Yenum {
				return &MappedType{
					NativeType:            st,
					IsEnumeratedValue:     true,
					EnumeratedYANGTypeKey: t.mtype.EnumeratedYANGTypeKey,
				}, nil
			}

			var n *MappedType
			var err error
			// Resolve the type of the single type within the union according to whether
			// we want scalar types or not. This is used in contexts where there may
			// be a union that is within a key message, which never uses wrapper types
			// since the keys of a list must all be set.
			if pargs.scalarTypeInSingleTypeUnion {
				n, err = s.yangTypeToProtoScalarType(resolveTypeArgs{
					yangType:     t.yangType,
					contextEntry: args.contextEntry,
				}, pargs, opts)
			} else {
				n, err = s.yangTypeToProtoType(resolveTypeArgs{
					yangType:     t.yangType,
					contextEntry: args.contextEntry,
				}, pargs, opts)
			}

			if err != nil {
				return nil, fmt.Errorf("error mapping single type within a union: %v", err)
			}
			return n, nil
		}
	}

	mtype := &MappedType{
		UnionTypes:     map[string]int{},
		UnionTypeInfos: map[string]MappedUnionSubtype{},
	}

	// Rewrite the map to be the expected format for the MappedType return value,
	// we sort the keys into alphabetical order to avoid test flakes.
	keys := []string{}
	for k, t := range unionTypes {
		keys = append(keys, k)
		mtype.UnionTypeInfos[k] = MappedUnionSubtype{
			EnumeratedYANGTypeKey: t.mtype.EnumeratedYANGTypeKey,
		}
	}

	sort.Strings(keys)
	for _, k := range keys {
		mtype.UnionTypes[k] = len(mtype.UnionTypes)
	}

	return mtype, nil
}

// protoUnionSubTypes extracts all possible subtypes of a YANG union. It returns a map keyed by the mapped type
// along with any errors that occur. The context entry is used to map types when the leaf that the type is associated
// with is required for mapping. The currentType map is updated as an in-out argument. The basePackageName and enumPackageName
// are used to map enumerated typedefs and identityrefs to the correct type. It returns a slice of errors if they occur
// mapping subtypes.
func (s *ProtoLangMapper) protoUnionSubTypes(subtype *yang.YangType, ctx *yang.Entry, currentTypes map[string]unionSubtypeInfo, pargs resolveProtoTypeArgs, opts IROptions) []error {
	var errs []error
	if util.IsUnionType(subtype) {
		for _, st := range subtype.Type {
			errs = append(errs, s.protoUnionSubTypes(st, ctx, currentTypes, pargs, opts)...)
		}
		return errs
	}

	var mtype *MappedType
	switch subtype.Kind {
	case yang.Yidentityref:
		n, key, err := s.protoIdentityName(pargs, subtype.IdentityBase)
		if err != nil {
			return append(errs, err)
		}
		// Handle the case that the context entry is not the correct entry to deal with. This occurs when the subtype is
		// an identityref.
		mtype = &MappedType{
			NativeType:            n,
			IsEnumeratedValue:     true,
			EnumeratedYANGTypeKey: key,
		}
	default:
		var err error
		mtype, err = s.yangTypeToProtoScalarType(resolveTypeArgs{yangType: subtype, contextEntry: ctx}, pargs, opts)
		if err != nil {
			return append(errs, err)
		}
	}

	// Only append the type if it not one that is currently in the list. The proto oneof only has the
	// base type that is included.
	if _, ok := currentTypes[mtype.NativeType]; !ok {
		currentTypes[mtype.NativeType] = unionSubtypeInfo{yangType: subtype, mtype: mtype}
	}

	return errs
}

// protoMsgName takes a yang.Entry and converts it to its protobuf message name,
// ensuring that the name that is returned is unique within the package that it is
// being contained within.
func (s *ProtoLangMapper) protoMsgName(e *yang.Entry, compressPaths bool) string {
	// Return a cached name if one has already been computed.
	if n, ok := s.uniqueDirectoryNames[e.Path()]; ok {
		return n
	}

	pkg := s.protobufPackage(e, compressPaths)
	if _, ok := s.uniqueProtoMsgNames[pkg]; !ok {
		s.uniqueProtoMsgNames[pkg] = make(map[string]bool)
	}

	n := genutil.MakeNameUnique(yang.CamelCase(e.Name), s.uniqueProtoMsgNames[pkg])
	s.uniqueProtoMsgNames[pkg][n] = true

	// Record that this was the proto message name that was used.
	s.uniqueDirectoryNames[e.Path()] = n

	return n
}

// protobufPackage generates a protobuf package name for a yang.Entry by taking its
// parent's path and converting it to a protobuf-style name. i.e., an entry with
// the path /openconfig-interfaces/interfaces/interface/config/name returns
// openconfig_interfaces.interfaces.interface.config. If path compression is
// enabled then entities that would not have messages generated from them
// are omitted from the path, i.e., /openconfig-interfaces/interfaces/interface/config/name
// becomes interface (since modules, surrounding containers, and config/state containers
// are not considered with path compression enabled.
func (s *ProtoLangMapper) protobufPackage(e *yang.Entry, compressPaths bool) string {
	if IsFakeRoot(e) {
		return ""
	}

	parent := e.Parent
	// In the case of path compression, then the parent of a list is the parent
	// one level up, as is the case for if there are config and state containers.
	if compressPaths && e.IsList() || compressPaths && util.IsConfigState(e) {
		parent = e.Parent.Parent
	}

	// If this entry has already had its parent's package calculated for it, then
	// simply return the already calculated name.
	if pkg, ok := s.uniqueProtoPackages[parent.Path()]; ok {
		return pkg
	}

	parts := []string{}
	for p := parent; p != nil; p = p.Parent {
		if compressPaths && !util.IsOCCompressedValidElement(p) || !compressPaths && util.IsChoiceOrCase(p) {
			// If compress paths is enabled, and this entity would not
			// have been included in the generated protobuf output, therefore
			// we also exclude it from the package name.
			continue
		}
		parts = append(parts, safeProtoIdentifierName(p.Name))
	}

	// Reverse the slice since we traversed from leaf back to root.
	for i := len(parts)/2 - 1; i >= 0; i-- {
		parts[i], parts[len(parts)-1-i] = parts[len(parts)-1-i], parts[i]
	}

	// Make the name unique since foo.bar.baz-bat and foo.bar.baz_bat will
	// become the same name in the safeProtoIdentifierName transformation above.
	n := genutil.MakeNameUnique(strings.Join(parts, "."), s.definedGlobals)
	s.definedGlobals[n] = true

	// Record the mapping between this entry's parent and the defined
	// package name that was used.
	s.uniqueProtoPackages[parent.Path()] = n

	return n
}

// protoIdentityName returns the name that should be used for an identityref base.
func (s *ProtoLangMapper) protoIdentityName(pargs resolveProtoTypeArgs, i *yang.Identity) (string, string, error) {
	n, key, err := s.enumSet.identityrefBaseTypeFromIdentity(i)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%s.%s.%s", pargs.basePackageName, pargs.enumPackageName, n), key, nil
}

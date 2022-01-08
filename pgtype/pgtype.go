package pgtype

import (
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"reflect"
	"time"
)

// PostgreSQL oids for common types
const (
	BoolOID             = 16
	ByteaOID            = 17
	QCharOID            = 18
	NameOID             = 19
	Int8OID             = 20
	Int2OID             = 21
	Int4OID             = 23
	TextOID             = 25
	OIDOID              = 26
	TIDOID              = 27
	XIDOID              = 28
	CIDOID              = 29
	JSONOID             = 114
	PointOID            = 600
	LsegOID             = 601
	PathOID             = 602
	BoxOID              = 603
	PolygonOID          = 604
	LineOID             = 628
	CIDROID             = 650
	CIDRArrayOID        = 651
	Float4OID           = 700
	Float8OID           = 701
	CircleOID           = 718
	CircleArrayOID      = 719
	UnknownOID          = 705
	MacaddrOID          = 829
	InetOID             = 869
	BoolArrayOID        = 1000
	NameArrayOID        = 1003
	Int2ArrayOID        = 1005
	Int4ArrayOID        = 1007
	TextArrayOID        = 1009
	ByteaArrayOID       = 1001
	BPCharArrayOID      = 1014
	VarcharArrayOID     = 1015
	Int8ArrayOID        = 1016
	PointArrayOID       = 1017
	BoxArrayOID         = 1020
	Float4ArrayOID      = 1021
	Float8ArrayOID      = 1022
	ACLItemOID          = 1033
	ACLItemArrayOID     = 1034
	InetArrayOID        = 1041
	BPCharOID           = 1042
	VarcharOID          = 1043
	DateOID             = 1082
	TimeOID             = 1083
	TimestampOID        = 1114
	TimestampArrayOID   = 1115
	DateArrayOID        = 1182
	TimestamptzOID      = 1184
	TimestamptzArrayOID = 1185
	IntervalOID         = 1186
	NumericArrayOID     = 1231
	BitOID              = 1560
	VarbitOID           = 1562
	NumericOID          = 1700
	RecordOID           = 2249
	UUIDOID             = 2950
	UUIDArrayOID        = 2951
	JSONBOID            = 3802
	JSONBArrayOID       = 3807
	DaterangeOID        = 3912
	Int4rangeOID        = 3904
	NumrangeOID         = 3906
	TsrangeOID          = 3908
	TsrangeArrayOID     = 3909
	TstzrangeOID        = 3910
	TstzrangeArrayOID   = 3911
	Int8rangeOID        = 3926
)

type InfinityModifier int8

const (
	Infinity         InfinityModifier = 1
	None             InfinityModifier = 0
	NegativeInfinity InfinityModifier = -Infinity
)

func (im InfinityModifier) String() string {
	switch im {
	case None:
		return "none"
	case Infinity:
		return "infinity"
	case NegativeInfinity:
		return "-infinity"
	default:
		return "invalid"
	}
}

// PostgreSQL format codes
const (
	TextFormatCode   = 0
	BinaryFormatCode = 1
)

// Value translates values to and from an internal canonical representation for the type. To actually be usable a type
// that implements Value should also implement some combination of BinaryDecoder, BinaryEncoder, TextDecoder,
// and TextEncoder.
//
// Operations that update a Value (e.g. Set, DecodeText, DecodeBinary) should entirely replace the value. e.g. Internal
// slices should be replaced not resized and reused. This allows Get and AssignTo to return a slice directly rather
// than incur a usually unnecessary copy.
type Value interface {
	// Set converts and assigns src to itself. Value takes ownership of src.
	Set(src interface{}) error

	// Get returns the simplest representation of Value. Get may return a pointer to an internal value but it must never
	// mutate that value. e.g. If Get returns a []byte Value must never change the contents of the []byte.
	Get() interface{}

	// AssignTo converts and assigns the Value to dst. AssignTo may a pointer to an internal value but it must never
	// mutate that value. e.g. If Get returns a []byte Value must never change the contents of the []byte.
	AssignTo(dst interface{}) error
}

// TypeValue is a Value where instances can represent different PostgreSQL types. This can be useful for
// representing types such as enums, composites, and arrays.
//
// In general, instances of TypeValue should not be used to directly represent a value. It should only be used as an
// encoder and decoder internal to ConnInfo.
type TypeValue interface {
	Value

	// NewTypeValue creates a TypeValue including references to internal type information. e.g. the list of members
	// in an EnumType.
	NewTypeValue() Value

	// TypeName returns the PostgreSQL name of this type.
	TypeName() string
}

type Codec interface {
	// FormatSupported returns true if the format is supported.
	FormatSupported(int16) bool

	// PreferredFormat returns the preferred format.
	PreferredFormat() int16

	// PlanEncode returns an Encode plan for encoding value into PostgreSQL format for oid and format. If no plan can be
	// found then nil is returned.
	PlanEncode(ci *ConnInfo, oid uint32, format int16, value interface{}) EncodePlan

	// PlanScan returns a ScanPlan for scanning a PostgreSQL value into a destination with the same type as target. If
	// actualTarget is true then the returned ScanPlan may be optimized to directly scan into target. If no plan can be
	// found then nil is returned.
	PlanScan(ci *ConnInfo, oid uint32, format int16, target interface{}, actualTarget bool) ScanPlan

	// DecodeDatabaseSQLValue returns src decoded into a value compatible with the sql.Scanner interface.
	DecodeDatabaseSQLValue(ci *ConnInfo, oid uint32, format int16, src []byte) (driver.Value, error)

	// DecodeValue returns src decoded into its default format.
	DecodeValue(ci *ConnInfo, oid uint32, format int16, src []byte) (interface{}, error)
}

type BinaryDecoder interface {
	// DecodeBinary decodes src into BinaryDecoder. If src is nil then the
	// original SQL value is NULL. BinaryDecoder takes ownership of src. The
	// caller MUST not use it again.
	DecodeBinary(ci *ConnInfo, src []byte) error
}

type TextDecoder interface {
	// DecodeText decodes src into TextDecoder. If src is nil then the original
	// SQL value is NULL. TextDecoder takes ownership of src. The caller MUST not
	// use it again.
	DecodeText(ci *ConnInfo, src []byte) error
}

// BinaryEncoder is implemented by types that can encode themselves into the
// PostgreSQL binary wire format.
type BinaryEncoder interface {
	// EncodeBinary should append the binary format of self to buf. If self is the
	// SQL value NULL then append nothing and return (nil, nil). The caller of
	// EncodeBinary is responsible for writing the correct NULL value or the
	// length of the data written.
	EncodeBinary(ci *ConnInfo, buf []byte) (newBuf []byte, err error)
}

// TextEncoder is implemented by types that can encode themselves into the
// PostgreSQL text wire format.
type TextEncoder interface {
	// EncodeText should append the text format of self to buf. If self is the
	// SQL value NULL then append nothing and return (nil, nil). The caller of
	// EncodeText is responsible for writing the correct NULL value or the
	// length of the data written.
	EncodeText(ci *ConnInfo, buf []byte) (newBuf []byte, err error)
}

type nullAssignmentError struct {
	dst interface{}
}

func (e *nullAssignmentError) Error() string {
	return fmt.Sprintf("cannot assign NULL to %T", e.dst)
}

type DataType struct {
	Value Value

	textDecoder   TextDecoder
	binaryDecoder BinaryDecoder

	Codec Codec

	Name string
	OID  uint32
}

type ConnInfo struct {
	oidToDataType         map[uint32]*DataType
	nameToDataType        map[string]*DataType
	reflectTypeToName     map[reflect.Type]string
	oidToFormatCode       map[uint32]int16
	oidToResultFormatCode map[uint32]int16

	reflectTypeToDataType map[reflect.Type]*DataType

	preferAssignToOverSQLScannerTypes map[reflect.Type]struct{}
}

func newConnInfo() *ConnInfo {
	return &ConnInfo{
		oidToDataType:                     make(map[uint32]*DataType),
		nameToDataType:                    make(map[string]*DataType),
		reflectTypeToName:                 make(map[reflect.Type]string),
		oidToFormatCode:                   make(map[uint32]int16),
		oidToResultFormatCode:             make(map[uint32]int16),
		preferAssignToOverSQLScannerTypes: make(map[reflect.Type]struct{}),
	}
}

func NewConnInfo() *ConnInfo {
	ci := newConnInfo()

	ci.RegisterDataType(DataType{Name: "_aclitem", OID: ACLItemArrayOID, Codec: &ArrayCodec{ElementCodec: &TextFormatOnlyCodec{TextCodec{}}, ElementOID: ACLItemOID}})
	ci.RegisterDataType(DataType{Name: "_bool", OID: BoolArrayOID, Codec: &ArrayCodec{ElementCodec: BoolCodec{}, ElementOID: BoolOID}})
	ci.RegisterDataType(DataType{Name: "_bpchar", OID: BPCharArrayOID, Codec: &ArrayCodec{ElementCodec: TextCodec{}, ElementOID: BPCharOID}})
	ci.RegisterDataType(DataType{Value: &ByteaArray{}, Name: "_bytea", OID: ByteaArrayOID})
	ci.RegisterDataType(DataType{Value: &CIDRArray{}, Name: "_cidr", OID: CIDRArrayOID})
	ci.RegisterDataType(DataType{Value: &DateArray{}, Name: "_date", OID: DateArrayOID})
	ci.RegisterDataType(DataType{Value: &Float4Array{}, Name: "_float4", OID: Float4ArrayOID})
	ci.RegisterDataType(DataType{Value: &Float8Array{}, Name: "_float8", OID: Float8ArrayOID})
	ci.RegisterDataType(DataType{Value: &InetArray{}, Name: "_inet", OID: InetArrayOID})
	ci.RegisterDataType(DataType{Name: "_int2", OID: Int2ArrayOID, Codec: &ArrayCodec{ElementCodec: Int2Codec{}, ElementOID: Int2OID}})
	ci.RegisterDataType(DataType{Name: "_int4", OID: Int4ArrayOID, Codec: &ArrayCodec{ElementCodec: Int4Codec{}, ElementOID: Int4OID}})
	ci.RegisterDataType(DataType{Name: "_int8", OID: Int8ArrayOID, Codec: &ArrayCodec{ElementCodec: Int8Codec{}, ElementOID: Int8OID}})
	ci.RegisterDataType(DataType{Name: "_box", OID: BoxArrayOID, Codec: &ArrayCodec{ElementCodec: BoxCodec{}, ElementOID: BoxOID}})
	ci.RegisterDataType(DataType{Name: "_circle", OID: CircleArrayOID, Codec: &ArrayCodec{ElementCodec: CircleCodec{}, ElementOID: CircleOID}})
	ci.RegisterDataType(DataType{Name: "_point", OID: PointArrayOID, Codec: &ArrayCodec{ElementCodec: PointCodec{}, ElementOID: PointOID}})
	ci.RegisterDataType(DataType{Name: "_name", OID: NameArrayOID, Codec: &ArrayCodec{ElementCodec: TextCodec{}, ElementOID: NameOID}})
	ci.RegisterDataType(DataType{Value: &NumericArray{}, Name: "_numeric", OID: NumericArrayOID})
	ci.RegisterDataType(DataType{Name: "_text", OID: TextArrayOID, Codec: &ArrayCodec{ElementCodec: TextCodec{}, ElementOID: TextOID}})
	ci.RegisterDataType(DataType{Value: &TimestampArray{}, Name: "_timestamp", OID: TimestampArrayOID})
	ci.RegisterDataType(DataType{Value: &TimestamptzArray{}, Name: "_timestamptz", OID: TimestamptzArrayOID})
	ci.RegisterDataType(DataType{Value: &UUIDArray{}, Name: "_uuid", OID: UUIDArrayOID})
	ci.RegisterDataType(DataType{Name: "_varchar", OID: VarcharArrayOID, Codec: &ArrayCodec{ElementCodec: TextCodec{}, ElementOID: VarcharOID}})
	ci.RegisterDataType(DataType{Name: "aclitem", OID: ACLItemOID, Codec: &TextFormatOnlyCodec{TextCodec{}}})
	ci.RegisterDataType(DataType{Name: "bit", OID: BitOID, Codec: BitsCodec{}})
	ci.RegisterDataType(DataType{Name: "bool", OID: BoolOID, Codec: BoolCodec{}})
	ci.RegisterDataType(DataType{Name: "box", OID: BoxOID, Codec: BoxCodec{}})
	ci.RegisterDataType(DataType{Name: "bpchar", OID: BPCharOID, Codec: TextCodec{}})
	ci.RegisterDataType(DataType{Value: &Bytea{}, Name: "bytea", OID: ByteaOID})
	ci.RegisterDataType(DataType{Value: &QChar{}, Name: "char", OID: QCharOID})
	ci.RegisterDataType(DataType{Value: &CID{}, Name: "cid", OID: CIDOID})
	ci.RegisterDataType(DataType{Value: &CIDR{}, Name: "cidr", OID: CIDROID})
	ci.RegisterDataType(DataType{Name: "circle", OID: CircleOID, Codec: CircleCodec{}})
	ci.RegisterDataType(DataType{Value: &Date{}, Name: "date", OID: DateOID})
	// ci.RegisterDataType(DataType{Value: &Daterange{}, Name: "daterange", OID: DaterangeOID})
	ci.RegisterDataType(DataType{Value: &Float4{}, Name: "float4", OID: Float4OID})
	ci.RegisterDataType(DataType{Value: &Float8{}, Name: "float8", OID: Float8OID})
	ci.RegisterDataType(DataType{Value: &Inet{}, Name: "inet", OID: InetOID})
	ci.RegisterDataType(DataType{Name: "int2", OID: Int2OID, Codec: Int2Codec{}})
	ci.RegisterDataType(DataType{Name: "int4", OID: Int4OID, Codec: Int4Codec{}})
	// ci.RegisterDataType(DataType{Value: &Int4range{}, Name: "int4range", OID: Int4rangeOID})
	ci.RegisterDataType(DataType{Name: "int8", OID: Int8OID, Codec: Int8Codec{}})
	// ci.RegisterDataType(DataType{Value: &Int8range{}, Name: "int8range", OID: Int8rangeOID})
	ci.RegisterDataType(DataType{Value: &Interval{}, Name: "interval", OID: IntervalOID})
	ci.RegisterDataType(DataType{Value: &JSON{}, Name: "json", OID: JSONOID})
	ci.RegisterDataType(DataType{Value: &JSONB{}, Name: "jsonb", OID: JSONBOID})
	ci.RegisterDataType(DataType{Value: &JSONBArray{}, Name: "_jsonb", OID: JSONBArrayOID})
	ci.RegisterDataType(DataType{Value: &Line{}, Name: "line", OID: LineOID})
	ci.RegisterDataType(DataType{Value: &Lseg{}, Name: "lseg", OID: LsegOID})
	ci.RegisterDataType(DataType{Value: &Macaddr{}, Name: "macaddr", OID: MacaddrOID})
	ci.RegisterDataType(DataType{Name: "name", OID: NameOID, Codec: TextCodec{}})
	ci.RegisterDataType(DataType{Value: &Numeric{}, Name: "numeric", OID: NumericOID})
	// ci.RegisterDataType(DataType{Value: &Numrange{}, Name: "numrange", OID: NumrangeOID})
	ci.RegisterDataType(DataType{Value: &OIDValue{}, Name: "oid", OID: OIDOID})
	ci.RegisterDataType(DataType{Value: &Path{}, Name: "path", OID: PathOID})
	ci.RegisterDataType(DataType{Name: "point", OID: PointOID, Codec: PointCodec{}})
	ci.RegisterDataType(DataType{Value: &Polygon{}, Name: "polygon", OID: PolygonOID})
	// ci.RegisterDataType(DataType{Value: &Record{}, Name: "record", OID: RecordOID})
	ci.RegisterDataType(DataType{Name: "text", OID: TextOID, Codec: TextCodec{}})
	ci.RegisterDataType(DataType{Value: &TID{}, Name: "tid", OID: TIDOID})
	ci.RegisterDataType(DataType{Value: &Time{}, Name: "time", OID: TimeOID})
	ci.RegisterDataType(DataType{Value: &Timestamp{}, Name: "timestamp", OID: TimestampOID})
	ci.RegisterDataType(DataType{Value: &Timestamptz{}, Name: "timestamptz", OID: TimestamptzOID})
	// ci.RegisterDataType(DataType{Value: &Tsrange{}, Name: "tsrange", OID: TsrangeOID})
	// ci.RegisterDataType(DataType{Value: &TsrangeArray{}, Name: "_tsrange", OID: TsrangeArrayOID})
	// ci.RegisterDataType(DataType{Value: &Tstzrange{}, Name: "tstzrange", OID: TstzrangeOID})
	// ci.RegisterDataType(DataType{Value: &TstzrangeArray{}, Name: "_tstzrange", OID: TstzrangeArrayOID})
	ci.RegisterDataType(DataType{Name: "unknown", OID: UnknownOID, Codec: TextCodec{}})
	ci.RegisterDataType(DataType{Value: &UUID{}, Name: "uuid", OID: UUIDOID})
	ci.RegisterDataType(DataType{Name: "varbit", OID: VarbitOID, Codec: BitsCodec{}})
	ci.RegisterDataType(DataType{Name: "varchar", OID: VarcharOID, Codec: TextCodec{}})
	ci.RegisterDataType(DataType{Value: &XID{}, Name: "xid", OID: XIDOID})

	registerDefaultPgTypeVariants := func(name, arrayName string, value interface{}) {
		ci.RegisterDefaultPgType(value, name)
		valueType := reflect.TypeOf(value)

		ci.RegisterDefaultPgType(reflect.New(valueType).Interface(), name)

		sliceType := reflect.SliceOf(valueType)
		ci.RegisterDefaultPgType(reflect.MakeSlice(sliceType, 0, 0).Interface(), arrayName)

		ci.RegisterDefaultPgType(reflect.New(sliceType).Interface(), arrayName)
	}

	// Integer types that directly map to a PostgreSQL type
	registerDefaultPgTypeVariants("int2", "_int2", int16(0))
	registerDefaultPgTypeVariants("int4", "_int4", int32(0))
	registerDefaultPgTypeVariants("int8", "_int8", int64(0))

	// Integer types that do not have a direct match to a PostgreSQL type
	registerDefaultPgTypeVariants("int8", "_int8", uint16(0))
	registerDefaultPgTypeVariants("int8", "_int8", uint32(0))
	registerDefaultPgTypeVariants("int8", "_int8", uint64(0))
	registerDefaultPgTypeVariants("int8", "_int8", int(0))
	registerDefaultPgTypeVariants("int8", "_int8", uint(0))

	registerDefaultPgTypeVariants("float4", "_float4", float32(0))
	registerDefaultPgTypeVariants("float8", "_float8", float64(0))

	registerDefaultPgTypeVariants("bool", "_bool", false)
	registerDefaultPgTypeVariants("timestamptz", "_timestamptz", time.Time{})
	registerDefaultPgTypeVariants("text", "_text", "")
	registerDefaultPgTypeVariants("bytea", "_bytea", []byte(nil))

	registerDefaultPgTypeVariants("inet", "_inet", net.IP{})
	ci.RegisterDefaultPgType((*net.IPNet)(nil), "cidr")
	ci.RegisterDefaultPgType([]*net.IPNet(nil), "_cidr")

	return ci
}

func (ci *ConnInfo) RegisterDataType(t DataType) {
	if t.Value != nil {
		t.Value = NewValue(t.Value)
	}

	ci.oidToDataType[t.OID] = &t
	ci.nameToDataType[t.Name] = &t

	{
		var formatCode int16
		if t.Codec != nil {
			formatCode = t.Codec.PreferredFormat()
		} else if _, ok := t.Value.(BinaryEncoder); ok {
			formatCode = BinaryFormatCode
		}
		ci.oidToFormatCode[t.OID] = formatCode
	}

	if d, ok := t.Value.(TextDecoder); ok {
		t.textDecoder = d
	}

	if d, ok := t.Value.(BinaryDecoder); ok {
		t.binaryDecoder = d
	}

	ci.reflectTypeToDataType = nil // Invalidated by type registration
}

// RegisterDefaultPgType registers a mapping of a Go type to a PostgreSQL type name. Typically the data type to be
// encoded or decoded is determined by the PostgreSQL OID. But if the OID of a value to be encoded or decoded is
// unknown, this additional mapping will be used by DataTypeForValue to determine a suitable data type.
func (ci *ConnInfo) RegisterDefaultPgType(value interface{}, name string) {
	ci.reflectTypeToName[reflect.TypeOf(value)] = name
	ci.reflectTypeToDataType = nil // Invalidated by registering a default type
}

func (ci *ConnInfo) DataTypeForOID(oid uint32) (*DataType, bool) {
	dt, ok := ci.oidToDataType[oid]
	return dt, ok
}

func (ci *ConnInfo) DataTypeForName(name string) (*DataType, bool) {
	dt, ok := ci.nameToDataType[name]
	return dt, ok
}

func (ci *ConnInfo) buildReflectTypeToDataType() {
	ci.reflectTypeToDataType = make(map[reflect.Type]*DataType)

	for _, dt := range ci.oidToDataType {
		if dt.Value != nil {
			if _, is := dt.Value.(TypeValue); !is {
				ci.reflectTypeToDataType[reflect.ValueOf(dt.Value).Type()] = dt
			}
		}
	}

	for reflectType, name := range ci.reflectTypeToName {
		if dt, ok := ci.nameToDataType[name]; ok {
			ci.reflectTypeToDataType[reflectType] = dt
		}
	}
}

// DataTypeForValue finds a data type suitable for v. Use RegisterDataType to register types that can encode and decode
// themselves. Use RegisterDefaultPgType to register that can be handled by a registered data type.
func (ci *ConnInfo) DataTypeForValue(v interface{}) (*DataType, bool) {
	if ci.reflectTypeToDataType == nil {
		ci.buildReflectTypeToDataType()
	}

	if tv, ok := v.(TypeValue); ok {
		dt, ok := ci.nameToDataType[tv.TypeName()]
		return dt, ok
	}

	dt, ok := ci.reflectTypeToDataType[reflect.TypeOf(v)]
	return dt, ok
}

func (ci *ConnInfo) FormatCodeForOID(oid uint32) int16 {
	fc, ok := ci.oidToFormatCode[oid]
	if ok {
		return fc
	}
	return TextFormatCode
}

// PreferAssignToOverSQLScannerForType makes a sql.Scanner type use the AssignTo scan path instead of sql.Scanner.
// This is primarily for efficient integration with 3rd party numeric and UUID types.
func (ci *ConnInfo) PreferAssignToOverSQLScannerForType(value interface{}) {
	ci.preferAssignToOverSQLScannerTypes[reflect.TypeOf(value)] = struct{}{}
}

// EncodePlan is a precompiled plan to encode a particular type into a particular OID and format.
type EncodePlan interface {
	// Encode appends the encoded bytes of value to buf. If value is the SQL value NULL then append nothing and return
	// (nil, nil). The caller of Encode is responsible for writing the correct NULL value or the length of the data
	// written.
	Encode(value interface{}, buf []byte) (newBuf []byte, err error)
}

// ScanPlan is a precompiled plan to scan into a type of destination.
type ScanPlan interface {
	// Scan scans src into dst. If the dst type has changed in an incompatible way a ScanPlan should automatically
	// replan and scan.
	Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error
}

type scanPlanDstResultDecoder struct{}

func (scanPlanDstResultDecoder) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	newPlan := ci.PlanScan(oid, formatCode, dst)
	return newPlan.Scan(ci, oid, formatCode, src, dst)
}

type scanPlanDstBinaryDecoder struct{}

func (scanPlanDstBinaryDecoder) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if d, ok := (dst).(BinaryDecoder); ok {
		return d.DecodeBinary(ci, src)
	}

	newPlan := ci.PlanScan(oid, formatCode, dst)
	return newPlan.Scan(ci, oid, formatCode, src, dst)
}

type scanPlanDstTextDecoder struct{}

func (plan scanPlanDstTextDecoder) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if d, ok := (dst).(TextDecoder); ok {
		return d.DecodeText(ci, src)
	}

	newPlan := ci.PlanScan(oid, formatCode, dst)
	return newPlan.Scan(ci, oid, formatCode, src, dst)
}

type scanPlanDataTypeSQLScanner DataType

func (plan *scanPlanDataTypeSQLScanner) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	scanner, ok := dst.(sql.Scanner)
	if !ok {
		newPlan := ci.PlanScan(oid, formatCode, dst)
		return newPlan.Scan(ci, oid, formatCode, src, dst)
	}

	dt := (*DataType)(plan)
	if dt.Codec != nil {
		sqlValue, err := dt.Codec.DecodeDatabaseSQLValue(ci, oid, formatCode, src)
		if err != nil {
			return err
		}
		return scanner.Scan(sqlValue)
	}
	var err error
	switch formatCode {
	case BinaryFormatCode:
		err = dt.binaryDecoder.DecodeBinary(ci, src)
	case TextFormatCode:
		err = dt.textDecoder.DecodeText(ci, src)
	}
	if err != nil {
		return err
	}

	sqlSrc, err := DatabaseSQLValue(ci, dt.Value)
	if err != nil {
		return err
	}
	return scanner.Scan(sqlSrc)
}

type scanPlanDataTypeAssignTo DataType

func (plan *scanPlanDataTypeAssignTo) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	dt := (*DataType)(plan)
	var err error

	switch formatCode {
	case BinaryFormatCode:
		if dt.binaryDecoder == nil {
			return fmt.Errorf("dt.binaryDecoder is nil")
		}
		err = dt.binaryDecoder.DecodeBinary(ci, src)
	case TextFormatCode:
		if dt.textDecoder == nil {
			return fmt.Errorf("dt.textDecoder is nil")
		}
		err = dt.textDecoder.DecodeText(ci, src)
	}
	if err != nil {
		return err
	}

	assignToErr := dt.Value.AssignTo(dst)
	if assignToErr == nil {
		return nil
	}

	if dstPtr, ok := dst.(*interface{}); ok {
		*dstPtr = dt.Value.Get()
		return nil
	}

	// assignToErr might have failed because the type of destination has changed
	newPlan := ci.PlanScan(oid, formatCode, dst)
	if newPlan, sameType := newPlan.(*scanPlanDataTypeAssignTo); !sameType {
		return newPlan.Scan(ci, oid, formatCode, src, dst)
	}

	return assignToErr
}

type scanPlanSQLScanner struct{}

func (scanPlanSQLScanner) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	scanner := dst.(sql.Scanner)
	if src == nil {
		// This is necessary because interface value []byte:nil does not equal nil:nil for the binary format path and the
		// text format path would be converted to empty string.
		return scanner.Scan(nil)
	} else if formatCode == BinaryFormatCode {
		return scanner.Scan(src)
	} else {
		return scanner.Scan(string(src))
	}
}

type scanPlanReflection struct{}

func (scanPlanReflection) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	// We might be given a pointer to something that implements the decoder interface(s),
	// even though the pointer itself doesn't.
	refVal := reflect.ValueOf(dst)
	if refVal.Kind() == reflect.Ptr && refVal.Type().Elem().Kind() == reflect.Ptr {
		// If the database returned NULL, then we set dest as nil to indicate that.
		if src == nil {
			nilPtr := reflect.Zero(refVal.Type().Elem())
			refVal.Elem().Set(nilPtr)
			return nil
		}

		// We need to allocate an element, and set the destination to it
		// Then we can retry as that element.
		elemPtr := reflect.New(refVal.Type().Elem().Elem())
		refVal.Elem().Set(elemPtr)

		plan := ci.PlanScan(oid, formatCode, elemPtr.Interface())
		return plan.Scan(ci, oid, formatCode, src, elemPtr.Interface())
	}

	return scanUnknownType(oid, formatCode, src, dst)
}

type scanPlanBinaryInt64 struct{}

func (scanPlanBinaryInt64) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if src == nil {
		return fmt.Errorf("cannot scan null into %T", dst)
	}

	if len(src) != 8 {
		return fmt.Errorf("invalid length for int8: %v", len(src))
	}

	if p, ok := (dst).(*int64); ok {
		*p = int64(binary.BigEndian.Uint64(src))
		return nil
	}

	newPlan := ci.PlanScan(oid, formatCode, dst)
	return newPlan.Scan(ci, oid, formatCode, src, dst)
}

type scanPlanBinaryFloat32 struct{}

func (scanPlanBinaryFloat32) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if src == nil {
		return fmt.Errorf("cannot scan null into %T", dst)
	}

	if len(src) != 4 {
		return fmt.Errorf("invalid length for int4: %v", len(src))
	}

	if p, ok := (dst).(*float32); ok {
		n := int32(binary.BigEndian.Uint32(src))
		*p = float32(math.Float32frombits(uint32(n)))
		return nil
	}

	newPlan := ci.PlanScan(oid, formatCode, dst)
	return newPlan.Scan(ci, oid, formatCode, src, dst)
}

type scanPlanBinaryFloat64 struct{}

func (scanPlanBinaryFloat64) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if src == nil {
		return fmt.Errorf("cannot scan null into %T", dst)
	}

	if len(src) != 8 {
		return fmt.Errorf("invalid length for int8: %v", len(src))
	}

	if p, ok := (dst).(*float64); ok {
		n := int64(binary.BigEndian.Uint64(src))
		*p = float64(math.Float64frombits(uint64(n)))
		return nil
	}

	newPlan := ci.PlanScan(oid, formatCode, dst)
	return newPlan.Scan(ci, oid, formatCode, src, dst)
}

type scanPlanBinaryBytes struct{}

func (scanPlanBinaryBytes) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if p, ok := (dst).(*[]byte); ok {
		*p = src
		return nil
	}

	newPlan := ci.PlanScan(oid, formatCode, dst)
	return newPlan.Scan(ci, oid, formatCode, src, dst)
}

type scanPlanString struct{}

func (scanPlanString) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if src == nil {
		return fmt.Errorf("cannot scan null into %T", dst)
	}

	if p, ok := (dst).(*string); ok {
		*p = string(src)
		return nil
	}

	newPlan := ci.PlanScan(oid, formatCode, dst)
	return newPlan.Scan(ci, oid, formatCode, src, dst)
}

type pointerPointerScanPlan struct {
	dstType reflect.Type
	next    ScanPlan
}

func (plan *pointerPointerScanPlan) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if plan.dstType != reflect.TypeOf(dst) {
		newPlan := ci.PlanScan(oid, formatCode, dst)
		return newPlan.Scan(ci, oid, formatCode, src, dst)
	}

	el := reflect.ValueOf(dst).Elem()
	if src == nil {
		el.Set(reflect.Zero(el.Type()))
		return nil
	}

	el.Set(reflect.New(el.Type().Elem()))
	return plan.next.Scan(ci, oid, formatCode, src, el.Interface())
}

func tryPointerPointerScanPlan(dst interface{}) (plan *pointerPointerScanPlan, nextDst interface{}, ok bool) {
	if dstValue := reflect.ValueOf(dst); dstValue.Kind() == reflect.Ptr {
		elemValue := dstValue.Elem()
		if elemValue.Kind() == reflect.Ptr {
			plan = &pointerPointerScanPlan{dstType: dstValue.Type()}
			return plan, reflect.Zero(elemValue.Type()).Interface(), true
		}
	}

	return nil, nil, false
}

var elemKindToBasePointerTypes map[reflect.Kind]reflect.Type = map[reflect.Kind]reflect.Type{
	reflect.Int:     reflect.TypeOf(new(int)),
	reflect.Int8:    reflect.TypeOf(new(int8)),
	reflect.Int16:   reflect.TypeOf(new(int16)),
	reflect.Int32:   reflect.TypeOf(new(int32)),
	reflect.Int64:   reflect.TypeOf(new(int64)),
	reflect.Uint:    reflect.TypeOf(new(uint)),
	reflect.Uint8:   reflect.TypeOf(new(uint8)),
	reflect.Uint16:  reflect.TypeOf(new(uint16)),
	reflect.Uint32:  reflect.TypeOf(new(uint32)),
	reflect.Uint64:  reflect.TypeOf(new(uint64)),
	reflect.Float32: reflect.TypeOf(new(float32)),
	reflect.Float64: reflect.TypeOf(new(float64)),
	reflect.String:  reflect.TypeOf(new(string)),
}

type baseTypeScanPlan struct {
	dstType     reflect.Type
	nextDstType reflect.Type
	next        ScanPlan
}

func (plan *baseTypeScanPlan) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if plan.dstType != reflect.TypeOf(dst) {
		newPlan := ci.PlanScan(oid, formatCode, dst)
		return newPlan.Scan(ci, oid, formatCode, src, dst)
	}

	return plan.next.Scan(ci, oid, formatCode, src, reflect.ValueOf(dst).Convert(plan.nextDstType).Interface())
}

func tryBaseTypeScanPlan(dst interface{}) (plan *baseTypeScanPlan, nextDst interface{}, ok bool) {
	dstValue := reflect.ValueOf(dst)

	if dstValue.Kind() == reflect.Ptr {
		elemValue := dstValue.Elem()
		nextDstType := elemKindToBasePointerTypes[elemValue.Kind()]
		if nextDstType != nil && dstValue.Type() != nextDstType {
			return &baseTypeScanPlan{dstType: dstValue.Type(), nextDstType: nextDstType}, dstValue.Convert(nextDstType).Interface(), true
		}
	}

	return nil, nil, false
}

type pointerEmptyInterfaceScanPlan struct {
	codec Codec
}

func (plan *pointerEmptyInterfaceScanPlan) Scan(ci *ConnInfo, oid uint32, formatCode int16, src []byte, dst interface{}) error {
	value, err := plan.codec.DecodeValue(ci, oid, formatCode, src)
	if err != nil {
		return err
	}

	ptrAny := dst.(*interface{})
	*ptrAny = value

	return nil
}

// PlanScan prepares a plan to scan a value into dst.
func (ci *ConnInfo) PlanScan(oid uint32, formatCode int16, dst interface{}) ScanPlan {
	switch formatCode {
	case BinaryFormatCode:
		switch dst.(type) {
		case *string:
			switch oid {
			case TextOID, VarcharOID:
				return scanPlanString{}
			}
		case *int64:
			if oid == Int8OID {
				return scanPlanBinaryInt64{}
			}
		case *float32:
			if oid == Float4OID {
				return scanPlanBinaryFloat32{}
			}
		case *float64:
			if oid == Float8OID {
				return scanPlanBinaryFloat64{}
			}
		case *[]byte:
			switch oid {
			case ByteaOID, TextOID, VarcharOID, JSONOID:
				return scanPlanBinaryBytes{}
			}
		case BinaryDecoder:
			return scanPlanDstBinaryDecoder{}
		}
	case TextFormatCode:
		switch dst.(type) {
		case *string:
			return scanPlanString{}
		case *[]byte:
			if oid != ByteaOID {
				return scanPlanBinaryBytes{}
			}
		case TextDecoder:
			return scanPlanDstTextDecoder{}
		case TextScanner:
			return scanPlanTextAnyToTextScanner{}
		}
	}

	var dt *DataType

	if oid == 0 {
		if dataType, ok := ci.DataTypeForValue(dst); ok {
			dt = dataType
		}
	} else {
		if dataType, ok := ci.DataTypeForOID(oid); ok {
			dt = dataType
		}
	}

	if dt != nil && dt.Codec != nil {
		if plan := dt.Codec.PlanScan(ci, oid, formatCode, dst, false); plan != nil {
			return plan
		}

		if pointerPointerPlan, nextDst, ok := tryPointerPointerScanPlan(dst); ok {
			if nextPlan := ci.PlanScan(oid, formatCode, nextDst); nextPlan != nil {
				pointerPointerPlan.next = nextPlan
				return pointerPointerPlan
			}
		}

		if baseTypePlan, nextDst, ok := tryBaseTypeScanPlan(dst); ok {
			if nextPlan := ci.PlanScan(oid, formatCode, nextDst); nextPlan != nil {
				baseTypePlan.next = nextPlan
				return baseTypePlan
			}
		}

		if _, ok := dst.(*interface{}); ok {
			return &pointerEmptyInterfaceScanPlan{codec: dt.Codec}
		}
	}

	if dt != nil {
		if _, ok := dst.(sql.Scanner); ok {
			if _, found := ci.preferAssignToOverSQLScannerTypes[reflect.TypeOf(dst)]; !found {
				return (*scanPlanDataTypeSQLScanner)(dt)
			}
		}
		return (*scanPlanDataTypeAssignTo)(dt)
	}

	if _, ok := dst.(sql.Scanner); ok {
		return scanPlanSQLScanner{}
	}

	return scanPlanReflection{}
}

func (ci *ConnInfo) Scan(oid uint32, formatCode int16, src []byte, dst interface{}) error {
	if dst == nil {
		return nil
	}

	plan := ci.PlanScan(oid, formatCode, dst)
	return plan.Scan(ci, oid, formatCode, src, dst)
}

func scanUnknownType(oid uint32, formatCode int16, buf []byte, dest interface{}) error {
	switch dest := dest.(type) {
	case *string:
		if formatCode == BinaryFormatCode {
			return fmt.Errorf("unknown oid %d in binary format cannot be scanned into %T", oid, dest)
		}
		*dest = string(buf)
		return nil
	case *[]byte:
		*dest = buf
		return nil
	default:
		if nextDst, retry := GetAssignToDstType(dest); retry {
			return scanUnknownType(oid, formatCode, buf, nextDst)
		}
		return fmt.Errorf("unknown oid %d cannot be scanned into %T", oid, dest)
	}
}

// NewValue returns a new instance of the same type as v.
func NewValue(v Value) Value {
	if tv, ok := v.(TypeValue); ok {
		return tv.NewTypeValue()
	} else {
		return reflect.New(reflect.ValueOf(v).Elem().Type()).Interface().(Value)
	}
}

var ErrScanTargetTypeChanged = errors.New("scan target type changed")

func codecScan(codec Codec, ci *ConnInfo, oid uint32, format int16, src []byte, dst interface{}) error {
	scanPlan := codec.PlanScan(ci, oid, format, dst, true)
	if scanPlan == nil {
		return fmt.Errorf("PlanScan did not find a plan")
	}
	return scanPlan.Scan(ci, oid, format, src, dst)
}

func codecDecodeToTextFormat(codec Codec, ci *ConnInfo, oid uint32, format int16, src []byte) (driver.Value, error) {
	if src == nil {
		return nil, nil
	}

	if format == TextFormatCode {
		return string(src), nil
	} else {
		value, err := codec.DecodeValue(ci, oid, format, src)
		if err != nil {
			return nil, err
		}
		buf, err := ci.Encode(oid, TextFormatCode, value, nil)
		if err != nil {
			return nil, err
		}
		return string(buf), nil
	}
}

// PlanEncode returns an Encode plan for encoding value into PostgreSQL format for oid and format. If no plan can be
// found then nil is returned.
func (ci *ConnInfo) PlanEncode(oid uint32, format int16, value interface{}) EncodePlan {

	var dt *DataType

	if oid == 0 {
		if dataType, ok := ci.DataTypeForValue(value); ok {
			dt = dataType
		}
	} else {
		if dataType, ok := ci.DataTypeForOID(oid); ok {
			dt = dataType
		}
	}

	if dt != nil && dt.Codec != nil {
		if plan := dt.Codec.PlanEncode(ci, oid, format, value); plan != nil {
			return plan
		}

		if derefPointerPlan, nextValue, ok := tryDerefPointerEncodePlan(value); ok {
			if nextPlan := ci.PlanEncode(oid, format, nextValue); nextPlan != nil {
				derefPointerPlan.next = nextPlan
				return derefPointerPlan
			}
		}

		if baseTypePlan, nextValue, ok := tryBaseTypeEncodePlan(value); ok {
			if nextPlan := ci.PlanEncode(oid, format, nextValue); nextPlan != nil {
				baseTypePlan.next = nextPlan
				return baseTypePlan
			}
		}

	}

	return nil
}

type derefPointerEncodePlan struct {
	next EncodePlan
}

func (plan *derefPointerEncodePlan) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	ptr := reflect.ValueOf(value)

	if ptr.IsNil() {
		return nil, nil
	}

	return plan.next.Encode(ptr.Elem().Interface(), buf)
}

func tryDerefPointerEncodePlan(value interface{}) (plan *derefPointerEncodePlan, nextValue interface{}, ok bool) {
	if valueType := reflect.TypeOf(value); valueType.Kind() == reflect.Ptr {
		return &derefPointerEncodePlan{}, reflect.New(valueType.Elem()).Elem().Interface(), true
	}

	return nil, nil, false
}

var kindToBaseTypes map[reflect.Kind]reflect.Type = map[reflect.Kind]reflect.Type{
	reflect.Int:     reflect.TypeOf(int(0)),
	reflect.Int8:    reflect.TypeOf(int8(0)),
	reflect.Int16:   reflect.TypeOf(int16(0)),
	reflect.Int32:   reflect.TypeOf(int32(0)),
	reflect.Int64:   reflect.TypeOf(int64(0)),
	reflect.Uint:    reflect.TypeOf(uint(0)),
	reflect.Uint8:   reflect.TypeOf(uint8(0)),
	reflect.Uint16:  reflect.TypeOf(uint16(0)),
	reflect.Uint32:  reflect.TypeOf(uint32(0)),
	reflect.Uint64:  reflect.TypeOf(uint64(0)),
	reflect.Float32: reflect.TypeOf(float32(0)),
	reflect.Float64: reflect.TypeOf(float64(0)),
	reflect.String:  reflect.TypeOf(""),
}

type baseTypeEncodePlan struct {
	nextValueType reflect.Type
	next          EncodePlan
}

func (plan *baseTypeEncodePlan) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	return plan.next.Encode(reflect.ValueOf(value).Convert(plan.nextValueType).Interface(), buf)
}

func tryBaseTypeEncodePlan(value interface{}) (plan *baseTypeEncodePlan, nextValue interface{}, ok bool) {
	refValue := reflect.ValueOf(value)

	nextValueType := kindToBaseTypes[refValue.Kind()]
	if nextValueType != nil && refValue.Type() != nextValueType {
		return &baseTypeEncodePlan{nextValueType: nextValueType}, refValue.Convert(nextValueType).Interface(), true
	}

	return nil, nil, false
}

// Encode appends the encoded bytes of value to buf. If value is the SQL value NULL then append nothing and return
// (nil, nil). The caller of Encode is responsible for writing the correct NULL value or the length of the data
// written.
func (ci *ConnInfo) Encode(oid uint32, formatCode int16, value interface{}, buf []byte) (newBuf []byte, err error) {
	if value == nil {
		return nil, nil
	}

	plan := ci.PlanEncode(oid, formatCode, value)
	if plan == nil {
		return nil, fmt.Errorf("unable to encode %v", value)
	}
	return plan.Encode(value, buf)
}

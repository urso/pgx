package pgtype_test

import (
	"bytes"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgtype/testutil"
)

func isExpectedEqBits(a interface{}) func(interface{}) bool {
	return func(v interface{}) bool {
		ab := a.(pgtype.Bits)
		vb := v.(pgtype.Bits)
		return bytes.Compare(ab.Bytes, vb.Bytes) == 0 && ab.Len == vb.Len && ab.Valid == vb.Valid
	}
}

func TestBitsCodecBit(t *testing.T) {
	testPgxCodec(t, "bit(40)", []PgxTranscodeTestCase{
		{
			pgtype.Bits{Bytes: []byte{0, 0, 0, 0, 0}, Len: 40, Valid: true},
			new(pgtype.Bits),
			isExpectedEqBits(pgtype.Bits{Bytes: []byte{0, 0, 0, 0, 0}, Len: 40, Valid: true}),
		},
		{
			pgtype.Bits{Bytes: []byte{0, 1, 128, 254, 255}, Len: 40, Valid: true},
			new(pgtype.Bits),
			isExpectedEqBits(pgtype.Bits{Bytes: []byte{0, 1, 128, 254, 255}, Len: 40, Valid: true}),
		},
		{pgtype.Bits{}, new(pgtype.Bits), isExpectedEqBits(pgtype.Bits{})},
		{nil, new(pgtype.Bits), isExpectedEqBits(pgtype.Bits{})},
	})
}

func TestBitsCodecVarbit(t *testing.T) {
	testPgxCodec(t, "varbit", []PgxTranscodeTestCase{
		{
			pgtype.Bits{Bytes: []byte{}, Len: 0, Valid: true},
			new(pgtype.Bits),
			isExpectedEqBits(pgtype.Bits{Bytes: []byte{}, Len: 0, Valid: true}),
		},
		{
			pgtype.Bits{Bytes: []byte{0, 1, 128, 254, 255}, Len: 40, Valid: true},
			new(pgtype.Bits),
			isExpectedEqBits(pgtype.Bits{Bytes: []byte{0, 1, 128, 254, 255}, Len: 40, Valid: true}),
		},
		{
			pgtype.Bits{Bytes: []byte{0, 1, 128, 254, 128}, Len: 33, Valid: true},
			new(pgtype.Bits),
			isExpectedEqBits(pgtype.Bits{Bytes: []byte{0, 1, 128, 254, 128}, Len: 33, Valid: true}),
		},
		{pgtype.Bits{}, new(pgtype.Bits), isExpectedEqBits(pgtype.Bits{})},
		{nil, new(pgtype.Bits), isExpectedEqBits(pgtype.Bits{})},
	})
}

func TestBitsNormalize(t *testing.T) {
	testutil.TestSuccessfulNormalize(t, []testutil.NormalizeTest{
		{
			SQL:   "select B'111111111'",
			Value: &pgtype.Bits{Bytes: []byte{255, 128}, Len: 9, Valid: true},
		},
	})
}

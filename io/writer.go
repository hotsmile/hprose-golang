/**********************************************************\
|                                                          |
|                          hprose                          |
|                                                          |
| Official WebSite: http://www.hprose.com/                 |
|                   http://www.hprose.org/                 |
|                                                          |
\**********************************************************/
/**********************************************************\
 *                                                        *
 * io/writer.go                                           *
 *                                                        *
 * hprose writer for Go.                                  *
 *                                                        *
 * LastModified: Sep 1, 2016                              *
 * Author: Ma Bingyao <andot@hprose.com>                  *
 *                                                        *
\**********************************************************/

package io

import (
	"container/list"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"time"
	"unsafe"
)

// Writer is a fine-grained operation struct for Hprose serialization
type Writer struct {
	BytesWriter
	Simple   bool
	classref map[uintptr]int
	ref      map[uintptr]int
	refcount int
}

// NewWriter is the constructor for Hprose Writer
func NewWriter(simple bool) (w *Writer) {
	w = new(Writer)
	w.Simple = simple
	w.classref = map[uintptr]int{}
	if !simple {
		w.ref = map[uintptr]int{}
	}
	return
}

// Serialize a data v to stream
func (w *Writer) Serialize(v interface{}) {
	if v == nil {
		w.WriteNil()
	} else {
		w.WriteValue(reflect.ValueOf(v))
	}
}

// WriteValue to stream
func (w *Writer) WriteValue(v reflect.Value) {
	valueEncoders[v.Kind()](w, v)
}

// WriteNil to stream
func (w *Writer) WriteNil() {
	w.writeByte(TagNull)
}

// WriteBool to stream
func (w *Writer) WriteBool(b bool) {
	if b {
		w.writeByte(TagTrue)
	} else {
		w.writeByte(TagFalse)
	}
}

// WriteInt to stream
func (w *Writer) WriteInt(i int64) {
	if i >= 0 && i <= 9 {
		w.writeByte(byte('0' + i))
		return
	}
	if i >= math.MinInt32 && i <= math.MaxInt32 {
		w.writeByte(TagInteger)
	} else {
		w.writeByte(TagLong)
	}
	var buf [20]byte
	w.write(getIntBytes(buf[:], i))
	w.writeByte(TagSemicolon)
}

// WriteUint to stream
func (w *Writer) WriteUint(i uint64) {
	if i <= 9 {
		w.writeByte(byte('0' + i))
		return
	}
	if i <= math.MaxInt32 {
		w.writeByte(TagInteger)
	} else {
		w.writeByte(TagLong)
	}
	var buf [20]byte
	w.write(getUintBytes(buf[:], i))
	w.writeByte(TagSemicolon)
}

// WriteFloat to stream
func (w *Writer) WriteFloat(f float64, bitSize int) {
	if f != f {
		w.writeByte(TagNaN)
		return
	}
	if f > math.MaxFloat64 {
		w.write([]byte{TagInfinity, TagPos})
		return
	}
	if f < -math.MaxFloat64 {
		w.write([]byte{TagInfinity, TagNeg})
		return
	}
	w.writeByte(TagDouble)
	var buf [64]byte
	w.write(strconv.AppendFloat(buf[:0], f, 'g', -1, bitSize))
	w.writeByte(TagSemicolon)
}

// WriteComplex64 to stream
func (w *Writer) WriteComplex64(c complex64) {
	if imag(c) == 0 {
		w.WriteFloat(float64(real(c)), 32)
		return
	}
	setRef(w, nil)
	writeListHeader(w, 2)
	w.WriteFloat(float64(real(c)), 32)
	w.WriteFloat(float64(imag(c)), 32)
	writeListFooter(w)
}

// WriteComplex128 to stream
func (w *Writer) WriteComplex128(c complex128) {
	if imag(c) == 0 {
		w.WriteFloat(real(c), 64)
		return
	}
	setRef(w, nil)
	writeListHeader(w, 2)
	w.WriteFloat(real(c), 64)
	w.WriteFloat(imag(c), 64)
	writeListFooter(w)
}

// WriteString to stream
func (w *Writer) WriteString(str string) {
	length := utf16Length(str)
	switch {
	case length == 0:
		w.writeByte(TagEmpty)
	case length < 0:
		w.WriteBytes(*(*[]byte)(unsafe.Pointer(&str)))
	case length == 1:
		w.writeByte(TagUTF8Char)
		w.writeString(str)
	default:
		setRef(w, nil)
		writeString(w, str, length)
	}
}

// WriteBytes to stream
func (w *Writer) WriteBytes(bytes []byte) {
	setRef(w, nil)
	writeBytes(w, bytes)
}

// WriteBigInt to stream
func (w *Writer) WriteBigInt(bi *big.Int) {
	w.writeByte(TagLong)
	w.writeString(bi.String())
	w.writeByte(TagSemicolon)
}

// WriteBigRat to stream
func (w *Writer) WriteBigRat(br *big.Rat) {
	if br.IsInt() {
		w.WriteBigInt(br.Num())
	} else {
		str := br.String()
		setRef(w, nil)
		writeString(w, str, len(str))
	}
}

// WriteBigFloat to stream
func (w *Writer) WriteBigFloat(bf *big.Float) {
	w.writeByte(TagDouble)
	var buf [64]byte
	w.write(bf.Append(buf[:0], 'g', -1))
	w.writeByte(TagSemicolon)
}

// WriteTime to stream
func (w *Writer) WriteTime(t *time.Time) {
	ptr := unsafe.Pointer(t)
	if writeRef(w, ptr) {
		return
	}
	setRef(w, ptr)
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	nsec := t.Nanosecond()
	tag := TagSemicolon
	if t.Location() == time.UTC {
		tag = TagUTC
	}
	var buf [27]byte
	if hour == 0 && min == 0 && sec == 0 && nsec == 0 {
		datelen := formatDate(buf[:], year, int(month), day)
		buf[datelen] = tag
		w.write(buf[:datelen+1])
	} else if year == 1970 && month == 1 && day == 1 {
		timelen := formatTime(buf[:], hour, min, sec, nsec)
		buf[timelen] = tag
		w.write(buf[:timelen+1])
	} else {
		datelen := formatDate(buf[:], year, int(month), day)
		timelen := formatTime(buf[datelen:], hour, min, sec, nsec)
		datetimelen := datelen + timelen
		buf[datetimelen] = tag
		w.write(buf[:datetimelen+1])
	}
}

// WriteList to stream
func (w *Writer) WriteList(lst *list.List) {
	ptr := unsafe.Pointer(lst)
	if writeRef(w, ptr) {
		return
	}
	setRef(w, ptr)
	count := lst.Len()
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	for e := lst.Front(); e != nil; e = e.Next() {
		w.Serialize(e.Value)
	}
	writeListFooter(w)
}

// WriteTuple to stream
func (w *Writer) WriteTuple(tuple ...interface{}) {
	setRef(w, nil)
	count := len(tuple)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	for _, v := range tuple {
		w.Serialize(v)
	}
	writeListFooter(w)
}

// WriteBoolSlice to stream
func (w *Writer) WriteBoolSlice(slice []bool) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	boolSliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteIntSlice to stream
func (w *Writer) WriteIntSlice(slice []int) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	intSliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteInt8Slice to stream
func (w *Writer) WriteInt8Slice(slice []int8) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	int8SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteInt16Slice to stream
func (w *Writer) WriteInt16Slice(slice []int16) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	int16SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteInt32Slice to stream
func (w *Writer) WriteInt32Slice(slice []int32) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	int32SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteInt64Slice to stream
func (w *Writer) WriteInt64Slice(slice []int64) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	int64SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteUintSlice to stream
func (w *Writer) WriteUintSlice(slice []uint) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	uintSliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteUint8Slice to stream
func (w *Writer) WriteUint8Slice(slice []uint8) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	uint8SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteUint16Slice to stream
func (w *Writer) WriteUint16Slice(slice []uint16) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	uint16SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteUint32Slice to stream
func (w *Writer) WriteUint32Slice(slice []uint32) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	uint32SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteUint64Slice to stream
func (w *Writer) WriteUint64Slice(slice []uint64) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	uint64SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteUintptrSlice to stream
func (w *Writer) WriteUintptrSlice(slice []uintptr) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	uintptrSliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteFloat32Slice to stream
func (w *Writer) WriteFloat32Slice(slice []float32) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	float32SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteFloat64Slice to stream
func (w *Writer) WriteFloat64Slice(slice []float64) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	float64SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteComplex64Slice to stream
func (w *Writer) WriteComplex64Slice(slice []complex64) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	complex64SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteComplex128Slice to stream
func (w *Writer) WriteComplex128Slice(slice []complex128) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	complex128SliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteStringSlice to stream
func (w *Writer) WriteStringSlice(slice []string) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	stringSliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// WriteBytesSlice to stream
func (w *Writer) WriteBytesSlice(slice [][]byte) {
	setRef(w, nil)
	count := len(slice)
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	bytesSliceEncoder(w, unsafe.Pointer(&slice))
	writeListFooter(w)
}

// Reset the reference counter
func (w *Writer) Reset() {
	for k := range w.classref {
		delete(w.classref, k)
	}
	if w.Simple {
		return
	}
	w.refcount = 0
	for k := range w.ref {
		delete(w.ref, k)
	}
}

// private functions

func writeRef(w *Writer, ref unsafe.Pointer) bool {
	if w.Simple {
		return false
	}
	n, found := w.ref[uintptr(ref)]
	if found {
		w.writeByte(TagRef)
		var buf [20]byte
		w.write(getIntBytes(buf[:], int64(n)))
		w.writeByte(TagSemicolon)
	}
	return found
}

func setRef(writer *Writer, ref unsafe.Pointer) {
	if writer.Simple {
		return
	}
	if ref != nil {
		writer.ref[uintptr(ref)] = writer.refcount
	}
	writer.refcount++
}

func writeString(w *Writer, str string, length int) {
	w.writeByte(TagString)
	var buf [20]byte
	w.write(getIntBytes(buf[:], int64(length)))
	w.writeByte(TagQuote)
	w.writeString(str)
	w.writeByte(TagQuote)
}

func writeBytes(w *Writer, bytes []byte) {
	count := len(bytes)
	if count == 0 {
		w.write([]byte{TagBytes, TagQuote, TagQuote})
		return
	}
	w.writeByte(TagBytes)
	var buf [20]byte
	w.write(getIntBytes(buf[:], int64(count)))
	w.writeByte(TagQuote)
	w.write(bytes)
	w.writeByte(TagQuote)
}

func writeListHeader(w *Writer, count int) {
	w.writeByte(TagList)
	var buf [20]byte
	w.write(getIntBytes(buf[:], int64(count)))
	w.writeByte(TagOpenbrace)
}

func writeListBody(w *Writer, list reflect.Value, count int) {
	for i := 0; i < count; i++ {
		e := list.Index(i)
		valueEncoders[e.Kind()](w, e)
	}
}

func writeListFooter(w *Writer) {
	w.writeByte(TagClosebrace)
}

func writeEmptyList(w *Writer) {
	w.write([]byte{TagList, TagOpenbrace, TagClosebrace})
}

func writeArray(w *Writer, v reflect.Value) {
	st := reflect.SliceOf(v.Type().Elem())
	sliceType := (*emptyInterface)(unsafe.Pointer(&st)).ptr
	count := v.Len()
	if sliceType == bytesType {
		sliceHeader := reflect.SliceHeader{
			Data: (*emptyInterface)(unsafe.Pointer(&v)).ptr,
			Len:  count,
			Cap:  count,
		}
		writeBytes(w, *(*[]byte)(unsafe.Pointer(&sliceHeader)))
		return
	}
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	encoder := sliceBodyEncoders[sliceType]
	if encoder != nil {
		sliceHeader := reflect.SliceHeader{
			Data: (*emptyInterface)(unsafe.Pointer(&v)).ptr,
			Len:  count,
			Cap:  count,
		}
		encoder(w, unsafe.Pointer(&sliceHeader))
	} else {
		writeListBody(w, v, count)
	}
	writeListFooter(w)
}

func writeSlice(w *Writer, v reflect.Value) {
	val := (*reflectValue)(unsafe.Pointer(&v))
	if val.typ == bytesType {
		writeBytes(w, v.Bytes())
		return
	}
	count := v.Len()
	if count == 0 {
		writeEmptyList(w)
		return
	}
	writeListHeader(w, count)
	encoder := sliceBodyEncoders[val.typ]
	if encoder != nil {
		encoder(w, val.ptr)
	} else {
		writeListBody(w, v, count)
	}
	writeListFooter(w)
}

func writeEmptyMap(w *Writer) {
	w.write([]byte{TagMap, TagOpenbrace, TagClosebrace})
}

func writeMapHeader(w *Writer, count int) {
	w.writeByte(TagMap)
	var buf [20]byte
	w.write(getIntBytes(buf[:], int64(count)))
	w.writeByte(TagOpenbrace)
}

func writeMapBody(w *Writer, v reflect.Value) {
	mapType := v.Type()
	keyEncoder := valueEncoders[mapType.Key().Kind()]
	valueEncoder := valueEncoders[mapType.Elem().Kind()]
	keys := v.MapKeys()
	for _, key := range keys {
		keyEncoder(w, key)
		valueEncoder(w, v.MapIndex(key))
	}
}

func writeMapFooter(w *Writer) {
	w.writeByte(TagClosebrace)
}

func writeMap(w *Writer, v reflect.Value) {
	count := v.Len()
	if count == 0 {
		writeEmptyMap(w)
		return
	}
	writeMapHeader(w, count)
	val := (*reflectValue)(unsafe.Pointer(&v))
	mapEncoder := mapBodyEncoders[val.typ]
	if mapEncoder != nil {
		mapEncoder(w, unsafe.Pointer(&val.ptr))
	} else {
		writeMapBody(w, v)
	}
	writeMapFooter(w)
}

func writeMapPtr(w *Writer, v reflect.Value) {
	count := v.Len()
	if count == 0 {
		writeEmptyMap(w)
		return
	}
	writeMapHeader(w, count)
	val := (*reflectValue)(unsafe.Pointer(&v))
	mapEncoder := mapBodyEncoders[val.typ]
	if mapEncoder != nil {
		mapEncoder(w, unsafe.Pointer(val.ptr))
	} else {
		writeMapBody(w, v)
	}
	writeMapFooter(w)
}

func writeStruct(w *Writer, v reflect.Value) {
	val := (*reflectValue)(unsafe.Pointer(&v))
	cache := getStructCache(v.Type().Elem())
	index, found := w.classref[val.typ]
	if !found {
		w.write(cache.Data)
		if !w.Simple {
			w.refcount += len(cache.Fields)
		}
		index = len(w.classref)
		w.classref[val.typ] = index
	}
	setRef(w, val.ptr)
	w.writeByte(TagObject)
	var buf [20]byte
	w.write(getIntBytes(buf[:], int64(index)))
	w.writeByte(TagOpenbrace)
	fields := cache.Fields
	for _, field := range fields {
		var f reflect.Value
		fp := (*reflectValue)(unsafe.Pointer(&f))
		fp.typ = field.Type
		fp.ptr = unsafe.Pointer(uintptr(val.ptr) + field.Offset)
		fp.flag = uintptr(field.Kind)
		if field.Kind == reflect.Ptr || field.Kind == reflect.Map {
			fp.ptr = **(**unsafe.Pointer)(unsafe.Pointer(&fp.ptr))
		}
		valueEncoders[field.Kind](w, f)
	}
	w.writeByte(TagClosebrace)
}

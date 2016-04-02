/*
If you cannot compile this file, just replace it with the one from no_gcc/ folder
*/

package qdb

/*
#include <stdlib.h>
#include <string.h>

void *alloc_ptr(void *c, unsigned long l) {
	void *ptr = malloc(l);
	memcpy(ptr, c, l);
	return ptr;
}

void *my_alloc(unsigned long l) {
	return malloc(l);
}

*/
import "C"

import (
	"os"
	"unsafe"
	"reflect"
	"sync/atomic"
)

type data_ptr_t unsafe.Pointer

func (v *oneIdx) FreeData() {
	if v.data!=nil {
		C.free(unsafe.Pointer(v.data))
		v.data = nil
		atomic.AddInt64(&ExtraMemoryConsumed, -int64(v.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, -1)
	}
}

func (v *oneIdx) Slice() (res []byte) {
	res = *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(v.data), Len:int(v.datlen), Cap:int(v.datlen)}))
	return
}

func newIdx(v []byte, f uint32) (r *oneIdx) {
	r = new(oneIdx)
	r.data = data_ptr_t(C.alloc_ptr(unsafe.Pointer(&v[0]), C.ulong(len(v))))
	r.datlen = uint32(len(v))
	atomic.AddInt64(&ExtraMemoryConsumed, int64(r.datlen))
	atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	r.flags = f
	return
}

func (r *oneIdx) SetData(v []byte) {
	if r.data!=nil {
		panic("This should not happen")
	}
	r.data = data_ptr_t(C.alloc_ptr(unsafe.Pointer(&v[0]), C.ulong(len(v))))
	atomic.AddInt64(&ExtraMemoryConsumed, int64(r.datlen))
	atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
}

func (v *oneIdx) LoadData(f *os.File) {
	if v.data!=nil {
		panic("This should not happen")
	}
	v.data = data_ptr_t(C.my_alloc(C.ulong(v.datlen)))
	atomic.AddInt64(&ExtraMemoryConsumed, int64(v.datlen))
	atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	f.Seek(int64(v.datpos), os.SEEK_SET)
	f.Read(*(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(v.data), Len:int(v.datlen), Cap:int(v.datlen)})))
}


func init() {
	println("Using mem_gcc for qdb records. Replace qdb/membind.go with the git version if you see issues.")
}

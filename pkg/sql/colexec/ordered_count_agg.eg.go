// Code generated by execgen; DO NOT EDIT.
// Copyright 2018 The Cockroach Authors.
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package colexec

import (
	"unsafe"

	"github.com/cockroachdb/cockroach/pkg/col/coldata"
	"github.com/cockroachdb/cockroach/pkg/sql/colmem"
)

func newCountRowsOrderedAggAlloc(
	allocator *colmem.Allocator, allocSize int64,
) aggregateFuncAlloc {
	return &countRowsOrderedAggAlloc{aggAllocBase: aggAllocBase{
		allocator: allocator,
		allocSize: allocSize,
	}}
}

// countRowsOrderedAgg supports either COUNT(*) or COUNT(col) aggregate.
type countRowsOrderedAgg struct {
	orderedAggregateFuncBase
	vec    []int64
	curAgg int64
}

var _ aggregateFunc = &countRowsOrderedAgg{}

const sizeOfCountRowsOrderedAgg = int64(unsafe.Sizeof(countRowsOrderedAgg{}))

func (a *countRowsOrderedAgg) Init(groups []bool, vec coldata.Vec) {
	a.orderedAggregateFuncBase.Init(groups, vec)
	a.vec = vec.Int64()
	a.Reset()
}

func (a *countRowsOrderedAgg) Reset() {
	a.orderedAggregateFuncBase.Reset()
	a.curAgg = 0
}

func (a *countRowsOrderedAgg) Compute(
	vecs []coldata.Vec, inputIdxs []uint32, inputLen int, sel []int,
) {
	var i int

	{
		if sel != nil {
			for _, i = range sel[:inputLen] {
				if a.groups[i] {
					a.vec[a.curIdx] = a.curAgg
					a.curIdx++
					a.curAgg = int64(0)
				}

				var y int64
				y = int64(1)
				a.curAgg += y
			}
		} else {
			for i = 0; i < inputLen; i++ {
				if a.groups[i] {
					a.vec[a.curIdx] = a.curAgg
					a.curIdx++
					a.curAgg = int64(0)
				}

				var y int64
				y = int64(1)
				a.curAgg += y
			}
		}
	}
}

func (a *countRowsOrderedAgg) Flush(outputIdx int) {
	// Go around "argument overwritten before first use" linter error.
	_ = outputIdx
	outputIdx = a.curIdx
	a.curIdx++
	a.vec[outputIdx] = a.curAgg
}

func (a *countRowsOrderedAgg) HandleEmptyInputScalar() {
	// COUNT aggregates are special because they return zero in case of an
	// empty input in the scalar context.
	a.vec[0] = 0
}

type countRowsOrderedAggAlloc struct {
	aggAllocBase
	aggFuncs []countRowsOrderedAgg
}

var _ aggregateFuncAlloc = &countRowsOrderedAggAlloc{}

func (a *countRowsOrderedAggAlloc) newAggFunc() aggregateFunc {
	if len(a.aggFuncs) == 0 {
		a.allocator.AdjustMemoryUsage(sizeOfCountRowsOrderedAgg * a.allocSize)
		a.aggFuncs = make([]countRowsOrderedAgg, a.allocSize)
	}
	f := &a.aggFuncs[0]
	a.aggFuncs = a.aggFuncs[1:]
	return f
}

func newCountOrderedAggAlloc(
	allocator *colmem.Allocator, allocSize int64,
) aggregateFuncAlloc {
	return &countOrderedAggAlloc{aggAllocBase: aggAllocBase{
		allocator: allocator,
		allocSize: allocSize,
	}}
}

// countOrderedAgg supports either COUNT(*) or COUNT(col) aggregate.
type countOrderedAgg struct {
	orderedAggregateFuncBase
	vec    []int64
	curAgg int64
}

var _ aggregateFunc = &countOrderedAgg{}

const sizeOfCountOrderedAgg = int64(unsafe.Sizeof(countOrderedAgg{}))

func (a *countOrderedAgg) Init(groups []bool, vec coldata.Vec) {
	a.orderedAggregateFuncBase.Init(groups, vec)
	a.vec = vec.Int64()
	a.Reset()
}

func (a *countOrderedAgg) Reset() {
	a.orderedAggregateFuncBase.Reset()
	a.curAgg = 0
}

func (a *countOrderedAgg) Compute(
	vecs []coldata.Vec, inputIdxs []uint32, inputLen int, sel []int,
) {
	var i int

	// If this is a COUNT(col) aggregator and there are nulls in this batch,
	// we must check each value for nullity. Note that it is only legal to do a
	// COUNT aggregate on a single column.
	nulls := vecs[inputIdxs[0]].Nulls()
	if nulls.MaybeHasNulls() {
		if sel != nil {
			for _, i = range sel[:inputLen] {
				if a.groups[i] {
					a.vec[a.curIdx] = a.curAgg
					a.curIdx++
					a.curAgg = int64(0)
				}

				var y int64
				y = int64(0)
				if !nulls.NullAt(i) {
					y = 1
				}
				a.curAgg += y
			}
		} else {
			for i = 0; i < inputLen; i++ {
				if a.groups[i] {
					a.vec[a.curIdx] = a.curAgg
					a.curIdx++
					a.curAgg = int64(0)
				}

				var y int64
				y = int64(0)
				if !nulls.NullAt(i) {
					y = 1
				}
				a.curAgg += y
			}
		}
	} else {
		if sel != nil {
			for _, i = range sel[:inputLen] {
				if a.groups[i] {
					a.vec[a.curIdx] = a.curAgg
					a.curIdx++
					a.curAgg = int64(0)
				}

				var y int64
				y = int64(1)
				a.curAgg += y
			}
		} else {
			for i = 0; i < inputLen; i++ {
				if a.groups[i] {
					a.vec[a.curIdx] = a.curAgg
					a.curIdx++
					a.curAgg = int64(0)
				}

				var y int64
				y = int64(1)
				a.curAgg += y
			}
		}
	}
}

func (a *countOrderedAgg) Flush(outputIdx int) {
	// Go around "argument overwritten before first use" linter error.
	_ = outputIdx
	outputIdx = a.curIdx
	a.curIdx++
	a.vec[outputIdx] = a.curAgg
}

func (a *countOrderedAgg) HandleEmptyInputScalar() {
	// COUNT aggregates are special because they return zero in case of an
	// empty input in the scalar context.
	a.vec[0] = 0
}

type countOrderedAggAlloc struct {
	aggAllocBase
	aggFuncs []countOrderedAgg
}

var _ aggregateFuncAlloc = &countOrderedAggAlloc{}

func (a *countOrderedAggAlloc) newAggFunc() aggregateFunc {
	if len(a.aggFuncs) == 0 {
		a.allocator.AdjustMemoryUsage(sizeOfCountOrderedAgg * a.allocSize)
		a.aggFuncs = make([]countOrderedAgg, a.allocSize)
	}
	f := &a.aggFuncs[0]
	a.aggFuncs = a.aggFuncs[1:]
	return f
}

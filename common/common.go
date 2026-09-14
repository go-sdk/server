// Package common 提供跨服务复用的 Protobuf 类型及其 Go 辅助方法。
package common

import (
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	// NewAny 及 Any 前缀方法提供 google.protobuf.Any 的常用构造和转换能力。
	NewAny          = anypb.New
	AnyMarshalFrom  = anypb.MarshalFrom
	AnyUnmarshalNew = anypb.UnmarshalNew
	AnyUnmarshalTo  = anypb.UnmarshalTo

	// NewDuration 根据 time.Duration 构造 google.protobuf.Duration。
	NewDuration = durationpb.New

	// NewList 和 NewStruct 将普通 Go 数据转换为对应的结构化 Protobuf 值。
	NewList   = structpb.NewList
	NewStruct = structpb.NewStruct

	// NewValue 及其同组方法构造 google.protobuf.Value 及具体值类型。
	NewValue       = structpb.NewValue
	NewBoolValue   = structpb.NewBoolValue
	NewListValue   = structpb.NewListValue
	NewNullValue   = structpb.NewNullValue
	NewNumberValue = structpb.NewNumberValue
	NewStringValue = structpb.NewStringValue
	NewStructValue = structpb.NewStructValue

	// NewTimestamp 根据 time.Time 构造 google.protobuf.Timestamp。
	NewTimestamp = timestamppb.New

	// Bool 至 UInt64 提供 google.protobuf 包装类型的便捷构造方法。
	Bool   = wrapperspb.Bool
	Bytes  = wrapperspb.Bytes
	Double = wrapperspb.Double
	Float  = wrapperspb.Float
	Int32  = wrapperspb.Int32
	Int64  = wrapperspb.Int64
	String = wrapperspb.String
	UInt32 = wrapperspb.UInt32
	UInt64 = wrapperspb.UInt64
)

// NewId 构造单个资源标识。
func NewId(id string) *Id {
	return &Id{Id: id}
}

// NewIds 构造一组资源标识。
func NewIds(ids ...string) *Ids {
	return &Ids{Ids: ids}
}

// DefaultLimit 是未指定有效每页数量时使用的默认值。
var DefaultLimit = 20

// NewPaging 构造分页对象，并将非正数页码和每页数量替换为默认值。
func NewPaging(page, limit int) *Paging {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	return &Paging{
		Page:     int32(page),
		PageSize: int32(limit),
	}
}

func (x *Paging) init() (int, int) {
	page, limit := 0, 0
	if x != nil {
		page, limit = int(x.Page), int(x.PageSize)
	}
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	return page, limit
}

// GetOffsetLimit 返回从零开始的偏移量和规范化后的每页数量。
func (x *Paging) GetOffsetLimit() (int, int) {
	page, limit := x.init()
	return (page - 1) * limit, limit
}

// GetOffset 返回从零开始的分页偏移量。
func (x *Paging) GetOffset() int {
	v, _ := x.GetOffsetLimit()
	return v
}

// GetLimit 返回规范化后的每页数量。
func (x *Paging) GetLimit() int {
	_, v := x.GetOffsetLimit()
	return v
}

// WithTotal 返回包含规范化页码、每页数量和总记录数的新分页对象。
func (x *Paging) WithTotal(total int64) *Paging {
	page, limit := x.init()
	return &Paging{
		Page:     int32(page),
		PageSize: int32(limit),
		Total:    int32(total),
	}
}

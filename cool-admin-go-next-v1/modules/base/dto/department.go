package dto

import (
   "bytes"
   "encoding/json"
   "fmt"
   "io"
)

// 部门排序项
type DepartmentOrderItem struct {
   ID       int64  `json:"id" v:"required|min:1"`
   ParentID *int64 `json:"parentId"`
   OrderNum int64  `json:"orderNum" v:"min:0"`
}

// 部门排序请求(保持 Node 的顶层数组 payload,同时让 runtime 使用 struct DTO)
type DepartmentOrderReq struct {
   DepartmentOrders []DepartmentOrderItem
}

func (r *DepartmentOrderReq) UnmarshalJSON(data []byte) error {
   decoder := json.NewDecoder(bytes.NewReader(data))
   decoder.DisallowUnknownFields()
   if err := decoder.Decode(&r.DepartmentOrders); err != nil {
      return err
   }
   var extra interface{}
   if err := decoder.Decode(&extra); err != io.EOF {
      if err == nil {
         return fmt.Errorf("请求 JSON 不能包含多个值")
      }
      return err
   }
   return nil
}

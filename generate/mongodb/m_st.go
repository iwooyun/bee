// @Time : 2019/9/3 11:40
// @Author : duanqiangwen
// @File : doc_model
// @Software: GoLand
package mongodb

import (
	"gopkg.in/mgo.v2/bson"
	"time"
)

const (
	IsDeletedFalse = 0
	IsDeletedTrue  = 1
	DefaultVersion = "1.0"
)

type SwaggerLocation struct {
	Id_       bson.ObjectId          `json:"_id" bson:"_id,omitempty"`
	ProjectId bson.ObjectId          `json:"project_id" bson:"project_id"`
	Version   string                 `json:"version" bson:"version"`
	Hash      string                 `json:"hash" bson:"hash"`
	Location  map[string]interface{} `json:"location" bson:"location"`
	IsDeleted uint8                  `json:"is_deleted" bson:"is_deleted"`
	CreatedAt time.Time              `json:"created_at"  bson:"created_at"`
	UpdateAt  time.Time              `json:"updated_at"  bson:"updated_at"`
}

type Project struct {
	Id_       bson.ObjectId `json:"_id" bson:"_id,omitempty"`
	Name      string        `json:"name" bson:"name"`
	Host      string        `json:"host" bson:"host"`
	IsDeleted uint8         `json:"is_deleted" bson:"is_deleted"`
	CreatedAt time.Time     `json:"created_at"  bson:"created_at"`
	UpdateAt  time.Time     `json:"updated_at"  bson:"updated_at"`
}

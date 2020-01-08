// @Time : 2019/9/3 10:26
// @Author : duanqiangwen
// @File : client
// @Software: GoLand
package mongodb

import (
	"fmt"
	beeLogger "github.com/iwooyun/bee/logger"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var dbClient *mgo.Database

func FindProjectByHost(host string) *Project {
	c := dbClient.C("center_project")
	result := &Project{}
	_ = c.Find(bson.M{"host": host, "is_deleted": IsDeletedFalse}).One(result)
	return result
}

func FindLocationMaxVersion(projectId bson.ObjectId) *SwaggerLocation {
	c := dbClient.C("swagger_location")
	result := &SwaggerLocation{}
	_ = c.Find(bson.M{"project_id": projectId, "is_deleted": IsDeletedFalse}).Sort("-version").One(result)
	return result
}

func Insert(db string, docs ...interface{}) {
	c := dbClient.C(db)
	err := c.Insert(docs...)
	if err != nil {
		panic(err)
	}
}

func init() {
	url := fmt.Sprintf("mongodb://%s:%s@%s:%s/%s",
		MongoUsername, MongoPassword, MongoHost, MongoPort, MongoDatabase)
	session, err := mgo.Dial(url)
	if err != nil {
		beeLogger.Log.Fatalf("mongodb connect failed! err => %s", err)
		return
	}

	// defer session.Close()
	session.SetMode(mgo.Monotonic, true)
	dbClient = session.DB(MongoDatabase)
}

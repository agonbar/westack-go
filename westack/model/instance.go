package model

import (
	"fmt"
	"log"
	"strconv"

	"github.com/oliveagle/jsonpath"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	wst "github.com/fredyk/westack-go/westack/common"
	"github.com/fredyk/westack-go/westack/datasource"
)

type Instance struct {
	Model *Model
	Id    interface{}

	data  wst.M
	bytes []byte
}

type InstanceA []Instance

func (modelInstance *Instance) ToJSON() wst.M {

	if modelInstance == nil {
		return nil
	}

	var result wst.M
	result = wst.CopyMap(modelInstance.data)
	for relationName, relationConfig := range *modelInstance.Model.Config.Relations {
		if modelInstance.data[relationName] != nil {
			if relationConfig.Type == "" {
				// relation not found
				continue
			}
			rawRelatedData := modelInstance.data[relationName]
			relatedModel, err := modelInstance.Model.App.FindModel(relationConfig.Model)
			if err != nil {
				return nil
			}
			if relatedModel != nil {
				switch {
				case isSingleRelation(relationConfig.Type):
					relatedInstance := rawRelatedData.(*Instance).ToJSON()
					result[relationName] = relatedInstance
					break
				case isManyRelation(relationConfig.Type):
					aux := make(wst.A, len(rawRelatedData.([]Instance)))
					for idx, v := range rawRelatedData.([]Instance) {
						aux[idx] = v.ToJSON()
					}
					result[relationName] = aux
					break
				}
			}
		}
	}

	return result
}

func (modelInstance *Instance) Get(relationName string) interface{} {
	result := modelInstance.data[relationName]
	switch (*modelInstance.Model.Config.Relations)[relationName].Type {
	case "hasMany", "hasAndBelongsToMany":
		if result == nil {
			result = make([]Instance, 0)
		}
		break
	}
	return result
}

func (modelInstance *Instance) GetOne(relationName string) *Instance {
	result := modelInstance.Get(relationName)
	if result == nil {
		return nil
	}
	return result.(*Instance)
}

func (modelInstance *Instance) GetMany(relationName string) []Instance {
	return modelInstance.Get(relationName).([]Instance)
}

func (modelInstance *Instance) HideProperties() {
	for _, propertyName := range modelInstance.Model.Config.Hidden {
		delete(modelInstance.data, propertyName)
		// TODO: Hide in nested
	}
}

func (modelInstance *Instance) Transform(out interface{}) error {
	err := bson.Unmarshal(modelInstance.bytes, out)
	if err != nil {
		return err
	}
	return nil
}

func (modelInstance *Instance) UncheckedTransform(out interface{}) interface{} {
	err := modelInstance.Transform(out)
	if err != nil {
		panic(err)
	}
	return out
}

func (modelInstance *Instance) UpdateAttributes(data interface{}, baseContext *EventContext) (*Instance, error) {

	var finalData wst.M
	switch data.(type) {
	case map[string]interface{}:
		finalData = wst.M{}
		for key, value := range data.(map[string]interface{}) {
			finalData[key] = value
		}
		break
	case *map[string]interface{}:
		finalData = wst.M{}
		for key, value := range *data.(*map[string]interface{}) {
			finalData[key] = value
		}
		break
	case wst.M:
		finalData = data.(wst.M)
		break
	case *wst.M:
		finalData = *data.(*wst.M)
		break
	case Instance:
		value := data.(Instance)
		finalData = (&value).ToJSON()
		break
	case *Instance:
		finalData = data.(*Instance).ToJSON()
		break
	default:
		log.Fatal(fmt.Sprintf("Invalid input for Model.UpdateAttributes() <- %s", data))
	}

	if baseContext == nil {
		baseContext = &EventContext{}
	}
	var targetBaseContext = baseContext
	deepLevel := 0
	for {
		if targetBaseContext.BaseContext != nil {
			targetBaseContext = targetBaseContext.BaseContext
		} else {
			break
		}
		deepLevel++
	}
	if !baseContext.DisableTypeConversions {
		datasource.ReplaceObjectIds(finalData)
	}

	eventContext := &EventContext{
		BaseContext: targetBaseContext,
	}
	eventContext.Data = &finalData
	eventContext.Instance = modelInstance
	eventContext.ModelID = modelInstance.Id
	eventContext.IsNewInstance = false
	if modelInstance.Model.DisabledHandlers["__operation__before_save"] != true {
		err := modelInstance.Model.GetHandler("__operation__before_save")(eventContext)
		if err != nil {
			return nil, err
		}
	}

	for key := range *modelInstance.Model.Config.Relations {
		delete(finalData, key)
	}
	document, err := modelInstance.Model.Datasource.UpdateById(modelInstance.Model.CollectionName, modelInstance.Id, &finalData)

	if err != nil {
		return nil, err
	} else if document == nil {
		return nil, datasource.NewError(400, "Could not update document")
	} else {
		err := modelInstance.Reload(eventContext)
		modelInstance.HideProperties()
		if err != nil {
			return nil, err
		}
		eventContext.Instance = modelInstance
		eventContext.ModelID = modelInstance.Id
		eventContext.IsNewInstance = false
		if modelInstance.Model.DisabledHandlers["__operation__after_save"] != true {
			err = modelInstance.Model.GetHandler("__operation__after_save")(eventContext)
			if err != nil {
				return nil, err
			}
		}
		return modelInstance, nil
	}
}

func (modelInstance *Instance) Reload(eventContext *EventContext) error {
	newInstance, err := modelInstance.Model.FindById(modelInstance.Id, nil, eventContext)
	if err != nil {
		return err
	}
	for k := range modelInstance.data {
		if (*modelInstance.Model.Config.Relations)[k] == nil {
			delete(modelInstance.data, k)
		}
	}
	for k, v := range newInstance.data {
		if (*modelInstance.Model.Config.Relations)[k] == nil {
			modelInstance.data[k] = v
		}
	}
	modelInstance.data = newInstance.data
	_bytes, err := bson.Marshal(modelInstance.data)
	if err != nil {
		return err
	}
	modelInstance.bytes = _bytes
	return nil
}

func (modelInstance *Instance) GetString(path string) string {
	res, err := jsonpath.JsonPathLookup(modelInstance.data, fmt.Sprintf("$.%v", path))
	if err != nil {
		return ""
	}
	switch res.(type) {
	case string:
		return res.(string)
	case float64:
		return strconv.FormatFloat(res.(float64), 'f', -1, 64)
	case int:
		return strconv.Itoa(res.(int))
	case int64:
		return strconv.FormatInt(res.(int64), 10)
	case bool:
		return strconv.FormatBool(res.(bool))
	default:
		return ""
	}
}

func (modelInstance *Instance) GetFloat64(path string) float64 {
	res, err := jsonpath.JsonPathLookup(modelInstance.data, fmt.Sprintf("$.%v", path))
	if err != nil {
		return 0
	}
	switch res.(type) {
	case string:
		aux, err := strconv.ParseFloat(res.(string), 64)
		if err != nil {
			return 0
		}
		return aux
	case float64:
		return res.(float64)
	case int:
		return float64(res.(int))
	case int64:
		return float64(res.(int64))
	case bool:
		if res.(bool) {
			return 1
		} else {
			return 0
		}
	default:
		return 0
	}
}

func (modelInstance *Instance) GetInt(path string) int64 {

	res, err := jsonpath.JsonPathLookup(modelInstance.data, fmt.Sprintf("$.%v", path))
	if err != nil {
		return 0
	}
	switch res.(type) {
	case float64:
		return int64(res.(float64))
	case int64:
		return res.(int64)
	case int32:
		return int64(res.(int32))
	case int:
		return int64(res.(int))
	case float32:
		return int64(res.(float32))
	default:
		return 0
	}
}

func (modelInstance *Instance) GetBoolean(path string, defaultValue bool) bool {
	res, err := jsonpath.JsonPathLookup(modelInstance.data, fmt.Sprintf("$.%v", path))
	if err != nil {
		return defaultValue
	}
	switch res.(type) {
	case bool:
		return res.(bool)
	default:
		return defaultValue
	}
}

func (modelInstance *Instance) GetObjectId(path string) (result primitive.ObjectID) {
	res, err := jsonpath.JsonPathLookup(modelInstance.data, fmt.Sprintf("$.%v", path))
	result = primitive.NilObjectID
	if err == nil {
		switch res.(type) {
		case string:
			_id, err := primitive.ObjectIDFromHex(res.(string))
			if err == nil {
				result = _id
			}
			break
		case primitive.ObjectID:
			result = res.(primitive.ObjectID)
			break
		}
	}
	return result
}

func (modelInstance *Instance) GetM(path string) *wst.M {
	res, err := jsonpath.JsonPathLookup(modelInstance.data, fmt.Sprintf("$.%v", path))
	if err != nil {
		return nil
	}
	switch res.(type) {
	case wst.M:
		v := res.(wst.M)
		return &v
	case primitive.M:
		out := &wst.M{}
		for k, v := range res.(primitive.M) {
			(*out)[k] = v
		}
		return out
	case map[string]interface{}:
		out := &wst.M{}
		for k, v := range res.(map[string]interface{}) {
			(*out)[k] = v
		}
		return out
	default:
		return nil
	}
}

func (modelInstance *Instance) GetA(path string) *wst.A {
	res, err := jsonpath.JsonPathLookup(modelInstance.data, fmt.Sprintf("$.%v", path))
	if err != nil {
		return nil
	}
	switch res.(type) {
	case wst.A:
		v := res.(wst.A)
		return &v
	case primitive.A:
		out := &wst.A{}
		for idx, v := range res.(primitive.A) {
			*out = append(*out, wst.M{})
			for k, v := range v.(primitive.M) {
				(*out)[idx][k] = v
			}
		}
		return out
	case []interface{}:
		out := &wst.A{}
		for idx, v := range res.([]interface{}) {
			*out = append(*out, wst.M{})
			switch v.(type) {
			case wst.M:
				for k, v := range v.(wst.M) {
					(*out)[idx][k] = v
				}
			case primitive.M:
				for k, v := range v.(primitive.M) {
					(*out)[idx][k] = v
				}
			}
		}
		return out
	case []map[string]interface{}:
		out := &wst.A{}
		for idx, v := range res.([]map[string]interface{}) {
			*out = append(*out, wst.M{})
			for k, v := range v {
				(*out)[idx][k] = v
			}
		}
		return out
	default:
		log.Printf("WARNING: GetA: %v <%s> is not an array\n", path, modelInstance.data[path])
		return nil
	}
}

func (instances InstanceA) ToJSON() []wst.M {
	result := make([]wst.M, len(instances))
	for idx, instance := range instances {
		result[idx] = instance.ToJSON()
	}
	return result
}

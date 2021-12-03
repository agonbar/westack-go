package common

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"regexp"
	"strings"
)

type IApp struct {
	Debug        bool
	SwaggerPaths func() *map[string]map[string]interface{}
}

func LoadFile(filePath string, out interface{}) error {
	jsonFile, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	//var result map[string]interface{}
	err2 := json.Unmarshal(jsonFile, &out)
	if err2 != nil {
		return err2
	}
	return nil
}

func DashedCase(st string) string {
	var res = strings.ToLower(st[:1])
	compile, err := regexp.Compile("([A-Z])")
	if err != nil {
		log.Println(err)
		return ""
	}
	res += string(compile.ReplaceAllFunc([]byte(st[1:]), func(bytes []byte) []byte {
		return []byte("-" + strings.ToLower(string(bytes[0])))
	}))
	return res
}

func CopyMap(src map[string]interface{}) map[string]interface{} {
	// Create the target map
	targetMap := make(map[string]interface{})

	// Copy from the original map to the target map
	for key, value := range src {
		targetMap[key] = value
	}
	return targetMap
}
